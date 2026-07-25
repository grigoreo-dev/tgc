package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmitCompactJSON(t *testing.T) {
	var buf bytes.Buffer
	stdout = &buf
	defer func() { stdout = defaultStdout }()

	Emit(map[string]any{"id": 1, "text": "hi"})

	got := buf.String()
	if strings.Contains(got, "  ") || strings.Count(got, "\n") != 1 {
		t.Fatalf("want single compact line, got %q", got)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestEmitAllJSONL(t *testing.T) {
	var buf bytes.Buffer
	stdout = &buf
	defer func() { stdout = defaultStdout }()

	EmitAll([]any{map[string]any{"a": 1}, map[string]any{"a": 2}})

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 JSONL lines, got %d: %q", len(lines), buf.String())
	}
	for _, ln := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("line %q not JSON: %v", ln, err)
		}
	}
}

func TestPrettyMapRendering(t *testing.T) {
	var buf bytes.Buffer
	stdout = &buf
	defer func() { stdout = defaultStdout; SetPretty(false) }()
	SetPretty(true)

	Emit(map[string]any{"id": 42, "title": "Chat"})

	got := buf.String()
	if strings.Contains(got, "{") {
		t.Fatalf("pretty must not print raw JSON: %q", got)
	}
	if !strings.Contains(got, "42") || !strings.Contains(got, "Chat") {
		t.Fatalf("values missing: %q", got)
	}
}

func TestPrettyNoColorWhenNotTTY(t *testing.T) {
	var buf bytes.Buffer
	stdout = &buf
	defer func() { stdout = defaultStdout; SetPretty(false) }()
	SetPretty(true)

	Emit(map[string]any{"a": 1})
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatalf("ANSI codes in non-TTY output: %q", buf.String())
	}
}

func TestPrettyMapOmitsNilKeepsFalseZeroEmpty(t *testing.T) {
	var buf bytes.Buffer
	restore := SwapStdout(&buf)
	defer func() { restore(); SetPretty(false) }()
	SetPretty(true)

	Emit(map[string]any{
		"flag":  false,
		"count": 0,
		"name":  "",
		"gone":  nil,
	})

	got := buf.String()
	if strings.Contains(got, "gone") {
		t.Fatalf("nil key must be omitted: %q", got)
	}
	if !strings.Contains(got, "flag: false") {
		t.Fatalf("false must be retained: %q", got)
	}
	if !strings.Contains(got, "count: 0") {
		t.Fatalf("zero must be retained: %q", got)
	}
	if !strings.Contains(got, "name:") {
		t.Fatalf("empty string key must be retained: %q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "name:") {
			rest := strings.TrimPrefix(line, "name:")
			if strings.TrimSpace(rest) != "" {
				t.Fatalf("empty string must render empty value: %q", line)
			}
		}
	}
}

func TestErrfProducesStructuredError(t *testing.T) {
	err := Errf("flood_wait", "wait %d seconds", 42)
	var e *Error
	if !AsError(err, &e) {
		t.Fatal("Errf must return *output.Error")
	}
	if e.Code != "flood_wait" || e.Message != "wait 42 seconds" {
		t.Fatalf("unexpected: %+v", e)
	}
}

func TestErrorJSONShape(t *testing.T) {
	e := &Error{Code: "not_found", Message: "no such chat", Extra: map[string]any{"query": "vasya"}}
	b, _ := json.Marshal(e.jsonBody())
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["error"] != "not_found" || m["message"] != "no such chat" || m["query"] != "vasya" {
		t.Fatalf("bad shape: %s", b)
	}
}
