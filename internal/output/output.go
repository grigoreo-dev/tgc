// Package output implements the tgc output contract:
// stdout carries only results (compact JSON / JSONL), everything else
// goes to stderr; errors are structured JSON on stderr + exit code 1.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"golang.org/x/term"
)

var (
	defaultStdout io.Writer = os.Stdout
	stdout        io.Writer = os.Stdout
	stderr        io.Writer = os.Stderr

	prettyMode bool
)

// SetPretty toggles human-readable rendering of Emit/EmitAll output.
func SetPretty(on bool) { prettyMode = on }

// IsTTY reports whether f is attached to a terminal.
func IsTTY(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// Error is a structured tgc error. Code is a stable machine-readable
// identifier (e.g. "flood_wait", "not_found", "ambiguous", "bot_unsupported").
type Error struct {
	Code    string
	Message string
	Extra   map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func (e *Error) jsonBody() map[string]any {
	m := map[string]any{"error": e.Code, "message": e.Message}
	for k, v := range e.Extra {
		m[k] = v
	}
	return m
}

// Errf creates a structured error with a code and printf-style message.
func Errf(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ErrfX is Errf with extra JSON fields.
func ErrfX(code string, extra map[string]any, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Extra: extra}
}

// AsError unwraps err into *Error.
func AsError(err error, target **Error) bool { return errors.As(err, target) }

// Emit writes one result to stdout. In pretty mode it renders a
// human-readable block; otherwise it writes one compact JSON line.
func Emit(v any) {
	if prettyMode {
		fmt.Fprint(stdout, renderPretty(v))
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		FailErr(fmt.Errorf("marshal output: %w", err))
	}
	fmt.Fprintln(stdout, string(b))
}

// colorEnabled reports whether ANSI styling should be applied. Color is
// gated on the real os.Stdout being a TTY (not the possibly-swapped
// package writer) and NO_COLOR being unset.
func colorEnabled() bool {
	return stdout == defaultStdout && IsTTY(os.Stdout) && os.Getenv("NO_COLOR") == ""
}

const (
	ansiDim   = "\x1b[2m"
	ansiReset = "\x1b[0m"
)

// dimKey styles a map key dimly when color is enabled.
func dimKey(k string) string {
	if colorEnabled() {
		return ansiDim + k + ansiReset
	}
	return k
}

// renderPretty renders v as human-readable text (no JSON braces):
//   - map[string]any → sorted "key: value" lines + a blank separator line
//   - slices → each element rendered recursively, blocks separated by blank lines
//   - scalars → the value followed by a newline
func renderPretty(v any) string {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := ""
		for _, k := range keys {
			out += fmt.Sprintf("%s: %v\n", dimKey(k), t[k])
		}
		out += "\n"
		return out
	case []any:
		out := ""
		for _, it := range t {
			out += renderPretty(it)
		}
		return out
	default:
		return fmt.Sprintf("%v\n", v)
	}
}

// EmitAll writes items as JSONL (one compact JSON object per line).
func EmitAll(items []any) {
	for _, it := range items {
		Emit(it)
	}
}

// FailErr prints a structured JSON error to stderr and exits 1.
// Unknown errors get code "internal".
func FailErr(err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: "internal", Message: err.Error()}
	}
	b, _ := json.Marshal(e.jsonBody())
	fmt.Fprintln(stderr, string(b))
	os.Exit(1)
}

// Fail is a convenience wrapper: structured error to stderr, exit 1.
func Fail(code, message string, extra map[string]any) {
	FailErr(&Error{Code: code, Message: message, Extra: extra})
}

// Warnf writes a structured warning line to stderr (non-fatal).
func Warnf(code, format string, a ...any) {
	line, _ := json.Marshal(map[string]any{"warning": code, "message": fmt.Sprintf(format, a...)})
	fmt.Fprintln(stderr, string(line))
}
