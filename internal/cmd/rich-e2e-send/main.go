// Command rich-e2e-send uploads demo media and sends the All Types rich fixture.
//
// Stdout is a single compact JSON line:
//
//	{"message_id":N,"chat_id":N,"blocks":37}
//
// Diagnostics go to stderr. Any failure exits 1.
package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/resolve"
	"github.com/grigoreo-dev/tgc/internal/richfixture"
)

type options struct {
	Profile  string
	Target   string
	Fixture  string
	MediaDir string
}

func parseOptions(args []string) (options, error) {
	fs := flag.NewFlagSet("rich-e2e-send", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var o options
	fs.StringVar(&o.Profile, "profile", "e2ebot", "tgc profile name")
	fs.StringVar(&o.Target, "target", "", "chat selector (required)")
	fs.StringVar(&o.Fixture, "fixture", "internal/markup/testdata/richmessage_alltypes.bin", "path to richmessage fixture")
	fs.StringVar(&o.MediaDir, "media-dir", "/tmp/demo_media", "directory with demo media files")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if o.Target == "" {
		return options{}, fmt.Errorf("missing required --target")
	}
	return o, nil
}

// requiredMediaFiles are exact basenames expected under --media-dir.
var requiredMediaFiles = []string{
	"photo_0.jpg",
	"photo_1.jpg",
	"photo_2.jpg",
	"dubaiVideo.mp4",
	"Neon Rain Train.mp3",
}

func mediaPaths(dir string) (map[string]string, error) {
	out := make(map[string]string, len(requiredMediaFiles))
	for _, name := range requiredMediaFiles {
		p := filepath.Join(dir, name)
		st, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("media file %s: %w", name, err)
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("media file %s: not a regular file", name)
		}
		if st.Size() == 0 {
			return nil, fmt.Errorf("media file %s: empty (zero bytes)", name)
		}
		out[name] = p
	}
	return out, nil
}

func sentMessageID(upd tg.UpdatesClass) (int, error) {
	if upd == nil {
		return 0, fmt.Errorf("send succeeded but updates were nil")
	}
	switch u := upd.(type) {
	case *tg.UpdateShortSentMessage:
		if u.ID == 0 {
			return 0, fmt.Errorf("send succeeded without message id (%T)", upd)
		}
		return u.ID, nil
	case *tg.Updates:
		if id, ok := messageIDFromUpdates(u.Updates); ok {
			return id, nil
		}
		return 0, fmt.Errorf("send succeeded without message id (%T)", upd)
	case *tg.UpdatesCombined:
		// Telegram may return updatesCombined#725b04c3 after a successful send.
		// Treat it like updates: walk nested UpdateNewMessage / UpdateNewChannelMessage.
		if id, ok := messageIDFromUpdates(u.Updates); ok {
			return id, nil
		}
		return 0, fmt.Errorf("send succeeded without message id (%T)", upd)
	default:
		return 0, fmt.Errorf("send succeeded without message id (%T)", upd)
	}
}

func messageIDFromUpdates(updates []tg.UpdateClass) (int, bool) {
	for _, up := range updates {
		var mc tg.MessageClass
		switch nu := up.(type) {
		case *tg.UpdateNewMessage:
			mc = nu.Message
		case *tg.UpdateNewChannelMessage:
			mc = nu.Message
		default:
			continue
		}
		if m, ok := mc.(*tg.Message); ok && m.ID != 0 {
			return m.ID, true
		}
	}
	return 0, false
}

func sendRequest(ip tg.InputPeerClass, rm tg.InputRichMessageClass) *tg.MessagesSendMessageRequest {
	req := &tg.MessagesSendMessageRequest{
		Peer:     ip,
		Message:  "",
		RandomID: randomID(),
	}
	req.SetRichMessage(rm)
	return req
}

func randomID() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UnixNano() | 1
	}
	id := int64(binary.LittleEndian.Uint64(b[:]))
	if id == 0 {
		id = 1
	}
	return id
}

func uploadPhoto(conn *client.Conn, ip tg.InputPeerClass, path string) (*tg.InputPhoto, error) {
	up := uploader.NewUploader(conn.Ctx.Raw)
	f, err := up.FromPath(conn.Ctx, path)
	if err != nil {
		return nil, fmt.Errorf("upload %s: %w", filepath.Base(path), err)
	}
	ready, err := conn.Ctx.Raw.MessagesUploadMedia(conn.Ctx, &tg.MessagesUploadMediaRequest{
		Peer:  ip,
		Media: &tg.InputMediaUploadedPhoto{File: f},
	})
	if err != nil {
		return nil, fmt.Errorf("upload media %s: %w", filepath.Base(path), err)
	}
	mp, ok := ready.(*tg.MessageMediaPhoto)
	if !ok {
		return nil, fmt.Errorf("upload %s: not a photo: %T", filepath.Base(path), ready)
	}
	ph, ok := mp.Photo.(*tg.Photo)
	if !ok {
		return nil, fmt.Errorf("upload %s: photo unavailable", filepath.Base(path))
	}
	return &tg.InputPhoto{ID: ph.ID, AccessHash: ph.AccessHash, FileReference: ph.FileReference}, nil
}

