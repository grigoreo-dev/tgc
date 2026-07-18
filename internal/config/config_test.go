package config

import (
	"os"
	"path/filepath"
	"testing"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func withTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TGC_CONFIG_DIR", dir)
	return dir
}

func TestDirRespectsEnvOverride(t *testing.T) {
	dir := withTempConfig(t)
	if Dir() != dir {
		t.Fatalf("want %s, got %s", dir, Dir())
	}
}

func TestLoadMissingConfigIsEmpty(t *testing.T) {
	withTempConfig(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultProfile != "" || c.APIID != 0 {
		t.Fatalf("want zero config, got %+v", c)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	withTempConfig(t)
	if err := Save(&Config{DefaultProfile: "personal", APIID: 42, APIHash: "abc"}); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultProfile != "personal" || c.APIID != 42 || c.APIHash != "abc" {
		t.Fatalf("roundtrip mismatch: %+v", c)
	}
}

func TestResolveProfileDefaultChain(t *testing.T) {
	dir := withTempConfig(t)

	p, err := ResolveProfile("")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "default" {
		t.Fatalf(`want "default", got %q`, p.Name)
	}
	want := filepath.Join(dir, "profiles", "default", "session.db")
	if p.SessionPath != want {
		t.Fatalf("want %s, got %s", want, p.SessionPath)
	}
	if _, err := os.Stat(filepath.Dir(p.SessionPath)); err != nil {
		t.Fatalf("profile dir must be created: %v", err)
	}

	_ = Save(&Config{DefaultProfile: "work"})
	p, _ = ResolveProfile("")
	if p.Name != "work" {
		t.Fatalf(`config default: want "work", got %q`, p.Name)
	}

	p, _ = ResolveProfile("explicit")
	if p.Name != "explicit" {
		t.Fatalf(`explicit wins: got %q`, p.Name)
	}
}

func TestProfileTypeMarker(t *testing.T) {
	withTempConfig(t)
	p, _ := ResolveProfile("mybot")
	if p.Type != "" {
		t.Fatalf("new profile type must be empty, got %q", p.Type)
	}
	if err := SetProfileType(p, "bot"); err != nil {
		t.Fatal(err)
	}
	p2, _ := ResolveProfile("mybot")
	if p2.Type != "bot" {
		t.Fatalf(`want "bot", got %q`, p2.Type)
	}
}

func TestListProfiles(t *testing.T) {
	withTempConfig(t)
	_, _ = ResolveProfile("a")
	_, _ = ResolveProfile("b")
	list, err := ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2, got %d", len(list))
	}
}

func TestAPICredentialsEnvWinsOverConfig(t *testing.T) {
	withTempConfig(t)
	_ = Save(&Config{APIID: 1, APIHash: "cfg"})
	t.Setenv("TGC_API_ID", "99")
	t.Setenv("TGC_API_HASH", "env")
	c, _ := Load()
	id, hash, err := APICredentials(c)
	if err != nil {
		t.Fatal(err)
	}
	if id != 99 || hash != "env" {
		t.Fatalf("env must win: %d %s", id, hash)
	}
}

func TestAPICredentialsMissing(t *testing.T) {
	withTempConfig(t)
	c, _ := Load()
	_, _, err := APICredentials(c)
	if err == nil {
		t.Fatal("want error when no credentials anywhere")
	}
}

func TestFindLocalDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// layout: home/ws/.tgc  and home/ws/projectA/.git (no .tgc)
	ws := filepath.Join(home, "ws")
	projA := filepath.Join(ws, "projectA")
	if err := os.MkdirAll(filepath.Join(ws, ".tgc"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projA, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}

	// From projectA (which has .git but no .tgc): climb PAST .git to ws/.tgc.
	got := findLocalDirFrom(projA)
	if got != filepath.Join(ws, ".tgc") {
		t.Fatalf("climb-past-git: want %s, got %q", filepath.Join(ws, ".tgc"), got)
	}

	// Nearest wins: give projectA its own .tgc.
	if err := os.MkdirAll(filepath.Join(projA, ".tgc"), 0o700); err != nil {
		t.Fatal(err)
	}
	got = findLocalDirFrom(projA)
	if got != filepath.Join(projA, ".tgc") {
		t.Fatalf("nearest wins: want %s, got %q", filepath.Join(projA, ".tgc"), got)
	}
}

func TestFindLocalDirIgnoresFileAndMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sub := filepath.Join(home, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	// A .tgc that is a *file* must be ignored.
	if err := os.WriteFile(filepath.Join(sub, ".tgc"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := findLocalDirFrom(sub); got != "" {
		t.Fatalf(".tgc-as-file must be ignored, got %q", got)
	}
}

func TestDirLocalBeatsGlobalButNotEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "") // force ~/.config path for global
	// Create a local .tgc under a working dir, and chdir into it.
	proj := filepath.Join(home, "proj")
	local := filepath.Join(proj, ".tgc")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	chdir(t, proj)

	// No env override: local wins.
	t.Setenv("TGC_CONFIG_DIR", "")
	if got := Dir(); got != local {
		t.Fatalf("local should win: want %s, got %s", local, got)
	}
	dir, source := DirSource()
	if dir != local || source != "local" {
		t.Fatalf("DirSource local: got %s/%s", dir, source)
	}

	// Env override wins over local.
	envDir := t.TempDir()
	t.Setenv("TGC_CONFIG_DIR", envDir)
	if got := Dir(); got != envDir {
		t.Fatalf("env should win: want %s, got %s", envDir, got)
	}
	_, source = DirSource()
	if source != "env" {
		t.Fatalf("DirSource env: got %s", source)
	}
}

func TestDirGlobalFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TGC_CONFIG_DIR", "")
	// chdir somewhere with no .tgc anywhere up to home.
	work := filepath.Join(home, "empty")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	chdir(t, work)
	want := filepath.Join(home, ".config", "tgc")
	if got := Dir(); got != want {
		t.Fatalf("global fallback: want %s, got %s", want, got)
	}
	_, source := DirSource()
	if source != "global" {
		t.Fatalf("DirSource global: got %s", source)
	}
}
