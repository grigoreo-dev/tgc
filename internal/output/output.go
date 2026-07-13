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
)

var (
	defaultStdout io.Writer = os.Stdout
	stdout        io.Writer = os.Stdout
	stderr        io.Writer = os.Stderr
)

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

// Emit writes one compact JSON line to stdout.
func Emit(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		FailErr(fmt.Errorf("marshal output: %w", err))
	}
	fmt.Fprintln(stdout, string(b))
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
