package ops

import (
	"time"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

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
		return collectMessages(res, peer.ID, sinceT), nil
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
		return collectMessages(res, peer.ID, sinceT), nil
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
	return collectMessages(res, peer.ID, time.Time{}), nil
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

	return out
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
