# Local `./.tgc` Config Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use beads-superpowers:subagent-driven-development (recommended) or beads-superpowers:executing-plans to implement this plan task-by-task. Each Task becomes a bead (`bd create -t task --parent <epic-id>`). Steps within tasks use checkbox (`- [ ]`) syntax for human readability.

**Goal:** Let tgc prefer a local `./.tgc` config directory discovered by walking up from CWD to `$HOME`/FS-root, so multiple agents on one machine each get their own default Telegram account, with an explicit `tgc init` and a `tgc config path` diagnostic.

**Architecture:** A single change point — `config.Dir()` — gains a walk-up lookup (`findLocalDir`) inserted between the `TGC_CONFIG_DIR` env override and the global XDG/home fallback. Everything already built on `Dir()` (profiles, sessions, credentials) inherits the new root unchanged. Two new CLI commands (`tgc init`, `tgc config path`) manage and diagnose the local dir; `config.Save` becomes atomic for concurrent shared-workspace writes.

**Tech Stack:** Go 1.25, cobra (CLI), BurntSushi/toml, stdlib `os`/`path/filepath`.

## Global Constraints

- Module path: `github.com/grigoreo-dev/tgc` (use in all import paths).
- Output contract: stdout = compact JSONL (or `--pretty`); errors = structured `output.Errf`/`ErrfX` to stderr + exit 1. Never print to stdout outside `output.Emit`.
- Dir permissions `0o700`, file permissions `0o600` (match existing `config.Save`/`ResolveProfile`).
- No new third-party dependencies.
- CLI commands register via `func init()` + `rootCmd.AddCommand` (existing pattern).
- Tests use `t.TempDir()` + `t.Setenv(...)` (existing pattern in `config_test.go`).
- `TGC_CONFIG_DIR` remains the highest-priority override — never regress this.

---

### Task 1: Walk-up discovery helper `findLocalDir`

**Files:**
- Modify: `internal/config/config.go` (add `findLocalDir`, `homeDir` helpers; imports already include `os`, `path/filepath`, `strings`)
- Test: `internal/config/config_test.go` (add tests)

**Interfaces:**
- Consumes: nothing new.
- Produces: `func findLocalDir() string` — returns the nearest ancestor directory (CWD upward, `$HOME`/FS-root inclusive boundary) that contains a `.tgc` **directory**, or `""` if none. Not exported.

**Acceptance Criteria:**
- Finds `.tgc` in CWD; finds nearest in a parent; climbs PAST a subdirectory containing `.git` to a higher `.tgc`.
- Stops at `$HOME` inclusive; when `$HOME` unset, only stops at FS root.
- A `.tgc` that is a regular file (not a dir) is ignored; walk continues.
- A `stat`/permission error mid-climb is skipped, never fatal.
- Returns `""` when no `.tgc` exists in range.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
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
```

Note: tests call an internal seam `findLocalDirFrom(start string)`; `findLocalDir()` wraps it with `os.Getwd()`. This keeps tests independent of the process CWD.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestFindLocalDir -v`
Expected: FAIL — `findLocalDirFrom` undefined.

- [ ] **Step 3: Implement the helper**

Add to `internal/config/config.go`:

```go
// homeDir returns the user's home directory, or "" if it cannot be determined
// (e.g. HOME unset in a container). Callers treat "" as "no home boundary".
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// findLocalDir walks up from the current working directory looking for a
// directory that contains a ".tgc" subdirectory. The nearest match wins. The
// climb stops after inspecting $HOME (when known) or the filesystem root. It is
// NOT bounded by git-root: a shared workspace/.tgc must be reachable from a
// subproject that has its own .git. Returns "" when no local .tgc exists.
func findLocalDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return findLocalDirFrom(wd)
}

func findLocalDirFrom(start string) string {
	home := homeDir()
	dir := start
	for {
		candidate := filepath.Join(dir, ".tgc")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		// stat error (missing/permission) => keep climbing, never fatal.
		if home != "" && dir == home {
			return "" // inspected $HOME inclusively; stop.
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached FS root.
		}
		dir = parent
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run TestFindLocalDir -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: findLocalDir walk-up helper for local .tgc discovery (<epic-id>)"
```

