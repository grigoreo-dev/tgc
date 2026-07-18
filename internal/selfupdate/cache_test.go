package selfupdate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/version"
)

func withGlobalConfig(t *testing.T) string {
	t.Helper()
	// Update-check cache lives under GlobalDir (XDG/HOME), not TGC_CONFIG_DIR.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("TGC_CONFIG_DIR", "")
	return filepath.Join(dir, "tgc")
}

func TestCacheRoundTrip(t *testing.T) {
	withGlobalConfig(t)
	if err := WriteCache("v1.5.0"); err != nil {
		t.Fatalf("WriteCache err: %v", err)
	}
	_, latest, ok := readCache()
	if !ok || latest != "v1.5.0" {
		t.Fatalf("readCache = %q, %v; want v1.5.0, true", latest, ok)
	}
}

// Regression tgc-1zm: update-check cache must live in the GLOBAL config dir,
// not under a local ./.tgc discovered via walk-up. Otherwise every project
// with its own .tgc is a cache miss and burns the GitHub rate limit.
func TestCachePathIgnoresLocalTgc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TGC_CONFIG_DIR", "")

	// CWD with a local .tgc so config.Dir() would prefer it.
	work := filepath.Join(home, "proj")
	if err := os.MkdirAll(filepath.Join(work, ".tgc"), 0o700); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	wantDir := filepath.Join(home, ".config", "tgc")
	got := cachePath()
	want := filepath.Join(wantDir, "update-check.json")
	if got != want {
		t.Fatalf("cachePath() = %q, want %q (must ignore local .tgc)", got, want)
	}

	if err := WriteCache("v9.9.9"); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}
	// Written under global, not under ./proj/.tgc
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected cache at global %s: %v", want, err)
	}
	localCache := filepath.Join(work, ".tgc", "update-check.json")
	if _, err := os.Stat(localCache); !os.IsNotExist(err) {
		t.Fatalf("cache must not be written under local .tgc; found %s", localCache)
	}
	_, latest, ok := readCache()
	if !ok || latest != "v9.9.9" {
		t.Fatalf("readCache = %q, %v; want v9.9.9, true", latest, ok)
	}
}

func TestReadCacheCorrupt(t *testing.T) {
	withGlobalConfig(t)
	if err := os.MkdirAll(filepath.Dir(cachePath()), 0o700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(cachePath(), []byte("{not json"), 0o600)
	if _, _, ok := readCache(); ok {
		t.Fatalf("readCache on corrupt file: ok=true, want false")
	}
}

func TestStartupNotifyWarns(t *testing.T) {
	withGlobalConfig(t)
	old := version.Version
	defer func() { version.Version = old }()
	version.Version = "1.0.0"
	WriteCache("v2.0.0")

	var buf bytes.Buffer
	StartupNotify(&buf)
	if !strings.Contains(buf.String(), `"update_available"`) {
		t.Fatalf("StartupNotify output = %q, want update_available warning", buf.String())
	}
}

func TestStartupNotifyDisabled(t *testing.T) {
	withGlobalConfig(t)
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")
	old := version.Version
	defer func() { version.Version = old }()
	version.Version = "1.0.0"
	WriteCache("v2.0.0")

	var buf bytes.Buffer
	StartupNotify(&buf)
	if buf.Len() != 0 {
		t.Fatalf("StartupNotify with TGC_NO_UPDATE_CHECK wrote %q, want nothing", buf.String())
	}
}
