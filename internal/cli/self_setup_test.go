package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/setup"
)

func TestSelfSetup_FishCreatesFilesAndOneJSONLine(t *testing.T) {
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("SHELL", "/usr/bin/fish")
	t.Setenv("PATH", "/usr/bin:/bin")

	var stdout, stderr bytes.Buffer
	restoreOut := output.SwapStdout(&stdout)
	restoreErr := output.SwapStderr(&stderr)
	t.Cleanup(func() {
		restoreOut()
		restoreErr()
	})

	rootCmd.SetArgs([]string{"self", "setup", "--shell", "fish"})
	rootCmd.SetOut(&bytes.Buffer{}) // cobra chatter must not pollute result channel
	rootCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstderr=%s", err, stderr.String())
	}

	// Exactly one JSON result line on the output package stdout (Emit).
	out := stdout.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("want exactly one JSON line on stdout, got %q", out)
	}
	var res setup.Result
	if err := json.Unmarshal([]byte(lines[0]), &res); err != nil {
		t.Fatalf("stdout not JSON Result: %v (%q)", err, lines[0])
	}
	if res.Shell != "fish" {
		t.Fatalf("shell=%q", res.Shell)
	}
	if !res.PathConfigured {
		t.Fatal("path_configured want true")
	}
	if !res.CompletionInstalled {
		t.Fatal("completion_installed want true")
	}
	if res.RcFile != "" {
		t.Fatalf("fish rc_file must be empty, got %q", res.RcFile)
	}
	if len(res.Changed) == 0 {
		t.Fatal("changed must list created files")
	}

	conf := filepath.Join(home, ".config", "fish", "conf.d", "tgc.fish")
	comp := filepath.Join(home, ".config", "fish", "completions", "tgc.fish")
	if _, err := os.Stat(conf); err != nil {
		t.Fatalf("fish conf.d missing: %v", err)
	}
	if _, err := os.Stat(comp); err != nil {
		t.Fatalf("fish completions missing: %v", err)
	}
	body, err := os.ReadFile(comp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), setup.FileMarker+"\n") {
		t.Fatalf("completion must start with FileMarker, got prefix %q", truncate(string(body), 60))
	}
}

func TestSelfSetup_RemoveReportsRemovedPaths(t *testing.T) {
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("PATH", "/usr/bin:/bin")

	// Install first.
	if err := execSelfSetup(t, "--shell", "bash"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var stdout, stderr bytes.Buffer
	restoreOut := output.SwapStdout(&stdout)
	restoreErr := output.SwapStderr(&stderr)
	t.Cleanup(func() {
		restoreOut()
		restoreErr()
	})

	rootCmd.SetArgs([]string{"self", "setup", "--shell", "bash", "--remove"})
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("remove: %v", err)
	}

	var res setup.Result
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &res); err != nil {
		t.Fatalf("json: %v (%q)", err, stdout.String())
	}
	if res.Shell != "bash" {
		t.Fatalf("shell=%q", res.Shell)
	}
	if len(res.Changed) == 0 {
		t.Fatal("remove Result.Changed must list removed paths")
	}
	// Completion file should be gone when it was marked.
	comp := filepath.Join(home, ".local", "share", "bash-completion", "completions", "tgc")
	if _, err := os.Stat(comp); !os.IsNotExist(err) {
		t.Fatalf("marked completion should be removed, stat=%v", err)
	}
}

func TestSelfSetup_UnsupportedShell(t *testing.T) {
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/usr/bin/powershell")
	t.Setenv("PATH", "/usr/bin:/bin")

	var stdout, stderr bytes.Buffer
	restoreOut := output.SwapStdout(&stdout)
	restoreErr := output.SwapStderr(&stderr)
	t.Cleanup(func() {
		restoreOut()
		restoreErr()
	})

	rootCmd.SetArgs([]string{"self", "setup", "--shell", "powershell"})
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("want unsupported_shell error")
	}
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != "unsupported_shell" {
		t.Fatalf("want unsupported_shell, got %v", err)
	}
	if !strings.Contains(oe.Message, "tgc completion") {
		t.Fatalf("message should mention manual completion: %q", oe.Message)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must stay empty on error, got %q", stdout.String())
	}
}

