package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/config"
)

func TestConfigPathShadowedLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	proj := filepath.Join(home, "proj")
	local := filepath.Join(proj, ".tgc")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	chdirInit(t, proj) // reuse helper from init_test.go (same package)

	// env shadows local
	envDir := t.TempDir()
	t.Setenv("TGC_CONFIG_DIR", envDir)

	res := runConfigPath()
	if res["source"] != "env" {
		t.Fatalf("source want env, got %v", res["source"])
	}
	if res["shadowed_local"] != local {
		t.Fatalf("shadowed_local want %s, got %v", local, res["shadowed_local"])
	}
}

func TestEnsureLocalGitignore(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, ".tgc")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	config.EnsureLocalGitignore(local)
	b, err := os.ReadFile(filepath.Join(local, ".gitignore"))
	if err != nil || string(b) != "*\n" {
		t.Fatalf("gitignore want '*\\n', got %q err=%v", string(b), err)
	}
}
