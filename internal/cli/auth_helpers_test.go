package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/grigoreo-dev/tgc/internal/output"
)

func TestReadSessionInputFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.txt")
	if err := os.WriteFile(path, []byte("  abcDEF  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TGC_SESSION", "")
	got, err := readSessionInput([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if got != "abcDEF" {
		t.Fatalf("want trimmed session, got %q", got)
	}
}

func TestReadSessionInputFromEnv(t *testing.T) {
	t.Setenv("TGC_SESSION", "env-session-token")
	got, err := readSessionInput(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "env-session-token" {
		t.Fatalf("got %q", got)
	}
}

func TestReadSessionInputEmptyFileIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte("\n\t"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TGC_SESSION", "")
	_, err := readSessionInput([]string{path})
	if err == nil {
		t.Fatal("want error for empty session string")
	}
	var e *output.Error
	if !output.AsError(err, &e) || e.Code != "bad_args" {
		t.Fatalf("want bad_args structured error, got %v", err)
	}
}

func TestSelfUsername(t *testing.T) {
	if selfUsername(nil) != "" {
		t.Fatal("nil user")
	}
	u := &tg.User{Username: "alice"}
	if selfUsername(u) != "alice" {
		t.Fatalf("got %q", selfUsername(u))
	}
	u2 := &tg.User{Usernames: []tg.Username{{Username: "secondary"}}}
	if selfUsername(u2) != "secondary" {
		t.Fatalf("got %q", selfUsername(u2))
	}
}
