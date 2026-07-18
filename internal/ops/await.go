package ops

import (
	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
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
	if peer.Type == "channel" {
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
