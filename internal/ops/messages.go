package ops

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/markup"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

const richFetchBudget = 10

// ParseDateArg parses a date argument in YYYY-MM-DD or RFC3339 form.
func ParseDateArg(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, output.Errf("bad_args", "cannot parse date %q (want YYYY-MM-DD or RFC3339)", s)
}

// ReadOpts controls Read: paging, id/date windows, sender and search filters.
type ReadOpts struct {
	Limit    int
	BeforeID int
	AfterID  int
	Since    string
	Until    string
	From     string
	Search   string
}

// Read returns chat messages newest-first. Search or From routes through
// messages.search; otherwise messages.getHistory with id/date windows.
func Read(conn *client.Conn, selector string, o ReadOpts) ([]map[string]any, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	if o.Limit == 0 {
		o.Limit = 20
	}

	var sinceT time.Time
	if o.Since != "" {
		if sinceT, err = ParseDateArg(o.Since); err != nil {
			return nil, err
		}
	}

	switch {
	case o.Search != "" || o.From != "":
		req := &tg.MessagesSearchRequest{
			Peer: ip, Q: o.Search, Filter: &tg.InputMessagesFilterEmpty{}, Limit: o.Limit,
		}
		if o.From != "" {
			fromPeer, err := resolve.Resolve(conn, o.From)
			if err != nil {
				return nil, err
			}
			fip, err := resolve.InputPeer(conn, fromPeer)
			if err != nil {
				return nil, err
			}
			req.SetFromID(fip)
		}
		res, err := conn.Ctx.Raw.MessagesSearch(conn.Ctx, req)
		if err != nil {
			return nil, client.WrapErr(err)
		}
		maps := collectMessages(res, peer.ID, sinceT)
		autofetchRichParts(conn, ip, res, maps)
		return maps, nil
	default:
		req := &tg.MessagesGetHistoryRequest{Peer: ip, Limit: o.Limit}
		if o.BeforeID > 0 {
			req.OffsetID = o.BeforeID
			req.MaxID = o.BeforeID
		}
		if o.AfterID > 0 {
			req.MinID = o.AfterID
		}
		if o.Until != "" {
			t, err := ParseDateArg(o.Until)
			if err != nil {
				return nil, err
			}
			req.OffsetDate = int(t.Unix())
		}
		res, err := conn.Ctx.Raw.MessagesGetHistory(conn.Ctx, req)
		if err != nil {
			return nil, client.WrapErr(err)
		}
		maps := collectMessages(res, peer.ID, sinceT)
		autofetchRichParts(conn, ip, res, maps)
		return maps, nil
	}
}

// Context returns a message plus radius messages on each side.
func Context(conn *client.Conn, selector string, msgID, radius int) ([]map[string]any, error) {
	if radius <= 0 {
		radius = 10
	}
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	res, err := conn.Ctx.Raw.MessagesGetHistory(conn.Ctx, &tg.MessagesGetHistoryRequest{
		Peer: ip, OffsetID: msgID + radius + 1, Limit: radius*2 + 1,
	})
	if err != nil {
		return nil, client.WrapErr(err)
	}
	maps := collectMessages(res, peer.ID, time.Time{})
	autofetchRichParts(conn, ip, res, maps)
	return maps, nil
}

