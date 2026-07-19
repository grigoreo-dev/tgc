package ops

import (
	"time"

	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

// MarkRead marks the chat read up to and including maxID. No-op for maxID<=0.
// Best-effort at the call site: callers log failures to stderr but do not fail
// the command (messages are already emitted before MarkRead).
func MarkRead(conn *client.Conn, selector string, maxID int) error {
	if maxID <= 0 {
		return nil
	}
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return err
	}
	// Megagroups resolve as Type=="group" with AccessHash!=0 and, like channels,
	// require channels.readHistory (messages.readHistory rejects them with
	// PEER_ID_INVALID). Basic groups (AccessHash==0) stay on messages.readHistory.
	if peer.Type == "channel" || (peer.Type == "group" && peer.AccessHash != 0) {
		ch, err := resolve.InputChannel(peer)
		if err != nil {
			return err
		}
		_, err = conn.Ctx.Raw.ChannelsReadHistory(conn.Ctx, &tg.ChannelsReadHistoryRequest{
			Channel: ch,
			MaxID:   maxID,
		})
		return client.WrapErr(err)
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return err
	}
	_, err = conn.Ctx.Raw.MessagesReadHistory(conn.Ctx, &tg.MessagesReadHistoryRequest{
		Peer:  ip,
		MaxID: maxID,
	})
	return client.WrapErr(err)
}

// acceptMessage is the filter chain shared by live + drain: inbound, from the
// target peer, optionally from a specific sender (fromID>0). Service messages
// are filtered separately in the live path (via types.Message.IsService).
func acceptMessage(m *tg.Message, target int64, fromID int64) bool {
	if m == nil || m.Out {
		return false
	}
	if resolve.TDLibPeerID(m.PeerID) != target {
		return false
	}
	if fromID > 0 {
		if m.FromID == nil || peerUserID(m.FromID) != fromID {
			return false
		}
	}
	return true
}

// AwaitOpts controls Await: the overall deadline, the quiet-period debounce
// that closes a burst, and an optional sender selector ("" = any sender).
type AwaitOpts struct {
	Timeout  time.Duration
	Debounce time.Duration
	From     string
}

// Await blocks until the target's unread messages arrive (debounced) or the
// deadline fires. Returns (messages oldest→newest, lastID, chatID, timedOut,
// error). lastID is the max message id in the batch (0 when empty) for MarkRead.
// User profiles get a deterministic startup drain of standing unread; bot
// profiles are live-only. The caller owns conn lifecycle (defer Close).
//
// Invariant: Await registers exactly one dispatcher handler and assumes ONE
// call per connection (the process is short-lived: connect → Await → exit).
// Calling Await twice on the same conn is unsupported (would double-register).
func Await(conn *client.Conn, selector string, o AwaitOpts) ([]map[string]any, int, int64, bool, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, 0, 0, false, err
	}
	target := peer.ID

	var fromID int64
	if o.From != "" {
		fp, err := resolve.Resolve(conn, o.From)
		if err != nil {
			return nil, 0, 0, false, err
		}
		// acceptMessage's sender filter only engages when fromID>0. A non-user
		// peer resolves to a negative TDLib id, which would silently disable the
		// filter the user explicitly asked for — reject it instead.
		if fp.Type != "user" {
			return nil, 0, 0, false, output.Errf("bad_args", "--from must be a user selector")
		}
		fromID = fp.ID
	}

	ev := make(chan awaitEvent, 64)

	// Register live handler BEFORE the drain so the windows overlap; collectBatch
	// dedups by id. The gotgproto callback runs on its own goroutine and only
	// sends on ev (non-blocking), so it never contends with the drain or the
	// buffer loop.
	handler := handlers.NewMessage(filters.Message.All, func(_ *ext.Context, u *ext.Update) error {
		tm := u.EffectiveMessage
		if tm == nil || tm.IsService || tm.Message == nil {
			return nil
		}
		raw := tm.Message // embedded *tg.Message
		if !acceptMessage(raw, target, fromID) {
			return nil
		}
		// Best-effort sender enrichment so live output matches read's shape.
		users := map[int64]*tg.User{}
		if su := u.EffectiveUser(); su != nil {
			users[su.ID] = su
		}
		select {
		case ev <- awaitEvent{ID: raw.ID, Msg: messageToMap(raw, users, nil, target)}:
		default:
		}
		return nil
	})
	handler.Outgoing = false
	conn.Client.Dispatcher.AddHandler(handler)

	// Startup drain (user profiles only). Bots have no standing unread window to
	// replay, and messages.getPeerDialogs is a user-layer method. The drained
	// events are returned (NOT pushed through ev) and pre-seeded into
	// collectBatch, so an unbounded standing-unread count can never overflow the
	// bounded ev channel and be silently dropped (which would let MarkRead mark
	// never-emitted messages read).
	var preseed []awaitEvent
	if conn.Profile == nil || conn.Profile.Type != "bot" {
		preseed, err = drainUnread(conn, peer, target, fromID)
		if err != nil {
			return nil, 0, 0, false, err
		}
	}

	deadline := time.After(o.Timeout)
	batch, lastID, timedOut := collectBatch(ev, preseed, o.Debounce, deadline)
	return batch, lastID, target, timedOut, nil
}

