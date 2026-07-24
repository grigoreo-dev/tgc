package setup

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCompletion_AtomicAndMarked(t *testing.T) {
	e := testEnv(t, "bash")
	path, err := e.CompletionPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	changed, skipped, err := writeCompletion(path, "bash", fakeGen("atom"))
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("absent path must not be skipped")
	}
	if !changed {
		t.Fatal("first write should change")
	}
	body := readFile(t, path)
	if !strings.HasPrefix(body, FileMarker+"\n") {
		t.Fatalf("want FileMarker first line: %q", body)
	}
	if !strings.Contains(body, "COMP:bash:atom") {
		t.Fatalf("gen body missing: %s", body)
	}
	// Parent dirs created.
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	// Idempotent.
	changed2, skipped2, err := writeCompletion(path, "bash", fakeGen("atom"))
	if err != nil {
		t.Fatal(err)
	}
	if skipped2 || changed2 {
		t.Fatalf("identical marked content: changed=%v skipped=%v", changed2, skipped2)
	}
}

func TestWriteCompletion_GeneratorError(t *testing.T) {
	e := testEnv(t, "bash")
	path, _ := e.CompletionPath("bash")
	gen := func(_ string, _ io.Writer) error {
		return errors.New("gen boom")
	}
	_, _, err := writeCompletion(path, "bash", gen)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed gen must not leave a completion file: %v", err)
	}
}

func TestWriteCompletion_UnmarkedSkipped(t *testing.T) {
	e := testEnv(t, "bash")
	path, _ := e.CompletionPath("bash")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := "# hand-written\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, skipped, err := writeCompletion(path, "bash", fakeGen("x"))
	if err != nil {
		t.Fatal(err)
	}
	if !skipped || changed {
		t.Fatalf("want skipped unmarked: changed=%v skipped=%v", changed, skipped)
	}
	if readFile(t, path) != orig {
		t.Fatal("unmarked bytes mutated")
	}
}

func TestHasFileMarker_FirstLine(t *testing.T) {
	dir := t.TempDir()
	marked := filepath.Join(dir, "m")
	if err := os.WriteFile(marked, []byte(FileMarker+"\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := fileHasMarker(marked)
	if err != nil || !ok {
		t.Fatalf("marked: ok=%v err=%v", ok, err)
	}

	unmarked := filepath.Join(dir, "u")
	if err := os.WriteFile(unmarked, []byte("nope\n"+FileMarker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err = fileHasMarker(unmarked)
	if err != nil || ok {
		t.Fatalf("unmarked: ok=%v err=%v", ok, err)
	}

	// Mid-line mention on first line still counts if contains marker substring per brief
	// ("first line contains FileMarker").
	contains := filepath.Join(dir, "c")
	if err := os.WriteFile(contains, []byte("prefix "+FileMarker+" suffix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err = fileHasMarker(contains)
	if err != nil || !ok {
		t.Fatalf("contains: ok=%v err=%v", ok, err)
	}

	missing := filepath.Join(dir, "missing")
	ok, err = fileHasMarker(missing)
	if err != nil {
		t.Fatalf("missing should not error: %v", err)
	}
	if ok {
		t.Fatal("missing is not marked")
	}
}

func TestAtomicWriteFile_NoPartialOnCompare(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := atomicWriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "hello" {
		t.Fatalf("got %q", got)
	}
	// Same content write is fine.
	if err := atomicWriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// No leftover temps.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "f.txt" {
			t.Fatalf("leftover temp entry: %s", e.Name())
		}
	}
}
