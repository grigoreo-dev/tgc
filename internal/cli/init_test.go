package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
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
	gi, err := os.ReadFile(filepath.Join(tgcDir, ".gitignore")) //#nosec G304 -- test path under temp .tgc dir
	if err != nil || string(gi) != "*\n" {
		t.Fatalf(".gitignore want '*\\n', got %q err=%v", string(gi), err)
	}

	// config.toml inherits creds + default_profile
	var c struct {
		DefaultProfile string `toml:"default_profile"`
		APIID          int    `toml:"api_id"`
		APIHash        string `toml:"api_hash"`
	}
	b, err := os.ReadFile(filepath.Join(tgcDir, "config.toml")) //#nosec G304 -- test path under temp .tgc dir
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
	if res["scope"] != "local" {
		t.Fatalf("scope want local, got %v", res["scope"])
	}
	if res["path"] != tgcDir {
		t.Fatalf("path want %s, got %v", tgcDir, res["path"])
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
	b, _ := os.ReadFile(filepath.Join(tgcDir, "config.toml")) //#nosec G304 -- test path under temp .tgc dir
	var c struct {
		DefaultProfile string `toml:"default_profile"`
	}
	_ = toml.Unmarshal(b, &c)
	if c.DefaultProfile != "manual" {
		t.Fatalf("additive-only must not clobber: got %q", c.DefaultProfile)
	}
}

