package setup

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/grigoreo-dev/tgc/internal/output"
)

// Result is the structured outcome of Run or Remove.
// Changed lists touched file paths; empty non-nil slice on no-op.
// Skipped lists existing unmarked managed-target files left intact (warn-friendly).
type Result struct {
	Shell               string   `json:"shell"`
	PathConfigured      bool     `json:"path_configured"`
	CompletionInstalled bool     `json:"completion_installed"`
	RcFile              string   `json:"rc_file,omitempty"`
	Changed             []string `json:"changed"`
	Skipped             []string `json:"skipped,omitempty"`
}

// Run installs PATH configuration and completion for shell.
// If shell is empty, it is detected from the basename of e.Shell.
//
// Multi-file non-atomicity (accepted): rc/conf.d is written before completion.
// A failure on the second write can leave the first file already updated; callers
// treat that as a partial apply with io_error (no success Result).
func Run(e Env, binDir, shell string, gen Generator) (*Result, error) {
	shell, err := resolveShell(e, shell)
	if err != nil {
		return nil, err
	}

	res := &Result{
		Shell:   shell,
		Changed: make([]string, 0),
	}

	switch shell {
	case "bash", "zsh":
		rc, err := e.RcFile(shell)
		if err != nil {
			return nil, unsupportedShell(shell)
		}
		res.RcFile = rc
		block := buildRCBlock(e, binDir, shell)
		// Empty block (bash + PATH already set): do not write markers-only block.
		if block != "" {
			changed, err := upsertRCFile(rc, block)
			if err != nil {
				return nil, wrapIO(err, "write rc %s", rc)
			}
			if changed {
				res.Changed = append(res.Changed, rc)
			}
		}
		res.PathConfigured = true

	case "fish":
		res.RcFile = ""
		conf := e.FishConfDPath()
		body := buildFishConf(e, binDir)
		// Marker discipline: never rewrite unmarked conf.d (I1).
		changed, skipped, err := writeManagedIfAllowed(conf, []byte(body), defaultCreatePerm)
		if err != nil {
			return nil, wrapIO(err, "write fish conf.d %s", conf)
		}
		if skipped {
			res.Skipped = append(res.Skipped, conf)
		} else if changed {
			res.Changed = append(res.Changed, conf)
		}
		// PATH is considered configured when we manage conf.d or it was already
		// present (user-owned). Either way the user has a path story; for
		// unmarked we did not touch it.
		res.PathConfigured = true

	default:
		return nil, unsupportedShell(shell)
	}

	compPath, err := e.CompletionPath(shell)
	if err != nil {
		return nil, unsupportedShell(shell)
	}
	// Marker discipline: never rewrite unmarked completion (I1).
	compChanged, compSkipped, err := writeCompletion(compPath, shell, gen)
	if err != nil {
		return nil, wrapIO(err, "write completion %s", compPath)
	}
	if compSkipped {
		res.Skipped = append(res.Skipped, compPath)
		res.CompletionInstalled = false
	} else {
		if compChanged {
			res.Changed = append(res.Changed, compPath)
		}
		res.CompletionInstalled = true
	}
	return res, nil
}

// Remove removes managed PATH/completion artifacts for shell.
// Deletes completion and fish conf.d files only when the first line contains FileMarker.
func Remove(e Env, shell string) (*Result, error) {
	shell, err := resolveShell(e, shell)
	if err != nil {
		return nil, err
	}

	res := &Result{
		Shell:   shell,
		Changed: make([]string, 0),
	}

	switch shell {
	case "bash", "zsh":
		rc, err := e.RcFile(shell)
		if err != nil {
			return nil, unsupportedShell(shell)
		}
		res.RcFile = rc
		changed, err := removeRCBlock(rc)
		if err != nil {
			return nil, wrapIO(err, "update rc %s", rc)
		}
		if changed {
			res.Changed = append(res.Changed, rc)
		}

	case "fish":
		res.RcFile = ""
		conf := e.FishConfDPath()
		removed, err := deleteIfMarked(conf)
		if err != nil {
			return nil, wrapIO(err, "remove fish conf.d %s", conf)
		}
		if removed {
			res.Changed = append(res.Changed, conf)
		}

	default:
		return nil, unsupportedShell(shell)
	}

	compPath, err := e.CompletionPath(shell)
	if err != nil {
		return nil, unsupportedShell(shell)
	}
	removed, err := deleteIfMarked(compPath)
	if err != nil {
		return nil, wrapIO(err, "remove completion %s", compPath)
	}
	if removed {
		res.Changed = append(res.Changed, compPath)
	}
	return res, nil
}

