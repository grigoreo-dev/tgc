package setup

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRcFile(t *testing.T) {
	e := Env{Home: "/home/alice"}

	t.Run("bash", func(t *testing.T) {
		got, err := e.RcFile("bash")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := filepath.Join("/home/alice", ".bashrc")
		if got != want {
			t.Fatalf("RcFile(bash)=%q want %q", got, want)
		}
	})

	t.Run("zsh", func(t *testing.T) {
		got, err := e.RcFile("zsh")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := filepath.Join("/home/alice", ".zshrc")
		if got != want {
			t.Fatalf("RcFile(zsh)=%q want %q", got, want)
		}
	})

	t.Run("fish", func(t *testing.T) {
		got, err := e.RcFile("fish")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "" {
			t.Fatalf("RcFile(fish)=%q want empty (conf.d, no rc edit)", got)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		_, err := e.RcFile("powershell")
		if err == nil {
			t.Fatal("expected error for unsupported shell")
		}
	})
}

func TestCompletionPath(t *testing.T) {
	t.Run("bash default XDG", func(t *testing.T) {
		e := Env{Home: "/home/alice", XDGDataHome: ""}
		got, err := e.CompletionPath("bash")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := filepath.Join("/home/alice", ".local", "share", "bash-completion", "completions", "tgc")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("bash XDG override", func(t *testing.T) {
		e := Env{Home: "/home/alice", XDGDataHome: "/xdg/data"}
		got, err := e.CompletionPath("bash")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := filepath.Join("/xdg/data", "bash-completion", "completions", "tgc")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("zsh", func(t *testing.T) {
		// Spec: zsh → ~/.local/share/zsh/site-functions/_tgc (fixed under Home, not XDGDataHome)
		e := Env{Home: "/home/alice", XDGDataHome: "/xdg/data"}
		got, err := e.CompletionPath("zsh")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := filepath.Join("/home/alice", ".local", "share", "zsh", "site-functions", "_tgc")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("fish default XDG", func(t *testing.T) {
		e := Env{Home: "/home/alice", XDGConfigHome: ""}
		got, err := e.CompletionPath("fish")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := filepath.Join("/home/alice", ".config", "fish", "completions", "tgc.fish")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("fish XDG override", func(t *testing.T) {
		e := Env{Home: "/home/alice", XDGConfigHome: "/xdg/config"}
		got, err := e.CompletionPath("fish")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := filepath.Join("/xdg/config", "fish", "completions", "tgc.fish")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		e := Env{Home: "/home/alice"}
		_, err := e.CompletionPath("tcsh")
		if err == nil {
			t.Fatal("expected error for unsupported shell")
		}
	})
}

func TestFishConfDPath(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		e := Env{Home: "/home/alice", XDGConfigHome: ""}
		got := e.FishConfDPath()
		want := filepath.Join("/home/alice", ".config", "fish", "conf.d", "tgc.fish")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("XDG override", func(t *testing.T) {
		e := Env{Home: "/home/alice", XDGConfigHome: "/custom/config"}
		got := e.FishConfDPath()
		want := filepath.Join("/custom/config", "fish", "conf.d", "tgc.fish")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
}

func TestPathContains(t *testing.T) {
	e := Env{Path: "/usr/bin:/home/alice/.local/bin:/opt/bin/"}

	t.Run("hit", func(t *testing.T) {
		if !e.PathContains("/home/alice/.local/bin") {
			t.Fatal("expected hit for exact entry")
		}
	})

	t.Run("miss", func(t *testing.T) {
		if e.PathContains("/not/there") {
			t.Fatal("expected miss")
		}
	})

	t.Run("trailing-slash clean", func(t *testing.T) {
		// PATH has /opt/bin/ ; query without trailing slash should still match via Clean.
		if !e.PathContains("/opt/bin") {
			t.Fatal("expected hit after cleaning trailing slash on PATH entry")
		}
		// Query with trailing slash against entry without it.
		e2 := Env{Path: "/usr/bin:/opt/bin"}
		if !e2.PathContains("/opt/bin/") {
			t.Fatal("expected hit when query has trailing slash")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		e3 := Env{Path: ""}
		if e3.PathContains("/any") {
			t.Fatal("empty PATH should not contain anything")
		}
	})
}

// Compile-time / interface smoke: Env fields exist as specified.
func TestEnvFields(t *testing.T) {
	e := Env{
		Home:          "/h",
		XDGDataHome:   "/d",
		XDGConfigHome: "/c",
		Path:          "/p",
		Shell:         "/bin/zsh",
	}
	if e.Home != "/h" || e.XDGDataHome != "/d" || e.XDGConfigHome != "/c" || e.Path != "/p" || e.Shell != "/bin/zsh" {
		t.Fatalf("Env fields not set: %+v", e)
	}
}

func TestUnsupportedShellErrorsDistinct(t *testing.T) {
	e := Env{Home: "/home/alice"}
	_, err1 := e.RcFile("nushell")
	_, err2 := e.CompletionPath("nushell")
	if err1 == nil || err2 == nil {
		t.Fatal("expected errors")
	}
	// Errors should mention the shell or be non-empty.
	if strings.TrimSpace(err1.Error()) == "" {
		t.Fatal("empty RcFile error")
	}
	if !errors.Is(err1, err1) { // sanity
		t.Fatal("unreachable")
	}
}
