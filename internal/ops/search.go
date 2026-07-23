// internal/ops/search.go
// Package ops: unified search (peers + messages) per
// docs/2026-07-23-search-ux-overhaul-design.md.
package ops

import (
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
