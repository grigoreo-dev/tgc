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
	b, err := os.ReadFile(filepath.Join(local, ".gitignore")) //#nosec G304 -- test path under temp .tgc dir
	if err != nil || string(b) != "*\n" {
		t.Fatalf("gitignore want '*\\n', got %q err=%v", string(b), err)
	}
}

// TestConfigPathReportsDefaultProfileFromConfig is the I1 regression guard:
// `tgc config path` must report the effective profile — i.e. the active
// config's default_profile — not the literal "default", so the diagnostic is
// accurate for the `tgc init --profile work` flow.
func TestConfigPathReportsDefaultProfileFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TGC_PROFILE", "")

	proj := filepath.Join(home, "proj")
	local := filepath.Join(proj, ".tgc")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	// Local config sets default_profile = work (as `tgc init --profile work` would).
	if err := os.WriteFile(filepath.Join(local, "config.toml"),
		[]byte("default_profile = \"work\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TGC_CONFIG_DIR", "")
	chdirInit(t, proj)

	res := runConfigPath()
	if res["source"] != "local" {
		t.Fatalf("precondition source=local, got %v", res["source"])
	}
	if res["profile"] != "work" {
		t.Fatalf("profile want work (from config default_profile), got %v", res["profile"])
	}
}
