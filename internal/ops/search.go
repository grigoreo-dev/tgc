// internal/ops/search.go
// Package ops: unified search (peers + messages) per
// docs/2026-07-23-search-ux-overhaul-design.md.
package ops

import (
	"github.com/grigoreo-dev/tgc/internal/output"
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