func resolveShell(e Env, shell string) (string, error) {
	if shell == "" {
		if strings.TrimSpace(e.Shell) == "" {
			return "", output.Errf("unsupported_shell",
				"shell could not be detected; pass --shell bash|zsh|fish, or run 'tgc completion <shell>' manually")
		}
		shell = filepath.Base(e.Shell)
	}
	shell = strings.TrimSpace(shell)
	switch shell {
	case "bash", "zsh", "fish":
		return shell, nil
	default:
		return "", unsupportedShell(shell)
	}
}

func unsupportedShell(shell string) error {
	return output.Errf("unsupported_shell",
		"unsupported shell %q; run 'tgc completion <shell>' manually to generate a completion script",
		shell)
}

// buildRCBlock returns a full managed block, or "" when there is nothing to
// inject (bash with binDir already on PATH — no export and no fpath).
// zsh always includes fpath even when PATH is already set.
func buildRCBlock(e Env, binDir, shell string) string {
	var body []string
	if !e.PathContains(binDir) {
		body = append(body, `export PATH="`+binDir+`:$PATH"`)
	}
	if shell == "zsh" {
		if comp, err := e.CompletionPath("zsh"); err == nil {
			body = append(body, "fpath=("+filepath.Dir(comp)+" $fpath)")
		}
	}
	if len(body) == 0 {
		return ""
	}
	lines := make([]string, 0, len(body)+2)
	lines = append(lines, BlockStart)
	lines = append(lines, body...)
	lines = append(lines, BlockEnd)
	return strings.Join(lines, "\n")
}

func buildFishConf(e Env, binDir string) string {
	var lines []string
	lines = append(lines, FileMarker)
	if !e.PathContains(binDir) {
		lines = append(lines, "fish_add_path -g "+binDir)
	}
	return strings.Join(lines, "\n") + "\n"
}

func upsertRCFile(path, block string) (bool, error) {
	target, err := resolveWriteTarget(path)
	if err != nil {
		return false, err
	}
	var content string
	b, err := os.ReadFile(target) //#nosec G304 -- target is a resolved shell RC path from the user's home, not arbitrary user input
	if err != nil {
		if !os.IsNotExist(err) {
			return false, err
		}
		content = ""
	} else {
		content = string(b)
	}
	newContent, changed := UpsertBlock(content, block)
	if !changed {
		return false, nil
	}
	perm := filePermOr(target, defaultCreatePerm)
	if err := atomicWriteFile(target, []byte(newContent), perm); err != nil {
		return false, err
	}
	return true, nil
}

func removeRCBlock(path string) (bool, error) {
	// If the rc path is a dangling symlink, surface a clear error rather than
	// replacing the link or silently no-op'ing.
	target, err := resolveWriteTarget(path)
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(target) //#nosec G304 -- target is a resolved shell RC path from the user's home, not arbitrary user input
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	newContent, changed := RemoveBlock(string(b))
	if !changed {
		return false, nil
	}
	perm := filePermOr(target, defaultCreatePerm)
	if err := atomicWriteFile(target, []byte(newContent), perm); err != nil {
		return false, err
	}
	return true, nil
}
