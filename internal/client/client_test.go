package client

import (
	"bufio"
	"strings"
	"testing"
)

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
