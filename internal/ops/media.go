package ops

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

// FileOpts controls media sending: caption, document override, reply and
// Markdown vs plain caption parsing.
type FileOpts struct {
	Caption    string
	AsDocument bool // force image/* as document; default for images is photo
	ReplyTo    int
	Plain      bool
}

// classifyUpload determines how a local file should be sent: image/* becomes a
// photo unless asDocument forces a document; video/* and audio/* map to their
// kinds; everything else is a document. The returned mime is derived from the
// extension and defaults to application/octet-stream for documents.
func classifyUpload(path string, asDocument bool) (string, string) {
	m := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = m[:i]
	}
	switch {
	case strings.HasPrefix(m, "image/") && !asDocument:
		return "photo", m
	case strings.HasPrefix(m, "video/"):
		return "video", m
	case strings.HasPrefix(m, "audio/"):
		return "audio", m
	default:
		if m == "" {
			m = "application/octet-stream"
		}
		return "document", m
	}
}

// downloadRoot is the base directory for downloads: TGC_DOWNLOAD_DIR when set,
// else ~/.tgc/downloads.
func downloadRoot() string {
	if d := os.Getenv("TGC_DOWNLOAD_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tgc", "downloads")
}

// uniquePath returns p if free, else appends " (1)", " (2)", ... before the
// extension until a non-existing path is found.
func uniquePath(p string) string {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}

// uploadedMedia uploads a local file and builds the corresponding
// InputMediaUploaded* (photo for images by default; document otherwise).
func uploadedMedia(conn *client.Conn, path string, asDocument bool) (tg.InputMediaClass, error) {
	up := uploader.NewUploader(conn.Ctx.Raw)
	f, err := up.FromPath(conn.Ctx, path)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	kind, mimeType := classifyUpload(path, asDocument)
	if kind == "photo" {
		return &tg.InputMediaUploadedPhoto{File: f}, nil
	}
	attrs := []tg.DocumentAttributeClass{
		&tg.DocumentAttributeFilename{FileName: filepath.Base(path)},
	}
	if kind == "video" {
		attrs = append(attrs, &tg.DocumentAttributeVideo{SupportsStreaming: true})
	}
	return &tg.InputMediaUploadedDocument{
		File:       f,
		MimeType:   mimeType,
		Attributes: attrs,
	}, nil
}

// SendFiles sends one file (messages.sendMedia) or an album of 2–10
// (messages.sendMultiMedia). The count is validated before conn is used.
// Caption (Markdown unless Plain) applies to the first message only. Images
// default to photo; AsDocument forces document. It returns one result map per
// sent message ({message_id, chat_id, date, grouped_id?}).
func SendFiles(conn *client.Conn, selector string, files []string, o FileOpts) ([]map[string]any, error) {
	if len(files) == 0 {
		return nil, output.Errf("bad_args", "no files to send")
	}
	if len(files) > 10 {
		return nil, output.Errf("bad_args", "too many files: %d (max 10 per album); split into batches", len(files))
	}

	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	body, entities, err := parseText(o.Caption, o.Plain)
	if err != nil {
		return nil, err
	}

	if len(files) == 1 {
		return sendSingleFile(conn, peer.ID, ip, files[0], body, entities, o)
	}
	return sendAlbum(conn, peer.ID, ip, files, body, entities, o)
}

func sendSingleFile(conn *client.Conn, chatID int64, ip tg.InputPeerClass, path, body string, entities []tg.MessageEntityClass, o FileOpts) ([]map[string]any, error) {
	media, err := uploadedMedia(conn, path, o.AsDocument)
	if err != nil {
		return nil, err
	}
	req := &tg.MessagesSendMediaRequest{
		Peer:     ip,
		Media:    media,
		Message:  body,
		RandomID: randomID(),
	}
	if len(entities) > 0 {
		req.SetEntities(entities)
	}
	if o.ReplyTo > 0 {
		req.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: o.ReplyTo})
	}
	upd, err := conn.Ctx.Raw.MessagesSendMedia(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	return []map[string]any{sentResult(upd, chatID)}, nil
}

func sendAlbum(conn *client.Conn, chatID int64, ip tg.InputPeerClass, files []string, body string, entities []tg.MessageEntityClass, o FileOpts) ([]map[string]any, error) {
	multi := make([]tg.InputSingleMedia, 0, len(files))
	for i, path := range files {
		uploaded, err := uploadedMedia(conn, path, o.AsDocument)
		if err != nil {
			return nil, err
		}
		// sendMultiMedia requires each media to be pre-uploaded via
		// messages.uploadMedia and referenced by its input constructor.
		ready, err := conn.Ctx.Raw.MessagesUploadMedia(conn.Ctx, &tg.MessagesUploadMediaRequest{
			Peer:  ip,
			Media: uploaded,
		})
		if err != nil {
			return nil, client.WrapErr(err)
		}
		inputMedia, err := inputMediaFromReady(ready)
		if err != nil {
			return nil, err
		}
		single := tg.InputSingleMedia{
			Media:    inputMedia,
			RandomID: randomID(),
		}
		if i == 0 {
			single.Message = body
			if len(entities) > 0 {
				single.SetEntities(entities)
			}
		}
		multi = append(multi, single)
	}

	req := &tg.MessagesSendMultiMediaRequest{
		Peer:       ip,
		MultiMedia: multi,
	}
	if o.ReplyTo > 0 {
		req.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: o.ReplyTo})
	}
	upd, err := conn.Ctx.Raw.MessagesSendMultiMedia(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	return albumResults(upd, chatID), nil
}

