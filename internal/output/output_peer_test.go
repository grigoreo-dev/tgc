package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

func TestPrettyPeerOutputMapHidesAccessHash(t *testing.T) {
	var buf bytes.Buffer
	restore := output.SwapStdout(&buf)
	defer func() { restore(); output.SetPretty(false) }()
	output.SetPretty(true)

	p := resolve.Peer{ID: 42, AccessHash: 987654321, Type: "user", Title: "Alice"}
	output.Emit(p.OutputMap())

	got := buf.String()
	if !strings.Contains(got, "id:") || !strings.Contains(got, "42") {
		t.Fatalf("want labeled id: %q", got)
	}
	if !strings.Contains(got, "title:") || !strings.Contains(got, "Alice") {
		t.Fatalf("want labeled title: %q", got)
	}
	if !strings.Contains(got, "type:") || !strings.Contains(got, "user") {
		t.Fatalf("want labeled type: %q", got)
	}
	if strings.Contains(got, "987654321") {
		t.Fatalf("access hash number must not appear: %q", got)
	}
	if strings.Contains(got, "AccessHash") {
		t.Fatalf("AccessHash label must not appear: %q", got)
	}
}

func TestEmitRawPeerJSONOmitsAccessHash(t *testing.T) {
	var buf bytes.Buffer
	restore := output.SwapStdout(&buf)
	defer restore()

	output.Emit(resolve.Peer{ID: 42, AccessHash: 987654321, Type: "user", Title: "Alice"})

	got := strings.TrimSpace(buf.String())
	want := `{"id":42,"type":"user","title":"Alice"}`
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}