// drainUnread returns the standing unread messages for a user peer as await
// events, oldest→newest, already filtered by acceptMessage.
func drainUnread(conn *client.Conn, peer *resolve.Peer, target, fromID int64) ([]awaitEvent, error) {
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	pd, err := conn.Ctx.Raw.MessagesGetPeerDialogs(conn.Ctx, []tg.InputDialogPeerClass{
		&tg.InputDialogPeer{Peer: ip},
	})
	if err != nil {
		return nil, client.WrapErr(err)
	}
	var readMax, unread int
	for _, d := range pd.Dialogs {
		if dd, ok := d.(*tg.Dialog); ok {
			readMax = dd.ReadInboxMaxID
			unread = dd.UnreadCount
		}
	}
	if unread <= 0 {
		return nil, nil // unread_count is only a gate; do not trust it as an exact limit
	}
	// Fixed ceiling, NOT min(unread,100): unread_count may under-count (service
	// events, reactions), and MinID already excludes everything <= readMax, so a
	// larger limit is harmless while a too-small one would drop real unread.
	res, err := conn.Ctx.Raw.MessagesGetHistory(conn.Ctx, &tg.MessagesGetHistoryRequest{
		Peer: ip, MinID: readMax, Limit: 100,
	})
	if err != nil {
		return nil, client.WrapErr(err)
	}
	msgs, users, chats := historyMessages(res) // ascending + user/chat index
	out := make([]awaitEvent, 0, len(msgs))
	for _, mc := range msgs {
		m, ok := mc.(*tg.Message)
		if !ok || m.ID <= readMax {
			continue
		}
		if !acceptMessage(m, target, fromID) {
			continue
		}
		out = append(out, awaitEvent{ID: m.ID, Msg: messageToMap(m, users, chats, target)})
	}
	// The drain is synchronous (unlike the non-blocking live handler), so it may
	// auto-fetch truncated Part rich bodies within the shared budget.
	maps := make([]map[string]any, len(out))
	for i := range out {
		maps[i] = out[i].Msg
	}
	autofetchRichParts(conn, ip, res, maps)
	return out, nil
}

// historyMessages extracts messages ascending (oldest first) plus the user/chat
// indexes from a history response (reuses indexUsers/indexChats from messages.go).
func historyMessages(res tg.MessagesMessagesClass) ([]tg.MessageClass, map[int64]*tg.User, map[int64]tg.ChatClass) {
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
	}
	// getHistory returns newest-first; reverse to oldest-first.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, indexUsers(users), indexChats(chats)
}