---

### Task 2: Wire `findLocalDir` into `Dir()` priority chain

**Files:**
- Modify: `internal/config/config.go` (`Dir` function, lines ~28-37)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: `findLocalDir()` from Task 1.
- Produces: updated `Dir()` with priority `TGC_CONFIG_DIR` > local `.tgc` > `$XDG_CONFIG_HOME/tgc` > `~/.config/tgc`. Also produces `func DirSource() (dir, source string)` where `source ∈ {"env","local","global"}` (used by Task 4).

**Acceptance Criteria:**
- `TGC_CONFIG_DIR` still wins over an existing local `.tgc` (no regression).
- With no env override, a local `.tgc` in CWD is used.
- With neither, falls back to `$XDG_CONFIG_HOME/tgc` then `~/.config/tgc`.
- `DirSource()` reports the correct source label.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
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
```

Add this helper once (top of `config_test.go`, after imports):

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestDir(Local|Global)' -v`
Expected: FAIL — `DirSource` undefined and `Dir` doesn't consult local.

- [ ] **Step 3: Implement**

Replace the existing `Dir` and `configPath` region in `internal/config/config.go`:

```go
// Dir returns the tgc config root. Priority: TGC_CONFIG_DIR, then a local
// ./.tgc discovered by walking up from CWD, then $XDG_CONFIG_HOME/tgc, then
// ~/.config/tgc.
func Dir() string {
	dir, _ := DirSource()
	return dir
}

// DirSource returns the active config dir and how it was chosen:
// "env" | "local" | "global".
func DirSource() (string, string) {
	if d := os.Getenv("TGC_CONFIG_DIR"); d != "" {
		return d, "env"
	}
	if d := findLocalDir(); d != "" {
		return d, "local"
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "tgc"), "global"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tgc"), "global"
}
```

