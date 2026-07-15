package client

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gotd/td/tgerr"

	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
)

// RICH_MESSAGE_UNSUPPORTED must map to a structured rich_unsupported error,
// not leak as a raw "internal" RPC string.
func TestWrapErrRichUnsupported(t *testing.T) {
	err := WrapErr(tgerr.New(400, "RICH_MESSAGE_UNSUPPORTED"))
	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("want *output.Error, got %T: %v", err, err)
	}
	if oe.Code != "rich_unsupported" {
		t.Fatalf("code = %q, want rich_unsupported", oe.Code)
	}
	if strings.Contains(oe.Message, "rpcDoRequest") || strings.Contains(oe.Message, "RICH_MESSAGE_UNSUPPORTED") {
		t.Fatalf("message leaks raw RPC text: %q", oe.Message)
	}
}

// WrapErr passes through unmapped errors unchanged and leaves nil as nil.
func TestWrapErrPassthroughAndNil(t *testing.T) {
	if WrapErr(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	orig := errors.New("some transport failure")
	if got := WrapErr(orig); got != orig {
		t.Fatalf("unmapped error must pass through unchanged, got %v", got)
	}
}

// When stdin is not a TTY (piped input, e.g. automation or tests), secret
// prompts must fall back to a normal buffered read so the flow still works.
func TestReadAnswerSecretNonTTYFallback(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	c := &terminalConversator{in: bufio.NewReader(strings.NewReader("hunter2\n"))}
	got, err := c.readAnswer("2FA password: ", true /*secret*/, false /*isTTY*/)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hunter2" {
		t.Fatalf("got %q, want %q", got, "hunter2")
	}
}

// Non-secret prompts read from the buffered reader and trim whitespace.
func TestReadAnswerNonSecret(t *testing.T) {
	c := &terminalConversator{in: bufio.NewReader(strings.NewReader("  +79990000000  \n"))}
	got, err := c.readAnswer("Phone: ", false /*secret*/, false /*isTTY*/)
	if err != nil {
		t.Fatal(err)
	}
	if got != "+79990000000" {
		t.Fatalf("got %q, want %q", got, "+79990000000")
	}
}

// resetSession must remove the profile's sqlite session and its WAL/SHM
// sidecar files so a fresh auth flow starts clean (the AUTH_RESTART cure).
func TestResetSessionRemovesSQLiteFiles(t *testing.T) {
	dir := t.TempDir()
	p := &config.Profile{Name: "default", Dir: dir, SessionPath: filepath.Join(dir, "session.db")}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.WriteFile(p.SessionPath+suffix, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := resetSession(p); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(p.SessionPath + suffix); !os.IsNotExist(err) {
			t.Fatalf("%s still present after reset (err=%v)", p.SessionPath+suffix, err)
		}
	}
}

// resetSession must also drop an imported string session so a fresh login
// does not keep replaying stale credentials.
func TestResetSessionRemovesStringSession(t *testing.T) {
	dir := t.TempDir()
	p := &config.Profile{Name: "default", Dir: dir, SessionPath: filepath.Join(dir, "session.db")}
	txt := filepath.Join(dir, "session.txt")
	if err := os.WriteFile(txt, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resetSession(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(txt); !os.IsNotExist(err) {
		t.Fatalf("session.txt still present after reset (err=%v)", err)
	}
}

// resetSession on a profile with no session files must be a no-op, not an error.
func TestResetSessionNoFilesIsNoop(t *testing.T) {
	dir := t.TempDir()
	p := &config.Profile{Name: "default", Dir: dir, SessionPath: filepath.Join(dir, "session.db")}
	if err := resetSession(p); err != nil {
		t.Fatalf("reset on empty profile should not error: %v", err)
	}
}
