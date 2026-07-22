package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/output"
)

func TestCompletion_ZshMatchesGenZshCompletion(t *testing.T) {
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")

	var want bytes.Buffer
	if err := rootCmd.GenZshCompletion(&want); err != nil {
		t.Fatalf("GenZshCompletion: %v", err)
	}

	var got bytes.Buffer
	rootCmd.SetArgs([]string{"completion", "zsh"})
	rootCmd.SetOut(&got)
	rootCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.String() != want.String() {
		t.Fatalf("zsh completion mismatch: got %d bytes, want %d", got.Len(), want.Len())
	}
	if !strings.Contains(got.String(), "tgc") {
		t.Fatalf("expected completion script to mention tgc, got prefix %q", truncate(got.String(), 80))
	}
}

func TestCompletion_BashMatchesGenBashCompletionV2(t *testing.T) {
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")

	var want bytes.Buffer
	if err := rootCmd.GenBashCompletionV2(&want, true); err != nil {
		t.Fatalf("GenBashCompletionV2: %v", err)
	}

	var got bytes.Buffer
	rootCmd.SetArgs([]string{"completion", "bash"})
	rootCmd.SetOut(&got)
	rootCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.String() != want.String() {
		t.Fatalf("bash completion mismatch: got %d bytes, want %d", got.Len(), want.Len())
	}
}

func TestCompletion_FishMatchesGenFishCompletion(t *testing.T) {
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")

	var want bytes.Buffer
	if err := rootCmd.GenFishCompletion(&want, true); err != nil {
		t.Fatalf("GenFishCompletion: %v", err)
	}

	var got bytes.Buffer
	rootCmd.SetArgs([]string{"completion", "fish"})
	rootCmd.SetOut(&got)
	rootCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.String() != want.String() {
		t.Fatalf("fish completion mismatch: got %d bytes, want %d", got.Len(), want.Len())
	}
}

func TestCompletion_InvalidShell_BadArgs(t *testing.T) {
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")

	rootCmd.SetArgs([]string{"completion", "powershell"})
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("want error for unsupported completion shell")
	}
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != "bad_args" {
		t.Fatalf("want bad_args, got %v", err)
	}
}

func TestCompletion_DefaultCobraCmdDisabled(t *testing.T) {
	if !rootCmd.CompletionOptions.DisableDefaultCmd {
		t.Fatal("rootCmd.CompletionOptions.DisableDefaultCmd must be true")
	}
	// Default hidden "completion" from Cobra must not be the only path;
	// our explicit command is registered under Use "completion".
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "completion" && !c.Hidden {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("explicit non-hidden completion command must be registered")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
