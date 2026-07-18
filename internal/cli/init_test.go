package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

func chdirInit(t *testing.T, dir string) {
	t.Helper()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func TestRunInitCreatesLocalTgc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TGC_CONFIG_DIR", "")
	t.Setenv("TGC_API_ID", "321")
	t.Setenv("TGC_API_HASH", "envhash")

	proj := filepath.Join(home, "proj")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	chdirInit(t, proj)

	res, err := runInit("work")
	if err != nil {
		t.Fatal(err)
	}
	tgcDir := filepath.Join(proj, ".tgc")

	// .gitignore = *
	gi, err := os.ReadFile(filepath.Join(tgcDir, ".gitignore"))
	if err != nil || string(gi) != "*\n" {
		t.Fatalf(".gitignore want '*\\n', got %q err=%v", string(gi), err)
	}

	// config.toml inherits creds + default_profile
	var c struct {
		DefaultProfile string `toml:"default_profile"`
		APIID          int    `toml:"api_id"`
		APIHash        string `toml:"api_hash"`
	}
	b, err := os.ReadFile(filepath.Join(tgcDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c.DefaultProfile != "work" || c.APIID != 321 || c.APIHash != "envhash" {
		t.Fatalf("config: %+v", c)
	}
	if res["inherited_creds"] != true {
		t.Fatalf("inherited_creds want true, got %v", res["inherited_creds"])
	}
}

func TestRunInitAdditiveNoClobber(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TGC_CONFIG_DIR", "")
	t.Setenv("TGC_API_ID", "")
	t.Setenv("TGC_API_HASH", "")

	proj := filepath.Join(home, "proj")
	tgcDir := filepath.Join(proj, ".tgc")
	if err := os.MkdirAll(tgcDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Pre-existing config with a manual default_profile.
	if err := os.WriteFile(filepath.Join(tgcDir, "config.toml"),
		[]byte("default_profile = \"manual\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirInit(t, proj)

	if _, err := runInit("other"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(tgcDir, "config.toml"))
	var c struct {
		DefaultProfile string `toml:"default_profile"`
	}
	_ = toml.Unmarshal(b, &c)
	if c.DefaultProfile != "manual" {
		t.Fatalf("additive-only must not clobber: got %q", c.DefaultProfile)
	}
}