func uploadDocument(conn *client.Conn, ip tg.InputPeerClass, path, mime string, attrs []tg.DocumentAttributeClass) (*tg.InputDocument, error) {
	up := uploader.NewUploader(conn.Ctx.Raw)
	f, err := up.FromPath(conn.Ctx, path)
	if err != nil {
		return nil, fmt.Errorf("upload %s: %w", filepath.Base(path), err)
	}
	// Always include filename; caller supplies media-type attributes.
	allAttrs := append([]tg.DocumentAttributeClass{
		&tg.DocumentAttributeFilename{FileName: filepath.Base(path)},
	}, attrs...)
	ready, err := conn.Ctx.Raw.MessagesUploadMedia(conn.Ctx, &tg.MessagesUploadMediaRequest{
		Peer: ip,
		Media: &tg.InputMediaUploadedDocument{
			File:       f,
			MimeType:   mime,
			Attributes: allAttrs,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("upload media %s: %w", filepath.Base(path), err)
	}
	md, ok := ready.(*tg.MessageMediaDocument)
	if !ok {
		return nil, fmt.Errorf("upload %s: not a document: %T", filepath.Base(path), ready)
	}
	doc, ok := md.Document.(*tg.Document)
	if !ok {
		return nil, fmt.Errorf("upload %s: document unavailable", filepath.Base(path))
	}
	return &tg.InputDocument{ID: doc.ID, AccessHash: doc.AccessHash, FileReference: doc.FileReference}, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rich-e2e-send:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	// Validate media before any network work.
	paths, err := mediaPaths(opts.MediaDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "rich-e2e-send: media preflight ok (%d files)\n", len(paths))

	conn, err := client.Connect(opts.Profile)
	if err != nil {
		return err
	}
	defer conn.Close()

	peer, err := resolve.Resolve(conn, opts.Target)
	if err != nil {
		return err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "rich-e2e-send: uploading media…")
	mainPhoto, err := uploadPhoto(conn, ip, paths["photo_0.jpg"])
	if err != nil {
		return fmt.Errorf("photo main: %w", err)
	}
	collageA, err := uploadPhoto(conn, ip, paths["photo_1.jpg"])
	if err != nil {
		return fmt.Errorf("photo collage A: %w", err)
	}
	collageB, err := uploadPhoto(conn, ip, paths["photo_2.jpg"])
	if err != nil {
		return fmt.Errorf("photo collage B: %w", err)
	}
	video, err := uploadDocument(conn, ip, paths["dubaiVideo.mp4"], "video/mp4", []tg.DocumentAttributeClass{
		&tg.DocumentAttributeVideo{SupportsStreaming: true, W: 1088, H: 1088, Duration: 5.062},
	})
	if err != nil {
		return fmt.Errorf("video: %w", err)
	}
	audio, err := uploadDocument(conn, ip, paths["Neon Rain Train.mp3"], "audio/mpeg", []tg.DocumentAttributeClass{
		&tg.DocumentAttributeAudio{Duration: 224, Title: "Neon Rain Train", Performer: "alphavano"},
	})
	if err != nil {
		return fmt.Errorf("audio: %w", err)
	}

	rm, err := richfixture.Decode(opts.Fixture)
	if err != nil {
		return err
	}
	prepared, err := richfixture.PrepareForSend(rm, richfixture.UploadedMedia{
		MainPhoto:     mainPhoto,
		CollagePhotoA: collageA,
		CollagePhotoB: collageB,
		Video:         video,
		Audio:         audio,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "rich-e2e-send: sending %d blocks to %s…\n", len(prepared.Blocks), opts.Target)
	upd, err := conn.Ctx.Raw.MessagesSendMessage(conn.Ctx, sendRequest(ip, prepared))
	if err != nil {
		return fmt.Errorf("MessagesSendMessage: %w", err)
	}
	msgID, err := sentMessageID(upd)
	if err != nil {
		return err
	}

	result := struct {
		MessageID int   `json:"message_id"`
		ChatID    int64 `json:"chat_id"`
		Blocks    int   `json:"blocks"`
	}{
		MessageID: msgID,
		ChatID:    peer.ID,
		Blocks:    len(prepared.Blocks),
	}
	enc := json.NewEncoder(os.Stdout)
	// Compact single line (Encoder default is compact; no indent).
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	return nil
}