// inputMediaFromReady converts a messages.uploadMedia result into the
// InputMedia* constructor needed for messages.sendMultiMedia.
func inputMediaFromReady(m tg.MessageMediaClass) (tg.InputMediaClass, error) {
	switch mv := m.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := mv.Photo.(*tg.Photo)
		if !ok {
			return nil, output.Errf("upload_failed", "uploaded photo is not available")
		}
		return &tg.InputMediaPhoto{ID: &tg.InputPhoto{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
		}}, nil
	case *tg.MessageMediaDocument:
		doc, ok := mv.Document.(*tg.Document)
		if !ok {
			return nil, output.Errf("upload_failed", "uploaded document is not available")
		}
		return &tg.InputMediaDocument{ID: &tg.InputDocument{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
		}}, nil
	default:
		return nil, output.Errf("upload_failed", "unexpected uploaded media type")
	}
}

// albumResults collects every new message from an album send update, tagging
// each with its grouped_id when present.
func albumResults(upd tg.UpdatesClass, chatID int64) []map[string]any {
	u, ok := upd.(*tg.Updates)
	if !ok {
		return []map[string]any{sentResult(upd, chatID)}
	}
	var out []map[string]any
	for _, up := range u.Updates {
		var mc tg.MessageClass
		switch nu := up.(type) {
		case *tg.UpdateNewMessage:
			mc = nu.Message
		case *tg.UpdateNewChannelMessage:
			mc = nu.Message
		default:
			continue
		}
		m, ok := mc.(*tg.Message)
		if !ok {
			continue
		}
		res := map[string]any{
			"message_id": m.ID,
			"chat_id":    chatID,
			"date":       time.Unix(int64(m.Date), 0).UTC().Format(time.RFC3339),
		}
		if gid, ok := m.GetGroupedID(); ok {
			res["grouped_id"] = gid
		}
		out = append(out, res)
	}
	if len(out) == 0 {
		return []map[string]any{sentResult(upd, chatID)}
	}
	return out
}

// Download fetches the media attached to msgID in selector. With toStdout the
// raw bytes go to stdout and no JSON is printed (the CLI layer handles that);
// otherwise the file is written under outPath (or the default download tree)
// and a {path, size, mime, file_name} result is returned.
func Download(conn *client.Conn, selector string, msgID int, outPath string, toStdout bool) (map[string]any, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	msg, err := fetchMessage(conn, peer, msgID)
	if err != nil {
		return nil, err
	}
	if msg.Media == nil {
		return nil, output.Errf("no_media", "message %d has no downloadable media", msgID)
	}
	loc, meta, err := downloadLocation(msg.Media, msgID)
	if err != nil {
		return nil, err
	}

	dl := downloader.NewDownloader()
	if toStdout {
		if _, err := dl.Download(conn.Ctx.Raw, loc).Stream(conn.Ctx, os.Stdout); err != nil {
			return nil, client.WrapErr(err)
		}
		return map[string]any{
			"path":      nil,
			"size":      meta.size,
			"mime":      meta.mime,
			"file_name": meta.name,
		}, nil
	}

	dest, err := resolveDest(outPath, meta)
	if err != nil {
		return nil, err
	}
	if _, err := dl.Download(conn.Ctx.Raw, loc).ToPath(conn.Ctx, dest); err != nil {
		return nil, client.WrapErr(err)
	}
	size := meta.size
	if fi, statErr := os.Stat(dest); statErr == nil {
		size = fi.Size()
	}
	return map[string]any{
		"path":      dest,
		"size":      size,
		"mime":      meta.mime,
		"file_name": meta.name,
	}, nil
}

// mediaMeta describes downloadable media for path and result construction.
type mediaMeta struct {
	fileID int64
	name   string
	mime   string
	size   int64
}

