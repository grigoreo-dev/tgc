package cli

import (
	"os"
	"strings"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/gotd/td/tg"
)

// readSessionInput loads a session string from args[0] (file path), TGC_SESSION, or stdin.
func readSessionInput(args []string) (string, error) {
	var raw []byte
	var err error
	switch {
	case len(args) == 1:
		raw, err = os.ReadFile(args[0])
	case os.Getenv("TGC_SESSION") != "":
		raw = []byte(os.Getenv("TGC_SESSION"))
	default:
		raw, err = os.ReadFile("/dev/stdin")
	}
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return "", output.Errf("bad_args", "empty session string")
	}
	return s, nil
}

// selfUsername returns the primary username for a logged-in user, if any.
func selfUsername(u *tg.User) string {
	if u == nil {
		return ""
	}
	if u.Username != "" {
		return u.Username
	}
	for _, un := range u.Usernames {
		if un.Username != "" {
			return un.Username
		}
	}
	return ""
}
