// internal/ops/search_test.go
package ops

import "testing"

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