// richPartIDs returns the ids of messages whose RichMessage is truncated (Part).
func richPartIDs(res tg.MessagesMessagesClass) []int {
	var msgs []tg.MessageClass
	switch r := res.(type) {
	case *tg.MessagesMessages:
		msgs = r.Messages
	case *tg.MessagesMessagesSlice:
		msgs = r.Messages
	case *tg.MessagesChannelMessages:
		msgs = r.Messages
	default:
		return nil
	}
	var ids []int
	for _, mc := range msgs {
		m, ok := mc.(*tg.Message)
		if !ok {
			continue
		}
		if rm, ok := m.GetRichMessage(); ok && rm.Part {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// autofetchRichParts re-renders truncated (Part) rich messages by fetching the
// full version via messages.getRichMessage, bounded by richFetchBudget and a
// stop-on-FLOOD_WAIT rule. It mutates maps in place (matched by "id").
func autofetchRichParts(conn *client.Conn, ip tg.InputPeerClass, res tg.MessagesMessagesClass, maps []map[string]any) {
	partIDs := richPartIDs(res)
	if len(partIDs) == 0 {
		return
	}
	byID := map[int]map[string]any{}
	for _, mp := range maps {
		if id, ok := mp["id"].(int); ok {
			byID[id] = mp
		}
	}
	spent := 0
	stopped := false
	for _, id := range partIDs {
		mp := byID[id]
		if mp == nil {
			// since-filtered or otherwise absent from maps — do not spend budget
			continue
		}
		if stopped || spent >= richFetchBudget {
			mp["rich_truncated"] = true
			continue
		}
		spent++
		full, err := conn.Ctx.Raw.MessagesGetRichMessage(conn.Ctx, &tg.MessagesGetRichMessageRequest{Peer: ip, ID: id})
		if err != nil {
			mp["rich_truncated"] = true
			output.Warnf("rich_fetch_failed", "id %d: %v", id, err)
			if isFloodWait(err) {
				stopped = true
			}
			continue
		}
		if rm := fullRichMessage(full, id); rm != nil {
			md, truncated := markup.RenderRichMessage(*rm, nil)
			if md != "" {
				mp["text"] = md
				mp["rich"] = true
				if truncated {
					mp["rich_truncated"] = true
				} else {
					delete(mp, "rich_truncated")
				}
			}
		}
	}
}

// fullRichMessage finds the message with id in a getRichMessage response and
// returns its RichMessage.
func fullRichMessage(res tg.MessagesMessagesClass, id int) *tg.RichMessage {
	var msgs []tg.MessageClass
	switch r := res.(type) {
	case *tg.MessagesMessages:
		msgs = r.Messages
	case *tg.MessagesMessagesSlice:
		msgs = r.Messages
	case *tg.MessagesChannelMessages:
		msgs = r.Messages
	default:
		return nil
	}
	for _, mc := range msgs {
		if m, ok := mc.(*tg.Message); ok && m.ID == id {
			if rm, ok := m.GetRichMessage(); ok {
				return &rm
			}
		}
	}
	return nil
}

// isFloodWait reports whether err is a Telegram FLOOD_WAIT.
func isFloodWait(err error) bool {
	return strings.Contains(strings.ToUpper(err.Error()), "FLOOD_WAIT")
}

// collectMessages extracts messages plus their attached users/chats from an
// API response, converts each to a map, and drops messages older than since
// (when set). A chatID of 0 makes each message derive its own chat_id.
func collectMessages(res tg.MessagesMessagesClass, chatID int64, since time.Time) []map[string]any {
	var (
		msgs  []tg.MessageClass
		users []tg.UserClass
		chats []tg.ChatClass
	)
	switch r := res.(type) {
	case *tg.MessagesMessages:
		msgs, users, chats = r.Messages, r.Users, r.Chats
	case *tg.MessagesMessagesSlice:
		msgs, users, chats = r.Messages, r.Users, r.Chats
	case *tg.MessagesChannelMessages:
		msgs, users, chats = r.Messages, r.Users, r.Chats
	default:
		return nil
	}
	userIdx := indexUsers(users)
	chatIdx := indexChats(chats)
	out := make([]map[string]any, 0, len(msgs))
	for _, mc := range msgs {
		m, ok := mc.(*tg.Message)
		if !ok {
			continue
		}
		if !since.IsZero() && time.Unix(int64(m.Date), 0).UTC().Before(since) {
			continue
		}
		cid := chatID
		if cid == 0 {
			cid = peerID(m.PeerID)
		}
		out = append(out, messageToMap(m, userIdx, chatIdx, cid))
	}
	return out
}

// messageToMap renders a message per the tgc field contract. Senders are read
// only from the users/chats attached to the API response (no extra resolves).
func messageToMap(m *tg.Message, users map[int64]*tg.User, chats map[int64]tg.ChatClass, chatID int64) map[string]any {
	out := map[string]any{
		"id":              m.ID,
		"chat_id":         chatID,
		"date":            time.Unix(int64(m.Date), 0).UTC().Format(time.RFC3339),
		"text":            m.Message,
		"reply_to":        nil,
		"media":           nil,
		"edited":          false,
		"fwd_from":        nil,
		"sender_id":       nil,
		"sender_name":     nil,
		"sender_username": nil,
	}

	if m.FromID != nil {
		if uid := peerUserID(m.FromID); uid != 0 {
			out["sender_id"] = uid
			if u, ok := users[uid]; ok {
				if name := userName(u); name != "" {
					out["sender_name"] = name
				}
				if u.Username != "" {
					out["sender_username"] = u.Username
				}
			}
		}
	}

	if m.ReplyTo != nil {
		if h, ok := m.ReplyTo.(*tg.MessageReplyHeader); ok && h.ReplyToMsgID != 0 {
			out["reply_to"] = h.ReplyToMsgID
		}
	}

	if m.EditDate != 0 {
		out["edited"] = true
	}

	if m.FwdFrom.FromID != nil || m.FwdFrom.FromName != "" {
		if name := fwdFromName(m.FwdFrom, users); name != "" {
			out["fwd_from"] = name
		}
	}

	if m.Media != nil {
		if mm := mediaToMap(m.Media); mm != nil {
			out["media"] = mm
		}
	}

	// rich:true means text contains a rich render. Photos/Documents on RichMessage
	// are a media reference pool for blocks, not standalone content — renderable
	// rich content always has Blocks.
	if rm, ok := m.GetRichMessage(); ok && !rm.Zero() && len(rm.Blocks) > 0 {
		md, truncated := markup.RenderRichMessage(rm, richResolveMap(users))
		if md != "" {
			if m.Message == "" {
				out["text"] = md
			} else {
				out["text"] = m.Message + "\n\n" + md
			}
			out["rich"] = true
			// rm.Part means the inline copy is itself truncated (the full body
			// needs a getRichMessage fetch). Flag it here so no-fetch paths
			// (await live handler, global search) never silently emit a partial
			// rich body; autofetchRichParts clears the flag on a successful
			// full re-render.
			if truncated || rm.Part {
				out["rich_truncated"] = true
			}
		}
	}

	// grouped_id ties together the members of an album / media group. Emit it
	// only when the message is actually grouped so ordinary messages stay
	// uncluttered; callers reassemble albums via jq group_by(.grouped_id).
	if gid, ok := m.GetGroupedID(); ok {
		out["grouped_id"] = gid
	}

	return out
}

// richResolveMap builds a user_id -> display-name map for rich mention rendering
// from the response-attached users (no extra resolves).
func richResolveMap(users map[int64]*tg.User) map[int64]string {
	if len(users) == 0 {
		return nil
	}
	m := make(map[int64]string, len(users))
	for id, u := range users {
		if name := userName(u); name != "" {
			m[id] = name
		} else if u.Username != "" {
			m[id] = "@" + u.Username
		}
	}
	return m
}

func mediaToMap(media tg.MessageMediaClass) map[string]any {
	switch mv := media.(type) {
	case *tg.MessageMediaPhoto:
		return map[string]any{"type": "photo"}
	case *tg.MessageMediaDocument:
		doc, ok := mv.Document.(*tg.Document)
		if !ok {
			return map[string]any{"type": "document"}
		}
		out := map[string]any{
			"type": "document",
			"size": doc.Size,
			"mime": doc.MimeType,
		}
		for _, attr := range doc.Attributes {
			switch a := attr.(type) {
			case *tg.DocumentAttributeVideo:
				out["type"] = "video"
			case *tg.DocumentAttributeAudio:
				if a.Voice {
					out["type"] = "voice"
				} else {
					out["type"] = "audio"
				}
			case *tg.DocumentAttributeFilename:
				out["file_name"] = a.FileName
			}
		}
		return out
	}
	return nil
}

func fwdFromName(fwd tg.MessageFwdHeader, users map[int64]*tg.User) string {
	if fwd.FromName != "" {
		return fwd.FromName
	}
	if fwd.FromID != nil {
		if uid := peerUserID(fwd.FromID); uid != 0 {
			if u, ok := users[uid]; ok {
				if name := userName(u); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

func userName(u *tg.User) string {
	name := u.FirstName
	if u.LastName != "" {
		if name != "" {
			name += " "
		}
		name += u.LastName
	}
	return name
}

func indexChats(chats []tg.ChatClass) map[int64]tg.ChatClass {
	m := make(map[int64]tg.ChatClass, len(chats))
	for _, c := range chats {
		m[c.GetID()] = c
	}
	return m
}

func peerID(peer tg.PeerClass) int64 {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChat:
		return p.ChatID
	case *tg.PeerChannel:
		return p.ChannelID
	}
	return 0
}

// SendOpts controls SendText: reply target, Markdown vs plain parsing, and an
// optional expert --rich payload (JSON) sent via sendMessage.rich_message.
type SendOpts struct {
	ReplyTo  int
	Plain    bool
	RichJSON string
}

// parseText converts message text to body + entities, honoring the plain flag.
func parseText(text string, plain bool) (string, []tg.MessageEntityClass, error) {
	if plain {
		return markup.ParsePlain(text)
	}
	return markup.Parse(text)
}

// randomID returns a non-zero cryptographically random int64 for message
// deduplication (Telegram's random_id).
func randomID() int64 {
	var b [8]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			// crypto/rand should not fail; fall back to a time-derived seed.
			return time.Now().UnixNano() | 1
		}
		id := int64(binary.LittleEndian.Uint64(b[:]))
		if id != 0 {
			return id
		}
	}
}

// sentResult extracts message_id + date from a send/edit/forward update,
// pairing them with the given chat id in the tgc result shape.
func sentResult(upd tg.UpdatesClass, chatID int64) map[string]any {
	res := map[string]any{
		"message_id": nil,
		"chat_id":    chatID,
		"date":       nil,
	}
	switch u := upd.(type) {
	case *tg.UpdateShortSentMessage:
		res["message_id"] = u.ID
		res["date"] = time.Unix(int64(u.Date), 0).UTC().Format(time.RFC3339)
	case *tg.Updates:
		for _, up := range u.Updates {
			var mc tg.MessageClass
			switch nu := up.(type) {
			case *tg.UpdateNewMessage:
				mc = nu.Message
			case *tg.UpdateNewChannelMessage:
				mc = nu.Message
			case *tg.UpdateEditMessage:
				mc = nu.Message
			case *tg.UpdateEditChannelMessage:
				mc = nu.Message
			default:
				continue
			}
			if m, ok := mc.(*tg.Message); ok {
				res["message_id"] = m.ID
				res["date"] = time.Unix(int64(m.Date), 0).UTC().Format(time.RFC3339)
				break
			}
		}
	}
	return res
}

// useRichPath reports whether a send should attempt the rich_message path.
//
// It is used for the default (non-plain) path only, and bots must skip it:
// sendMessage.rich_message (InputRichMessageMarkdown) is a user/Premium-only
// feature. When a bot sends it, Telegram accepts the RPC WITHOUT error but
// silently drops the payload, delivering an empty-text message. Routing bots to
// the entities path instead carries their text correctly.
func useRichPath(plain bool, p *config.Profile) bool {
	if plain {
		return false
	}
	return p == nil || p.Type != "bot"
}

// SendText sends a text message to selector, applying Markdown unless plain.
func SendText(conn *client.Conn, selector, text string, o SendOpts) (map[string]any, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	body, entities, err := parseText(text, o.Plain)
	if err != nil {
		return nil, err
	}

	// setReplyTo applies the reply target to a request when requested.
	setReplyTo := func(req *tg.MessagesSendMessageRequest) {
		if o.ReplyTo > 0 {
			req.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: o.ReplyTo})
		}
	}

	// Path 1: explicit --rich payload. This is an expert request, so an RPC
	// failure surfaces to the caller — no silent entities fallback.
	if o.RichJSON != "" {
		rm, err := markup.ParseRichJSON(json.RawMessage(o.RichJSON))
		if err != nil {
			return nil, err
		}
		req := &tg.MessagesSendMessageRequest{
			Peer:     ip,
			Message:  body,
			RandomID: randomID(),
		}
		req.SetRichMessage(rm)
		setReplyTo(req)
		upd, err := conn.Ctx.Raw.MessagesSendMessage(conn.Ctx, req)
		if err != nil {
			return nil, client.WrapErr(err)
		}
		return sentResult(upd, peer.ID), nil
	}

	// Path 2: default (non-plain) — attempt rich_message, then transparently
	// fall back to the Task 6 entities path once if the user-layer rejects it.
	// Bots skip rich entirely (see useRichPath).
	if useRichPath(o.Plain, conn.Profile) {
		req := &tg.MessagesSendMessageRequest{
			Peer:     ip,
			Message:  body,
			RandomID: randomID(),
		}
		// InputRichMessageMarkdown expects the RAW Markdown source; parseText's
		// body has already been rendered/stripped, so use the original text.
		req.SetRichMessage(markup.TryRichMarkdown(text))
		setReplyTo(req)
		if upd, err := conn.Ctx.Raw.MessagesSendMessage(conn.Ctx, req); err == nil {
			return sentResult(upd, peer.ID), nil
		}
		// Rich rejected: retry once with the plain entities path, no error surfaced.
	}

	// Path 3 (and rich fallback): entities path — today's behavior.
	req := &tg.MessagesSendMessageRequest{
		Peer:     ip,
		Message:  body,
		RandomID: randomID(),
	}
	if len(entities) > 0 {
		req.SetEntities(entities)
	}
	setReplyTo(req)
	upd, err := conn.Ctx.Raw.MessagesSendMessage(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	return sentResult(upd, peer.ID), nil
}

// EditText replaces the text of an existing message.
func EditText(conn *client.Conn, selector string, msgID int, text string, plain bool) (map[string]any, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	body, entities, err := parseText(text, plain)
	if err != nil {
		return nil, err
	}
	req := &tg.MessagesEditMessageRequest{
		Peer:    ip,
		ID:      msgID,
		Message: body,
	}
	if len(entities) > 0 {
		req.SetEntities(entities)
	}
	upd, err := conn.Ctx.Raw.MessagesEditMessage(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	return sentResult(upd, peer.ID), nil
}

// Delete removes messages. For channels it uses channels.deleteMessages (no
// per-user option); elsewhere messages.deleteMessages with revoke = !forMe.
func Delete(conn *client.Conn, selector string, ids []int, forMe bool) (map[string]any, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	if peer.Type == "channel" {
		if forMe {
			return nil, output.Errf("bad_args", "channels cannot delete messages only for you")
		}
		ch, err := resolve.InputChannel(peer)
		if err != nil {
			return nil, err
		}
		if _, err := conn.Ctx.Raw.ChannelsDeleteMessages(conn.Ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: ch,
			ID:      ids,
		}); err != nil {
			return nil, client.WrapErr(err)
		}
		return map[string]any{"status": "ok", "deleted": len(ids)}, nil
	}
	if _, err := conn.Ctx.Raw.MessagesDeleteMessages(conn.Ctx, &tg.MessagesDeleteMessagesRequest{
		Revoke: !forMe,
		ID:     ids,
	}); err != nil {
		return nil, client.WrapErr(err)
	}
	return map[string]any{"status": "ok", "deleted": len(ids)}, nil
}

// Forward copies a single message from fromSel into toSel.
func Forward(conn *client.Conn, fromSel string, msgID int, toSel string) (map[string]any, error) {
	fromPeer, err := resolve.Resolve(conn, fromSel)
	if err != nil {
		return nil, err
	}
	fromIP, err := resolve.InputPeer(conn, fromPeer)
	if err != nil {
		return nil, err
	}
	toPeer, err := resolve.Resolve(conn, toSel)
	if err != nil {
		return nil, err
	}
	toIP, err := resolve.InputPeer(conn, toPeer)
	if err != nil {
		return nil, err
	}
	upd, err := conn.Ctx.Raw.MessagesForwardMessages(conn.Ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: fromIP,
		ToPeer:   toIP,
		ID:       []int{msgID},
		RandomID: []int64{randomID()},
	})
	if err != nil {
		return nil, client.WrapErr(err)
	}
	return sentResult(upd, toPeer.ID), nil
}
