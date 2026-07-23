// internal/ops/search_test.go
package ops

import (
	"testing"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/resolve"
)

func TestValidateSearchOpts(t *testing.T) {
	cases := []struct {
		name    string
		o       SearchOpts
		wantErr bool
	}{
		{"empty ok", SearchOpts{}, false},
		{"type chats ok", SearchOpts{Type: "chats"}, false},
		{"type messages ok", SearchOpts{Type: "messages"}, false},
		{"type user ok", SearchOpts{Type: "user"}, false},
		{"type group ok", SearchOpts{Type: "group"}, false},
		{"type channel ok", SearchOpts{Type: "channel"}, false},
		{"chat ok", SearchOpts{Chat: "@devops"}, false},
		{"chat+from ok", SearchOpts{Chat: "@devops", From: "@vasya"}, false},
		{"type with chat rejected", SearchOpts{Type: "messages", Chat: "@devops"}, true},
		{"from without chat rejected", SearchOpts{From: "@vasya"}, true},
		{"unknown type rejected", SearchOpts{Type: "posts"}, true},
		{"valid since ok", SearchOpts{Since: "2024-01-15"}, false},
		{"garbage since rejected", SearchOpts{Since: "garbage"}, true},
		{"garbage until rejected", SearchOpts{Until: "not-a-date"}, true},
		{"type chats+since rejected", SearchOpts{Type: "chats", Since: "2024-01-15"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateSearchOpts(c.o)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateSearchOpts(%+v) err=%v, wantErr=%v", c.o, err, c.wantErr)
			}
		})
	}
}

func TestPeerRow(t *testing.T) {
	p := resolve.Peer{ID: 42, AccessHash: 999, Type: "user", Title: "Anna", Username: "anna"}
	row := peerRow(p)
	if row["result"] != "chat" || row["id"] != int64(42) || row["type"] != "user" || row["title"] != "Anna" || row["username"] != "anna" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if _, leaked := row["access_hash"]; leaked {
		t.Fatal("AccessHash leaked into search output")
	}
	if _, ok := peerRow(resolve.Peer{ID: 1, Type: "group", Title: "G"})["username"]; ok {
		t.Fatal("empty username must be omitted")
	}
}

func TestFilterPeersByKind(t *testing.T) {
	in := []resolve.Peer{{ID: 1, Type: "user"}, {ID: 2, Type: "group"}, {ID: 3, Type: "channel"}}
	got := filterPeersByKind(in, "group")
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("want only group peer, got %+v", got)
	}
	if len(filterPeersByKind(in, "")) != 3 {
		t.Fatal("empty kind must keep all")
	}
}

func TestApplyGlobalKind(t *testing.T) {
	cases := []struct {
		kind                    string
		users, groups, channels bool
	}{
		{"", false, false, false},
		{"chats", false, false, false},
		{"messages", false, false, false},
		{"user", true, false, false},
		{"group", false, true, false},
		{"channel", false, false, true},
	}
	for _, c := range cases {
		req := &tg.MessagesSearchGlobalRequest{}
		applyGlobalKind(req, c.kind)
		if req.UsersOnly != c.users || req.GroupsOnly != c.groups || req.BroadcastsOnly != c.channels {
			t.Fatalf("kind %q: got users=%v groups=%v broadcasts=%v",
				c.kind, req.UsersOnly, req.GroupsOnly, req.BroadcastsOnly)
		}
	}
}

func TestSearchSections(t *testing.T) {
	cases := []struct {
		typ            string
		chats, message bool
	}{
		{"", true, true},
		{"chats", true, false},
		{"messages", false, true},
		{"user", true, true},
		{"group", true, true},
		{"channel", true, true},
	}
	for _, c := range cases {
		gc, gm := searchSections(SearchOpts{Type: c.typ})
		if gc != c.chats || gm != c.message {
			t.Fatalf("type %q: got (%v,%v), want (%v,%v)", c.typ, gc, gm, c.chats, c.message)
		}
	}
}
