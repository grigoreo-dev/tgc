package setup

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/grigoreo-dev/tgc/internal/output"
)

// Result is the structured outcome of Run or Remove.
// Changed lists touched file paths; empty non-nil slice on no-op.
type Result struct {
	Shell               string   `json:"shell"`
	PathConfigured      bool     `json:"path_configured"`
	CompletionInstalled bool     `json:"completion_installed"`
	RcFile              string   `json:"rc_file,omitempty"`
	Changed             []string `json:"changed"`
}

// Run installs PATH configuration and completion for shell.
// If shell is empty, it is detected from the basename of e.Shell.
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
		changed, err := upsertRCFile(rc, block)
		if err != nil {
			return nil, output.Errf("io_error", "write rc %s: %v", rc, err)
		}
		if changed {
			res.Changed = append(res.Changed, rc)
		}
		res.PathConfigured = true

	case "fish":
		res.RcFile = ""
		conf := e.FishConfDPath()
		body := buildFishConf(e, binDir)
		changed, err := writeIfChanged(conf, []byte(body), 0o644)
		if err != nil {
			return nil, output.Errf("io_error", "write fish conf.d %s: %v", conf, err)
		}
		if changed {
			res.Changed = append(res.Changed, conf)
		}
		res.PathConfigured = true

	default:
		return nil, unsupportedShell(shell)
	}

	compPath, err := e.CompletionPath(shell)
	if err != nil {
		return nil, unsupportedShell(shell)
	}
	compChanged, err := writeCompletion(compPath, shell, gen)
	if err != nil {
		return nil, output.Errf("io_error", "write completion %s: %v", compPath, err)
	}
	if compChanged {
		res.Changed = append(res.Changed, compPath)
	}
	res.CompletionInstalled = true
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
			return nil, output.Errf("io_error", "update rc %s: %v", rc, err)
		}
		if changed {
			res.Changed = append(res.Changed, rc)
		}

	case "fish":
		res.RcFile = ""
		conf := e.FishConfDPath()
		removed, err := deleteIfMarked(conf)
		if err != nil {
			return nil, output.Errf("io_error", "remove fish conf.d %s: %v", conf, err)
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
		return nil, output.Errf("io_error", "remove completion %s: %v", compPath, err)
	}
	if removed {
		res.Changed = append(res.Changed, compPath)
	}
	return res, nil
}

func resolveShell(e Env, shell string) (string, error) {
	if shell == "" {
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

func buildRCBlock(e Env, binDir, shell string) string {
	var lines []string
	lines = append(lines, BlockStart)
	if !e.PathContains(binDir) {
		lines = append(lines, `export PATH="`+binDir+`:$PATH"`)
	}
	if shell == "zsh" {
		if comp, err := e.CompletionPath("zsh"); err == nil {
			lines = append(lines, "fpath=("+filepath.Dir(comp)+" $fpath)")
		}
	}
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
	var content string
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, err
		}
		// Absent: create from empty.
		content = ""
	} else {
		content = string(b)
	}
	newContent, changed := UpsertBlock(content, block)
	if !changed {
		return false, nil
	}
	if err := atomicWriteFile(path, []byte(newContent), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func removeRCBlock(path string) (bool, error) {
	b, err := os.ReadFile(path)
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
	if err := atomicWriteFile(path, []byte(newContent), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
