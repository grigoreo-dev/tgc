package setup

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/grigoreo-dev/tgc/internal/output"
)

// Generator produces completion script source for shell into w.
// The CLI layer supplies a Cobra-backed implementation.
type Generator func(shell string, w io.Writer) error

// supportedShells is the v1 set for RefreshMarked and shell validation.
var supportedShells = []string{"bash", "zsh", "fish"}

// atomicWriteFile writes data to path via temp file in the same directory + rename.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tgc-setup-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// writeCompletion generates completion for shell, prepends FileMarker, and
// atomically writes to path. Returns whether bytes changed.
func writeCompletion(path, shell string, gen Generator) (bool, error) {
	var buf bytes.Buffer
	if gen == nil {
		return false, output.Errf("io_error", "completion generator is nil")
	}
	if err := gen(shell, &buf); err != nil {
		return false, err
	}
	data := append([]byte(FileMarker+"\n"), buf.Bytes()...)
	return writeIfChanged(path, data, 0o644)
}

// writeIfChanged atomically writes data when it differs from existing content
// (or the file is absent). Returns whether a write occurred.
func writeIfChanged(path string, data []byte, perm os.FileMode) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, data) {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := atomicWriteFile(path, data, perm); err != nil {
		return false, err
	}
	return true, nil
}

// fileHasMarker reports whether path exists and its first line contains FileMarker.
// Missing files return (false, nil).
func fileHasMarker(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	first, _, _ := strings.Cut(string(b), "\n")
	first = strings.TrimSuffix(first, "\r")
	return strings.Contains(first, FileMarker), nil
}

// deleteIfMarked removes path only when it exists and the first line contains FileMarker.
// Returns whether the file was removed.
func deleteIfMarked(path string) (bool, error) {
	ok, err := fileHasMarker(path)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// RefreshMarked regenerates completion files that exist and start with FileMarker
// for each supported shell. Missing and unmarked files are skipped silently.
// Returns paths that were refreshed.
func RefreshMarked(e Env, gen Generator) ([]string, error) {
	refreshed := make([]string, 0)
	for _, shell := range supportedShells {
		path, err := e.CompletionPath(shell)
		if err != nil {
			continue
		}
		ok, err := fileHasMarker(path)
		if err != nil {
			return refreshed, output.Errf("io_error", "check completion marker %s: %v", path, err)
		}
		if !ok {
			continue
		}
		changed, err := writeCompletion(path, shell, gen)
		if err != nil {
			return refreshed, output.Errf("io_error", "refresh completion %s: %v", path, err)
		}
		if changed {
			refreshed = append(refreshed, path)
		}
	}
	return refreshed, nil
}
