package setup

import (
	"bytes"
	"errors"
	"fmt"
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

// defaultCreatePerm is used only when creating a file that did not exist.
const defaultCreatePerm os.FileMode = 0o644

// resolveWriteTarget returns the path to rename onto for an atomic write.
// Existing symlinks resolve to their target so the symlink node is preserved
// (dotfile managers). Absent path → path (create). Dangling symlink → error.
func resolveWriteTarget(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve symlink %s: %w", path, err)
	}
	return resolved, nil
}

// filePermOr returns path's permission bits when it exists, otherwise def.
// Stat follows symlinks so rewrites keep the target's mode.
func filePermOr(path string, def os.FileMode) os.FileMode {
	fi, err := os.Stat(path)
	if err != nil {
		return def
	}
	return fi.Mode().Perm()
}

// atomicWriteFile writes data to path via temp file in the same directory + rename.
// Callers pass a resolved write target and the final permission bits (preserve
// existing Mode().Perm(); defaultCreatePerm only for create-from-absent).
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nosec G301 -- completion dir must be traversable by the shell/package manager; holds no secrets
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
// atomically writes to path when allowed by marker discipline:
//   - absent → create (0644)
//   - first line contains FileMarker → regenerate if bytes differ (preserve perms)
//   - unmarked existing → skip (changed=false, skipped=true)
//
// Returns whether bytes changed and whether an unmarked file was skipped.
func writeCompletion(path, shell string, gen Generator) (changed, skipped bool, err error) {
	if gen == nil {
		return false, false, output.Errf("io_error", "completion generator is nil")
	}
	var buf bytes.Buffer
	if err := gen(shell, &buf); err != nil {
		// Single structured error; callers must not re-wrap io_error.
		return false, false, output.Errf("io_error", "generate completion for %s: %v", shell, err)
	}
	data := append([]byte(FileMarker+"\n"), buf.Bytes()...)
	return writeManagedIfAllowed(path, data, defaultCreatePerm)
}

// writeManagedIfAllowed writes data only when path is absent or first-line marked.
// Unmarked existing files are left intact (skipped=true).
// Existing marked files keep their Mode().Perm(); create uses createPerm.
// Dangling symlinks error (do not replace the link with a regular file).
func writeManagedIfAllowed(path string, data []byte, createPerm os.FileMode) (changed, skipped bool, err error) {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			changed, err = writeIfChanged(path, data, createPerm)
			return changed, false, err
		}
		return false, false, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		if _, err := filepath.EvalSymlinks(path); err != nil {
			return false, false, fmt.Errorf("resolve symlink %s: %w", path, err)
		}
	}
	marked, err := fileHasMarker(path)
	if err != nil {
		return false, false, err
	}
	if !marked {
		return false, true, nil
	}
	changed, err = writeIfChanged(path, data, createPerm)
	return changed, false, err
}

// writeIfChanged atomically writes data when it differs from existing content
// (or the file is absent). Preserves existing permissions; createPerm only when
// the write target does not yet exist. Resolves symlinks to write-through.
func writeIfChanged(path string, data []byte, createPerm os.FileMode) (bool, error) {
	target, err := resolveWriteTarget(path)
	if err != nil {
		return false, err
	}
	if existing, err := os.ReadFile(target); err == nil { //nosec G304 -- target is a resolved managed completion path, not user input
		if bytes.Equal(existing, data) {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	perm := filePermOr(target, createPerm)
	if err := atomicWriteFile(target, data, perm); err != nil {
		return false, err
	}
	return true, nil
}

// fileHasMarker reports whether path exists and its first line contains FileMarker.
// Missing files return (false, nil). Read follows symlinks.
func fileHasMarker(path string) (bool, error) {
	b, err := os.ReadFile(path) //nosec G304 -- path is a resolved managed completion path, not user input
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

// wrapIO returns err unchanged when it is already a structured *output.Error
// (avoids double-wrapping io_error). Otherwise wraps as io_error with format.
func wrapIO(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	var oe *output.Error
	if errors.As(err, &oe) {
		return err
	}
	// Append ": %v" for the underlying error.
	full := format + ": %v"
	args = append(args, err)
	return output.Errf("io_error", full, args...)
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
			return refreshed, wrapIO(err, "check completion marker %s", path)
		}
		if !ok {
			continue
		}
		changed, _, err := writeCompletion(path, shell, gen)
		if err != nil {
			return refreshed, wrapIO(err, "refresh completion %s", path)
		}
		if changed {
			refreshed = append(refreshed, path)
		}
	}
	return refreshed, nil
}