(Leave `configPath()` unchanged — it already calls `Dir()`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (new tests + all existing, including `TestDirRespectsEnvOverride`).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: Dir() prefers local .tgc via walk-up, adds DirSource (<epic-id>)"
```

---

### Task 3: Atomic `config.Save` + global-creds reader

**Files:**
- Modify: `internal/config/config.go` (`Save`; add `GlobalCredentials`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `Save` writes atomically (temp file in the same dir + `os.Rename`).
  - `func GlobalCredentials() (int, string)` — reads api_id/api_hash from env `TGC_API_ID`/`TGC_API_HASH` first, else the **global** config (`$XDG_CONFIG_HOME/tgc` → `~/.config/tgc`), ignoring any local `.tgc` and `TGC_CONFIG_DIR`. Returns `(0,"")` if none. Used by Task 4 (`tgc init`).

**Acceptance Criteria:**
- `Save` result is loadable (roundtrip still passes) and leaves no `.tmp` file behind.
- A concurrent reader never sees a truncated file (verified structurally via temp+rename; test asserts no partial file remains and content is complete).
- `GlobalCredentials` reads from env when set; else from the global config path even when a local `.tgc` or `TGC_CONFIG_DIR` is present.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestSaveIsAtomicNoTempLeft(t *testing.T) {
	dir := withTempConfig(t)
	if err := Save(&Config{DefaultProfile: "p", APIID: 7, APIHash: "h"}); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil || c.APIID != 7 {
		t.Fatalf("roundtrip: %+v err=%v", c, err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" || strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestGlobalCredentialsIgnoresLocalAndEnvDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TGC_CONFIG_DIR", "")
	t.Setenv("TGC_API_ID", "")
	t.Setenv("TGC_API_HASH", "")

	// Write global config with creds.
	globalDir := filepath.Join(home, ".config", "tgc")
	if err := os.MkdirAll(globalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.toml"),
		[]byte("api_id = 555\napi_hash = \"gh\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	id, hash := GlobalCredentials()
	if id != 555 || hash != "gh" {
		t.Fatalf("want global creds 555/gh, got %d/%s", id, hash)
	}

	// env wins.
	t.Setenv("TGC_API_ID", "1")
	t.Setenv("TGC_API_HASH", "env")
	id, hash = GlobalCredentials()
	if id != 1 || hash != "env" {
		t.Fatalf("env should win: got %d/%s", id, hash)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestSaveIsAtomic|TestGlobalCredentials' -v`
Expected: FAIL — `GlobalCredentials` undefined; Save-atomic test may pass incidentally but keep it.

- [ ] **Step 3: Implement**

Replace `Save` in `internal/config/config.go` and add `GlobalCredentials`:

```go
// Save writes the config atomically: write to a temp file in the same
// directory, then rename over the target. This prevents a concurrent reader
// (e.g. another agent on a shared workspace/.tgc) from observing a torn file.
func Save(c *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "config-*.toml.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := toml.NewEncoder(tmp).Encode(c); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, configPath())
}

// GlobalCredentials returns api_id/api_hash for seeding a new local config:
// env TGC_API_ID/TGC_API_HASH first, else the GLOBAL config
// ($XDG_CONFIG_HOME/tgc → ~/.config/tgc), deliberately ignoring any local
// .tgc and TGC_CONFIG_DIR. Returns (0,"") when none are set.
func GlobalCredentials() (int, string) {
	// env first
	id := 0
	hash := ""
	if v := os.Getenv("TGC_API_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			id = n
		}
	}
	if v := os.Getenv("TGC_API_HASH"); v != "" {
		hash = v
	}
	if id != 0 && hash != "" {
		return id, hash
	}
	// global config path (bypass Dir()'s local walk-up)
	var globalDir string
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		globalDir = filepath.Join(x, "tgc")
	} else {
		home, _ := os.UserHomeDir()
		globalDir = filepath.Join(home, ".config", "tgc")
	}
	var gc Config
	if b, err := os.ReadFile(filepath.Join(globalDir, "config.toml")); err == nil {
		if err := toml.Unmarshal(b, &gc); err == nil {
			if id == 0 {
				id = gc.APIID
			}
			if hash == "" {
				hash = gc.APIHash
			}
		}
	}
	return id, hash
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: atomic config.Save + GlobalCredentials reader (<epic-id>)"
```

---

### Task 4: `tgc init` command

**Files:**
- Create: `internal/cli/init.go`
- Test: `internal/cli/init_test.go`

**Interfaces:**
- Consumes: `config.GlobalCredentials()` (Task 3), `config.Save` semantics, `output.Emit`.
- Produces: `tgc init [--profile <name>]` command registered on `rootCmd`. Writes `./.tgc/` in CWD (0700), `./.tgc/.gitignore` = `*`, and additively fills `./.tgc/config.toml`.

**Acceptance Criteria:**
- Creates `./.tgc/`, `./.tgc/.gitignore` (content `*`), and `./.tgc/config.toml`.
- `config.toml` gets `default_profile` (flag or `"default"`) and inherited creds when available; sessions are NOT copied.
- Re-running does NOT overwrite an existing non-empty `default_profile`/`api_id`/`api_hash` (additive-only).
- Emits `{"path":..., "inherited_creds":bool}`; when `inherited_creds=false` also emits a `"next"` hint. Never errors on missing creds.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/init_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run TestRunInit -v`
Expected: FAIL — `runInit` undefined.

- [ ] **Step 3: Implement**

Create `internal/cli/init.go`:

```go
package cli

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var initProfile string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a local ./.tgc config directory in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := initProfile
		if name == "" {
			name = "default"
		}
		res, err := runInit(name)
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

// localConfig is the on-disk shape of ./.tgc/config.toml. Kept local to init;
// mirrors config.Config's toml tags.
type localConfig struct {
	DefaultProfile string `toml:"default_profile"`
	APIID          int    `toml:"api_id"`
	APIHash        string `toml:"api_hash"`
}

// runInit creates ./.tgc in CWD (additive, git-init style) and returns the
// result map. It never uses walk-up: init always targets CWD.
func runInit(profile string) (map[string]any, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, output.Errf("io_error", "cannot determine working directory: %v", err)
	}
	tgcDir := filepath.Join(wd, ".tgc")
	if err := os.MkdirAll(tgcDir, 0o700); err != nil {
		return nil, output.Errf("io_error", "cannot create %s: %v", tgcDir, err)
	}

	// .gitignore = * (create if missing; do not clobber an existing one)
	giPath := filepath.Join(tgcDir, ".gitignore")
	if _, err := os.Stat(giPath); os.IsNotExist(err) {
		if err := os.WriteFile(giPath, []byte("*\n"), 0o600); err != nil {
			return nil, output.Errf("io_error", "cannot write %s: %v", giPath, err)
		}
	}

	// Load existing local config (if any) for additive merge.
	cfgPath := filepath.Join(tgcDir, "config.toml")
	var lc localConfig
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = toml.Unmarshal(b, &lc) // best effort; empty on parse failure
	}

	// Fill only empty fields.
	if lc.DefaultProfile == "" {
		lc.DefaultProfile = profile
	}
	inheritedCreds := false
	if lc.APIID == 0 || lc.APIHash == "" {
		id, hash := config.GlobalCredentials()
		if lc.APIID == 0 && id != 0 {
			lc.APIID = id
		}
		if lc.APIHash == "" && hash != "" {
			lc.APIHash = hash
		}
	}
	if lc.APIID != 0 && lc.APIHash != "" {
		inheritedCreds = true
	}

	// Write config.toml atomically (temp + rename in tgcDir).
	tmp, err := os.CreateTemp(tgcDir, "config-*.toml.tmp")
	if err != nil {
		return nil, output.Errf("io_error", "cannot write config: %v", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	_ = tmp.Chmod(0o600)
	if err := toml.NewEncoder(tmp).Encode(&lc); err != nil {
		tmp.Close()
		return nil, output.Errf("io_error", "cannot encode config: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, output.Errf("io_error", "cannot write config: %v", err)
	}
	if err := os.Rename(tmpName, cfgPath); err != nil {
		return nil, output.Errf("io_error", "cannot finalize config: %v", err)
	}

	res := map[string]any{"path": tgcDir, "inherited_creds": inheritedCreds}
	if !inheritedCreds {
		res["next"] = "set TGC_API_ID/TGC_API_HASH or edit .tgc/config.toml, then run: tgc auth login"
	}
	return res, nil
}

func init() {
	initCmd.Flags().StringVar(&initProfile, "profile", "", "default profile name for this local config (default: \"default\")")
	rootCmd.AddCommand(initCmd)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run TestRunInit -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/init.go internal/cli/init_test.go
git commit -m "feat: tgc init creates local .tgc (additive, gitignore, creds inherit) (<epic-id>)"
```

---

### Task 5: `tgc config path` diagnostic + `.gitignore` self-heal

**Files:**
- Create: `internal/cli/config.go`
- Modify: `internal/config/config.go` (add `EnsureLocalGitignore` helper, call site chosen in Task 6 review; here we add + test the helper and the command)
- Test: `internal/cli/config_test.go`

**Interfaces:**
- Consumes: `config.DirSource()` (Task 2), `config.ProfileName`-equivalent via `ProfileName()` (root.go), `config.findLocalDir` is internal — expose the shadow via a new exported `config.LocalDir() string` that returns `findLocalDir()`'s result regardless of env.
- Produces:
  - `config.LocalDir() string` — the walk-up result (may be non-empty even when env shadows it).
  - `config.EnsureLocalGitignore(dir string)` — if `dir` is a local `.tgc` lacking `.gitignore`, write `*` silently (best-effort, returns nothing).
  - `tgc config path` command emitting `{"config_dir","source","profile"[,"shadowed_local"]}`.

**Acceptance Criteria:**
- `tgc config path` reports `source=local` and the local dir when a local `.tgc` is active.
- When `TGC_CONFIG_DIR` shadows an existing local `.tgc`, output includes `shadowed_local` with the local path.
- `EnsureLocalGitignore` creates a missing `.gitignore`(`*`) in a local `.tgc` and is a no-op when present.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/config_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run 'TestConfigPath|TestEnsureLocalGitignore' -v`
Expected: FAIL — `runConfigPath`, `config.LocalDir`, `config.EnsureLocalGitignore` undefined.

- [ ] **Step 3: Implement**

Add to `internal/config/config.go`:

```go
// LocalDir returns the walk-up ./.tgc result regardless of TGC_CONFIG_DIR.
// It is "" when no local .tgc exists in range. Used to surface a shadowed
// local dir in diagnostics.
func LocalDir() string { return findLocalDir() }

// EnsureLocalGitignore writes a "*" .gitignore into a local .tgc directory when
// missing. Best-effort and silent: it never returns an error and prints nothing,
// preserving the JSONL/stderr output contract.
func EnsureLocalGitignore(tgcDir string) {
	gi := filepath.Join(tgcDir, ".gitignore")
	if _, err := os.Stat(gi); os.IsNotExist(err) {
		_ = os.WriteFile(gi, []byte("*\n"), 0o600)
	}
}
```

Create `internal/cli/config.go`:

```go
package cli

import (
	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect tgc configuration",
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show the active config directory and how it was selected",
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Emit(runConfigPath())
		return nil
	},
}

// runConfigPath reports the active config dir, its source, the selected profile,
// and — when env shadows an existing local .tgc — the shadowed local path.
func runConfigPath() map[string]any {
	dir, source := config.DirSource()
	profile := ProfileName()
	if profile == "" {
		profile = "default"
	}
	res := map[string]any{
		"config_dir": dir,
		"source":     source,
		"profile":    profile,
	}
	if source != "local" {
		if local := config.LocalDir(); local != "" && local != dir {
			res["shadowed_local"] = local
		}
	}
	return res
}

func init() {
	configCmd.AddCommand(configPathCmd)
	rootCmd.AddCommand(configCmd)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run 'TestConfigPath|TestEnsureLocalGitignore' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/config.go internal/cli/config_test.go internal/config/config.go
git commit -m "feat: tgc config path diagnostic + EnsureLocalGitignore self-heal (<epic-id>)"
```

---

### Task 6: Wire self-heal into resolution + enrich not_authenticated

**Files:**
- Modify: `internal/config/config.go` (call `EnsureLocalGitignore` when a local dir is resolved)
- Modify: `internal/client/client.go` (`Connect`, lines ~95-122: mention local context in the not_authenticated error)
- Test: `internal/config/config_test.go`, `internal/client/client_test.go`

**Interfaces:**
- Consumes: `config.DirSource`, `config.EnsureLocalGitignore`.
- Produces: side-effecting self-heal on local resolution; enriched error text.

**Acceptance Criteria:**
- When `DirSource()` returns `source="local"`, a missing `.gitignore` in that dir is created (self-heal) exactly once per resolution, silently.
- The `not_authenticated` error from `Connect` names the local `.tgc` when the active source is local.
- No stdout output is produced by self-heal.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestDirSelfHealsLocalGitignore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TGC_CONFIG_DIR", "")
	proj := filepath.Join(home, "proj")
	local := filepath.Join(proj, ".tgc")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	chdir(t, proj)

	if got := Dir(); got != local {
		t.Fatalf("precondition: local not active, got %s", got)
	}
	// self-heal must have created .gitignore
	if _, err := os.Stat(filepath.Join(local, ".gitignore")); err != nil {
		t.Fatalf(".gitignore should be self-healed: %v", err)
	}
}
```

Add to `internal/client/client_test.go` (a unit test on the error-building helper — see Step 3 for the extracted function):

```go
func TestNotAuthenticatedMentionsLocal(t *testing.T) {
	msg := notAuthenticatedMsg("work", "/proj/.tgc")
	if !strings.Contains(msg, "local") || !strings.Contains(msg, "/proj/.tgc") {
		t.Fatalf("want local context in message, got %q", msg)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestDirSelfHeals -v && go test ./internal/client/ -run TestNotAuthenticatedMentionsLocal -v`
Expected: FAIL — self-heal not wired; `notAuthenticatedMsg` undefined.

- [ ] **Step 3: Implement**

In `internal/config/config.go`, update `DirSource` to self-heal on the local branch:

```go
	if d := findLocalDir(); d != "" {
		EnsureLocalGitignore(d)
		return d, "local"
	}
```

In `internal/client/client.go`, extract the message builder and use the active source. Replace the not_authenticated block in `Connect`:

```go
		if isNotAuthenticated(err) {
			dir, source := config.DirSource()
			local := ""
			if source == "local" {
				local = dir
			}
			return nil, output.Errf("not_authenticated", "%s", notAuthenticatedMsg(p.Name, local))
		}
```

Add the helper in `internal/client/client.go`:

```go
// notAuthenticatedMsg builds the not_authenticated message, naming the local
// ./.tgc context when the active config came from a local dir.
func notAuthenticatedMsg(profile, localDir string) string {
	if localDir != "" {
		return "profile " + profile + " in local " + localDir +
			" has no valid session; run `tgc auth login` or `tgc auth import`"
	}
	return "profile " + profile +
		" has no valid session; run `tgc auth login` or `tgc auth import`"
}
```

(Ensure `internal/client/client.go` imports `github.com/grigoreo-dev/tgc/internal/config` — it already does — and `client_test.go` imports `strings`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ ./internal/client/ -v`
Expected: PASS (new + existing).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/client/client.go internal/client/client_test.go internal/config/config_test.go
git commit -m "feat: self-heal local .gitignore + local-aware not_authenticated (<epic-id>)"
```

---

### Task 7: Docs — README/README.ru/AGENTS.md

**Files:**
- Modify: `README.md`, `README.ru.md`, `AGENTS.md`

**Interfaces:**
- Consumes: the finished commands.
- Produces: user-facing docs.

**Acceptance Criteria:**
- README (EN+RU) document `tgc init`, local `./.tgc` discovery + priority, and `tgc config path`.
- AGENTS.md notes that agents can `tgc init` per-project for an isolated default account, and that `./.tgc` is auto-gitignored.
- No stale claims; commands match implemented flags.

- [ ] **Step 1: Add README (EN) section**

Add under an appropriate "Configuration" heading in `README.md`:

```markdown
## Local project config (`./.tgc`)

Run `tgc init` in a project directory to create a local `./.tgc` config, so that
directory (and its subdirectories) uses its own default account:

    tgc init --profile work
    tgc auth login

tgc discovers config in this order: `TGC_CONFIG_DIR` → the nearest `./.tgc`
walking up from the current directory (stopping at `$HOME`) → `~/.config/tgc`.
A shared parent `workspace/.tgc` covers all subprojects; a nearer `./.tgc`
overrides it.

`tgc init` writes `.tgc/.gitignore` (`*`) so sessions are never committed, and
inherits `api_id`/`api_hash` from your global config if set. Inspect the active
config with:

    tgc config path
```

- [ ] **Step 2: Add README (RU) section**

Add the Russian equivalent to `README.ru.md` (same structure, translated).

- [ ] **Step 3: Add AGENTS.md note**

Add to `AGENTS.md`:

```markdown
## Per-project account (`./.tgc`)

Run `tgc init` inside a project to give it an isolated default Telegram account
(`./.tgc`, discovered by walking up from the working directory). The directory
is auto-gitignored (`*`). Use `tgc config path` to see which config is active
(`source`: env | local | global) and whether an env var is shadowing a local one.
```

- [ ] **Step 4: Verify build + full suite**

Run: `go build ./... && go test ./...`
Expected: build OK; all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add README.md README.ru.md AGENTS.md
git commit -m "docs: document local .tgc config, tgc init, config path (<epic-id>)"
```

---

## Task dependency order

1 → 2 → 3 → 4 → 5 → 6 → 7 (each builds on the prior). Tasks 4 and 5 both depend on 2/3; 6 depends on 2+5; 7 depends on all.