// TestRunInitAtHomeUsesGlobalDir is the regression for CWD==$HOME: walk-up
// never treats $HOME/.tgc as local, so init must target GlobalDir instead of
// creating an unreachable ~/.tgc.
func TestRunInitAtHomeUsesGlobalDir(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("TGC_CONFIG_DIR", "")
	t.Setenv("TGC_API_ID", "42")
	t.Setenv("TGC_API_HASH", "hash42")

	chdirInit(t, home)

	var stderr bytes.Buffer
	restore := output.SwapStderr(&stderr)
	defer restore()

	res, err := runInit("default")
	if err != nil {
		t.Fatal(err)
	}

	wantDir := config.GlobalDir()
	if wantDir != filepath.Join(xdg, "tgc") {
		t.Fatalf("GlobalDir sanity: got %s", wantDir)
	}
	if res["path"] != wantDir {
		t.Fatalf("path want global %s, got %v", wantDir, res["path"])
	}
	if res["scope"] != "global" {
		t.Fatalf("scope want global, got %v", res["scope"])
	}
	if res["inherited_creds"] != true {
		t.Fatalf("inherited_creds want true, got %v", res["inherited_creds"])
	}

	// Must NOT create unreachable $HOME/.tgc.
	if _, err := os.Stat(filepath.Join(home, ".tgc")); err == nil {
		t.Fatal("init at $HOME must not create ~/.tgc")
	}
	// No project-style .gitignore in the global config dir.
	if _, err := os.Stat(filepath.Join(wantDir, ".gitignore")); err == nil {
		t.Fatal("global init must not write .gitignore")
	}

	var c struct {
		DefaultProfile string `toml:"default_profile"`
		APIID          int    `toml:"api_id"`
		APIHash        string `toml:"api_hash"`
	}
	b, err := os.ReadFile(filepath.Join(wantDir, "config.toml")) //#nosec G304 -- test path under temp .tgc dir
	if err != nil {
		t.Fatal(err)
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c.DefaultProfile != "default" || c.APIID != 42 || c.APIHash != "hash42" {
		t.Fatalf("global config: %+v", c)
	}

	// Config discovery with TGC_CONFIG_DIR unset must resolve to this global dir.
	dir, source := config.DirSource()
	if source != "global" || dir != wantDir {
		t.Fatalf("DirSource want global %s, got %s/%s", wantDir, source, dir)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultProfile != "default" || loaded.APIID != 42 || loaded.APIHash != "hash42" {
		t.Fatalf("Load via discovery: %+v", loaded)
	}

	warn := stderr.String()
	if !strings.Contains(warn, `"init_home_global"`) {
		t.Fatalf("stderr warning missing init_home_global: %q", warn)
	}
	if !strings.Contains(warn, wantDir) {
		t.Fatalf("stderr warning should name global dir %s: %q", wantDir, warn)
	}
}

// TestRunInitAtHomeAdditiveNoClobber mirrors local additive behavior for global.
func TestRunInitAtHomeAdditiveNoClobber(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("TGC_CONFIG_DIR", "")
	t.Setenv("TGC_API_ID", "")
	t.Setenv("TGC_API_HASH", "")

	gdir := filepath.Join(xdg, "tgc")
	if err := os.MkdirAll(gdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gdir, "config.toml"),
		[]byte("default_profile = \"manual\"\napi_id = 7\napi_hash = \"keep\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirInit(t, home)

	// Discard home→global warning; this test asserts additive merge only.
	restore := output.SwapStderr(io.Discard)
	defer restore()

	res, err := runInit("other")
	if err != nil {
		t.Fatal(err)
	}
	if res["scope"] != "global" {
		t.Fatalf("scope want global, got %v", res["scope"])
	}

	b, _ := os.ReadFile(filepath.Join(gdir, "config.toml")) //#nosec G304 -- test path under temp global config dir
	var c struct {
		DefaultProfile string `toml:"default_profile"`
		APIID          int    `toml:"api_id"`
		APIHash        string `toml:"api_hash"`
	}
	_ = toml.Unmarshal(b, &c)
	if c.DefaultProfile != "manual" || c.APIID != 7 || c.APIHash != "keep" {
		t.Fatalf("global additive must not clobber: %+v", c)
	}
}

// TestRunInitNotHomeKeepsLocal guards that CWD != HOME still creates ./.tgc.
func TestRunInitNotHomeKeepsLocal(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("TGC_CONFIG_DIR", "")
	t.Setenv("TGC_API_ID", "")
	t.Setenv("TGC_API_HASH", "")

	proj := filepath.Join(home, "proj")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	chdirInit(t, proj)

	res, err := runInit("work")
	if err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(proj, ".tgc")
	if res["path"] != local {
		t.Fatalf("path want local %s, got %v", local, res["path"])
	}
	if res["scope"] != "local" {
		t.Fatalf("scope want local, got %v", res["scope"])
	}
	if _, err := os.Stat(filepath.Join(local, ".gitignore")); err != nil {
		t.Fatalf("local init should write .gitignore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(xdg, "tgc")); err == nil {
		t.Fatal("non-home init must not create global dir")
	}
}

// TestRunInitAtHomeViaSymlink: when CWD is a symlink to $HOME, still retarget global.
func TestRunInitAtHomeViaSymlink(t *testing.T) {
	root := t.TempDir()
	realHome := filepath.Join(root, "realhome")
	if err := os.MkdirAll(realHome, 0o700); err != nil {
		t.Fatal(err)
	}
	linkHome := filepath.Join(root, "linkhome")
	if err := os.Symlink(realHome, linkHome); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	xdg := filepath.Join(root, "xdg")
	t.Setenv("HOME", realHome)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("TGC_CONFIG_DIR", "")
	t.Setenv("TGC_API_ID", "1")
	t.Setenv("TGC_API_HASH", "h")

	// CWD is the symlink path to HOME.
	chdirInit(t, linkHome)

	restore := output.SwapStderr(io.Discard)
	defer restore()

	res, err := runInit("default")
	if err != nil {
		t.Fatal(err)
	}
	want := config.GlobalDir()
	if res["scope"] != "global" || res["path"] != want {
		t.Fatalf("symlink HOME: want global %s, got scope=%v path=%v", want, res["scope"], res["path"])
	}
	if _, err := os.Stat(filepath.Join(realHome, ".tgc")); err == nil {
		t.Fatal("symlink HOME init must not create ~/.tgc")
	}
	if _, err := os.Stat(filepath.Join(linkHome, ".tgc")); err == nil {
		t.Fatal("symlink HOME init must not create .tgc under link path")
	}
}