func TestCompletionGenerator_IsSetupGenerator(t *testing.T) {
	gen := completionGenerator()
	if gen == nil {
		t.Fatal("completionGenerator must not be nil")
	}
	var buf bytes.Buffer
	if err := gen("zsh", &buf); err != nil {
		t.Fatalf("gen zsh: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("generator wrote nothing")
	}
}

// TestSelfSetup_AfterRemoveInstallsAgain is the I1 regression: package-level
// flag vars stuck at --remove=true would make a subsequent Execute without
// --remove still call Remove instead of Run.
func TestSelfSetup_AfterRemoveInstallsAgain(t *testing.T) {
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("PATH", "/usr/bin:/bin")

	if err := execSelfSetup(t, "--shell", "bash"); err != nil {
		t.Fatalf("initial setup: %v", err)
	}
	if err := execSelfSetup(t, "--shell", "bash", "--remove"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	comp := filepath.Join(home, ".local", "share", "bash-completion", "completions", "tgc")
	if _, err := os.Stat(comp); !os.IsNotExist(err) {
		t.Fatalf("precondition: completion should be gone after remove, stat=%v", err)
	}

	// Re-install WITHOUT --remove. Sticky package flag would wrongly remove again.
	var stdout bytes.Buffer
	restoreOut := output.SwapStdout(&stdout)
	restoreErr := output.SwapStderr(&bytes.Buffer{})
	t.Cleanup(func() {
		restoreOut()
		restoreErr()
	})

	rootCmd.SetArgs([]string{"self", "setup", "--shell", "bash"})
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("re-setup after remove: %v", err)
	}

	var res setup.Result
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &res); err != nil {
		t.Fatalf("json: %v (%q)", err, stdout.String())
	}
	if !res.CompletionInstalled {
		t.Fatalf("want install path (completion_installed=true), got %+v", res)
	}
	if len(res.Changed) == 0 {
		t.Fatalf("re-setup must change files, got empty changed: %+v", res)
	}
	if _, err := os.Stat(comp); err != nil {
		t.Fatalf("completion must exist after re-setup: %v", err)
	}
}

// TestSelfSetup_SkippedWarnsStderr covers M1: unmarked managed targets left
// intact surface as setup_skipped on stderr (Result.Skipped non-empty).
func TestSelfSetup_SkippedWarnsStderr(t *testing.T) {
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("SHELL", "/usr/bin/fish")
	t.Setenv("PATH", "/usr/bin:/bin")

	// Pre-create unmarked completion so Run skips it.
	comp := filepath.Join(home, ".config", "fish", "completions", "tgc.fish")
	if err := os.MkdirAll(filepath.Dir(comp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(comp, []byte("# user-owned completion\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	restoreOut := output.SwapStdout(&stdout)
	restoreErr := output.SwapStderr(&stderr)
	t.Cleanup(func() {
		restoreOut()
		restoreErr()
	})

	rootCmd.SetArgs([]string{"self", "setup", "--shell", "fish"})
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	warn := stderr.String()
	if !strings.Contains(warn, `"setup_skipped"`) {
		t.Fatalf("stderr missing setup_skipped warning: %q", warn)
	}
	if !strings.Contains(warn, comp) {
		t.Fatalf("stderr warning should name skipped path %s: %q", comp, warn)
	}

	var res setup.Result
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &res); err != nil {
		t.Fatalf("json: %v (%q)", err, stdout.String())
	}
	if len(res.Skipped) == 0 {
		t.Fatalf("Result.Skipped want non-empty, got %+v", res)
	}
	// Unmarked user file must be left intact.
	body, err := os.ReadFile(comp)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "# user-owned completion\n" {
		t.Fatalf("unmarked completion rewritten: %q", body)
	}
}

// execSelfSetup runs tgc self setup with given extra args, discarding Emit output.
func execSelfSetup(t *testing.T, args ...string) error {
	t.Helper()
	var discard bytes.Buffer
	restoreOut := output.SwapStdout(&discard)
	restoreErr := output.SwapStderr(&discard)
	defer restoreOut()
	defer restoreErr()

	full := append([]string{"self", "setup"}, args...)
	rootCmd.SetArgs(full)
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	defer func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	}()
	return rootCmd.Execute()
}
