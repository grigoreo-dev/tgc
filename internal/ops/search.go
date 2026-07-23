// internal/ops/search.go
// Package ops: unified search (peers + messages) per
// docs/2026-07-23-search-ux-overhaul-design.md.
package ops

import (
	"time"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

// SearchOpts controls the unified search command.
type SearchOpts struct {
	Type  string // "", "chats", "messages", "user", "group", "channel"
	Chat  string // in-chat peer selector; switches to messages.search mode
	From  string // sender selector; requires Chat
	Since string // YYYY-MM-DD or RFC3339
	Until string
	Limit int // per-section cap; 0 => 20
}

func searchTypeValid(t string) bool {
	switch t {
	case "", "chats", "messages", "user", "group", "channel":
		return true
	}
	return false
}

// ValidateSearchOpts enforces the flag compatibility matrix.
func ValidateSearchOpts(o SearchOpts) error {
	if !searchTypeValid(o.Type) {
		return output.Errf("bad_args",
			"invalid --type %q; valid values: chats|messages|user|group|channel", o.Type)
	}
	if o.Type != "" && o.Chat != "" {
		return output.Errf("bad_args",
			"--type applies to global search only; drop --type when using --chat")
	}
	if o.From != "" && o.Chat == "" {
		return output.Errf("bad_args",
			"--from requires --chat; global search cannot filter by sender")
	}
	if o.Type == "chats" && (o.Since != "" || o.Until != "") {
		return output.Errf("bad_args",
			"--since/--until apply to message search; drop them with --type chats")
	}
	// Fail hard pre-connect on unparseable dates (all modes).
	if o.Since != "" {
		if _, err := ParseDateArg(o.Since); err != nil {
			return err
		}
	}
	if o.Until != "" {
		if _, err := ParseDateArg(o.Until); err != nil {
			return err
		}
	}
	return nil
}

// peerRow projects a Peer into a tagged search-output row (no AccessHash).
func peerRow(p resolve.Peer) map[string]any {
	row := map[string]any{
		"result": "chat",
		"id":     p.ID,
		"type":   p.Type,
		"title":  p.Title,
	}
	if p.Username != "" {
		row["username"] = p.Username
	}
	return row
}

// filterPeersByKind keeps peers of the given kind; empty kind keeps all.
func filterPeersByKind(peers []resolve.Peer, kind string) []resolve.Peer {
	if kind == "" {
		return peers
	}
	out := make([]resolve.Peer, 0, len(peers))
	for _, p := range peers {
		if p.Type == kind {
			out = append(out, p)
		}
	}
	return out
}

// applyGlobalKind maps a --type chat-kind onto SearchGlobal only-flags.
func applyGlobalKind(req *tg.MessagesSearchGlobalRequest, kind string) {
	switch kind {
	case "user":
		req.UsersOnly = true
	case "group":
		req.GroupsOnly = true
	case "channel":
		req.BroadcastsOnly = true
	}
}

// searchSections reports which sections a --type selects.
func searchSections(o SearchOpts) (wantChats, wantMessages bool) {
	switch o.Type {
	case "chats":
		return true, false
	case "messages":
		return false, true
	default: // "", user, group, channel
		return true, true
	}
}

// peerKindFilter returns the chat-kind filter for the peers section.
func peerKindFilter(o SearchOpts) string {
	switch o.Type {
	case "user", "group", "channel":
		return o.Type
	}
	return ""
}

// searchInChat searches messages inside one chat via messages.search.
func searchInChat(conn *client.Conn, query string, o SearchOpts) ([]map[string]any, error) {
	peer, err := resolve.Resolve(conn, o.Chat)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	limit := o.Limit
	if limit == 0 {
		limit = 20
	}
	req := &tg.MessagesSearchRequest{
		Peer: ip, Q: query, Filter: &tg.InputMessagesFilterEmpty{}, Limit: limit,
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
	if o.Since != "" {
		t, err := ParseDateArg(o.Since)
		if err != nil {
			return nil, err
		}
		req.MinDate = int(t.Unix())
	}
	if o.Until != "" {
		t, err := ParseDateArg(o.Until)
		if err != nil {
			return nil, err
		}
		req.MaxDate = int(t.Unix())
	}
	res, err := conn.Ctx.Raw.MessagesSearch(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	maps := collectMessages(res, peer.ID, time.Time{})
	autofetchRichParts(conn, ip, res, maps)
	stripRichMapKeys(maps)
	for _, m := range maps {
		m["result"] = "message"
	}
	return maps, nil
}

// Search is the unified entry point: peers + messages by default, one
// section via --type, or in-chat via --chat.
func Search(conn *client.Conn, query string, o SearchOpts) ([]map[string]any, error) {
	if err := ValidateSearchOpts(o); err != nil {
		return nil, err
	}
	if o.Chat != "" {
		return searchInChat(conn, query, o)
	}
	wantChats, wantMessages := searchSections(o)
	var rows []map[string]any
	var chatErr, msgErr error
	if wantChats {
		peers, err := SearchChats(conn, query, peerKindFilter(o), o.Limit)
		if err != nil {
			chatErr = err
		} else {
			for _, p := range peers {
				rows = append(rows, peerRow(p))
			}
		}
	}
	if wantMessages {
		maps, err := SearchMessages(conn, query, o)
		if err != nil {
			msgErr = err
		} else {
			rows = append(rows, maps...)
		}
	}
	// Single-section modes fail hard; dual mode degrades with a warning.
	if wantChats && !wantMessages && chatErr != nil {
		return nil, chatErr
	}
	if wantMessages && !wantChats && msgErr != nil {
		return nil, msgErr
	}
	if chatErr != nil && msgErr != nil {
		return nil, msgErr
	}
	if chatErr != nil {
		output.Warnf("search_partial", "chat section failed: %v", chatErr)
	}
	if msgErr != nil {
		output.Warnf("search_partial", "message section failed: %v", msgErr)
	}
	return rows, nil
}
