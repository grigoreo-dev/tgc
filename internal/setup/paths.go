package setup

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Env holds process-environment values used by setup path resolution.
// Callers inject these; the package never reads the real environment.
type Env struct {
	Home          string
	XDGDataHome   string
	XDGConfigHome string
	Path          string
	Shell         string
}

func (e Env) xdgData() string {
	if e.XDGDataHome != "" {
		return e.XDGDataHome
	}
	return filepath.Join(e.Home, ".local", "share")
}

func (e Env) xdgConfig() string {
	if e.XDGConfigHome != "" {
		return e.XDGConfigHome
	}
	return filepath.Join(e.Home, ".config")
}

// RcFile returns the shell rc file path to edit for PATH configuration.
// fish returns "" (uses conf.d instead). Unsupported shells return an error.
func (e Env) RcFile(shell string) (string, error) {
	switch shell {
	case "bash":
		return filepath.Join(e.Home, ".bashrc"), nil
	case "zsh":
		return filepath.Join(e.Home, ".zshrc"), nil
	case "fish":
		return "", nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

// CompletionPath returns the per-user completion script path for shell.
func (e Env) CompletionPath(shell string) (string, error) {
	switch shell {
	case "bash":
		return filepath.Join(e.xdgData(), "bash-completion", "completions", "tgc"), nil
	case "zsh":
		// Fixed under ~/.local/share (not XDG_DATA_HOME) per design.
		return filepath.Join(e.Home, ".local", "share", "zsh", "site-functions", "_tgc"), nil
	case "fish":
		return filepath.Join(e.xdgConfig(), "fish", "completions", "tgc.fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

// FishConfDPath returns the fish conf.d snippet path for PATH configuration.
func (e Env) FishConfDPath() string {
	return filepath.Join(e.xdgConfig(), "fish", "conf.d", "tgc.fish")
}

// PathContains reports whether dir appears as an entry in e.Path (':' split),
// comparing filepath.Clean forms so trailing slashes match.
func (e Env) PathContains(dir string) bool {
	if e.Path == "" || dir == "" {
		return false
	}
	want := filepath.Clean(dir)
	for _, p := range strings.Split(e.Path, ":") {
		if p == "" {
			continue
		}
		if filepath.Clean(p) == want {
			return true
		}
	}
	return false
}