// fetchMessage retrieves a single *tg.Message by id, routing channel peers
// through channels.getMessages and everything else through messages.getMessages.
func fetchMessage(conn *client.Conn, peer *resolve.Peer, msgID int) (*tg.Message, error) {
	ids := []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}}
	var (
		res tg.MessagesMessagesClass
		err error
	)
	if peer.Type == "channel" || (peer.Type == "group" && peer.AccessHash != 0) {
		ch, cerr := resolve.InputChannel(peer)
		if cerr != nil {
			return nil, cerr
		}
		res, err = conn.Ctx.Raw.ChannelsGetMessages(conn.Ctx, &tg.ChannelsGetMessagesRequest{
			Channel: ch,
			ID:      ids,
		})
	} else {
		res, err = conn.Ctx.Raw.MessagesGetMessages(conn.Ctx, ids)
	}
	if err != nil {
		return nil, client.WrapErr(err)
	}
	var msgs []tg.MessageClass
	switch r := res.(type) {
	case *tg.MessagesMessages:
		msgs = r.Messages
	case *tg.MessagesMessagesSlice:
		msgs = r.Messages
	case *tg.MessagesChannelMessages:
		msgs = r.Messages
	default:
		return nil, output.Errf("not_found", "message %d not found", msgID)
	}
	for _, mc := range msgs {
		if m, ok := mc.(*tg.Message); ok && m.ID == msgID {
			return m, nil
		}
	}
	return nil, output.Errf("not_found", "message %d not found", msgID)
}

// downloadLocation builds the file location and metadata for a message's media.
func downloadLocation(media tg.MessageMediaClass, msgID int) (tg.InputFileLocationClass, mediaMeta, error) {
	switch mv := media.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := mv.Photo.(*tg.Photo)
		if !ok {
			return nil, mediaMeta{}, output.Errf("no_media", "message %d has no downloadable media", msgID)
		}
		sizeType, sizeBytes := largestPhotoSize(photo.Sizes)
		if sizeType == "" {
			return nil, mediaMeta{}, output.Errf("no_media", "message %d has no downloadable photo size", msgID)
		}
		loc := &tg.InputPhotoFileLocation{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			ThumbSize:     sizeType,
		}
		meta := mediaMeta{
			fileID: photo.ID,
			name:   fmt.Sprintf("photo_%d.jpg", msgID),
			mime:   "image/jpeg",
			size:   int64(sizeBytes),
		}
		return loc, meta, nil
	case *tg.MessageMediaDocument:
		doc, ok := mv.Document.(*tg.Document)
		if !ok {
			return nil, mediaMeta{}, output.Errf("no_media", "message %d has no downloadable media", msgID)
		}
		loc := &tg.InputDocumentFileLocation{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
			ThumbSize:     "",
		}
		meta := mediaMeta{
			fileID: doc.ID,
			name:   documentName(doc, msgID),
			mime:   doc.MimeType,
			size:   doc.Size,
		}
		return loc, meta, nil
	default:
		return nil, mediaMeta{}, output.Errf("no_media", "message %d has no downloadable media", msgID)
	}
}

// largestPhotoSize returns the type and byte size of the largest photo size.
func largestPhotoSize(sizes []tg.PhotoSizeClass) (string, int) {
	var (
		bestType string
		bestSize int
	)
	for _, s := range sizes {
		switch ps := s.(type) {
		case *tg.PhotoSize:
			if ps.Size >= bestSize {
				bestSize, bestType = ps.Size, ps.Type
			}
		case *tg.PhotoSizeProgressive:
			largest := 0
			for _, n := range ps.Sizes {
				if n > largest {
					largest = n
				}
			}
			if largest >= bestSize {
				bestSize, bestType = largest, ps.Type
			}
		}
	}
	return bestType, bestSize
}

// sanitizeName reduces a server-supplied filename to a safe base component,
// preventing path traversal: it strips any directory portion (absolute or
// relative, including "..") and falls back to a safe default for names that
// are empty, "." or "..".
func sanitizeName(raw, fallback string) string {
	base := filepath.Base(raw)
	if base == "" || base == "." || base == ".." || base == string(os.PathSeparator) {
		return fallback
	}
	return base
}

// documentName returns the document's filename attribute (sanitized to a base
// component to prevent path traversal), or a fallback based on msgID when
// unnamed.
func documentName(doc *tg.Document, msgID int) string {
	fallback := fmt.Sprintf("file_%d", msgID)
	for _, attr := range doc.Attributes {
		if fn, ok := attr.(*tg.DocumentAttributeFilename); ok && fn.FileName != "" {
			return sanitizeName(fn.FileName, fallback)
		}
	}
	return fallback
}

// resolveDest turns the -o override (empty, file, or directory) plus media
// metadata into a concrete, conflict-free destination path, creating parent
// directories with 0700 as needed.
func resolveDest(outPath string, meta mediaMeta) (string, error) {
	var dest string
	//nolint:staticcheck // QF1002: default branch is branching logic (os.Stat/suffix checks), not a value match — a tagged switch on outPath would be less clear here.
	switch {
	case outPath == "":
		dir := filepath.Join(downloadRoot(), fmt.Sprintf("%d", meta.fileID))
		dest = filepath.Join(dir, meta.name)
	default:
		if fi, err := os.Stat(outPath); err == nil && fi.IsDir() {
			dest = filepath.Join(outPath, meta.name)
		} else if strings.HasSuffix(outPath, string(os.PathSeparator)) {
			dest = filepath.Join(outPath, meta.name)
		} else {
			dest = outPath
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return "", output.Errf("io_error", "cannot create directory: %v", err)
	}
	return uniquePath(dest), nil
}
