# Expanded Security & Quality Linting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use beads-superpowers:subagent-driven-development (recommended) or beads-superpowers:executing-plans to implement this plan task-by-task. Each Task becomes a bead (`bd create -t task --parent <epic-id>`). Steps within tasks use checkbox (`- [ ]`) syntax for human readability.

**Goal:** Add security + quality linters (gosec, errorlint, bodyclose, gocritic, prealloc, unconvert, wastedassign, selective revive) to the golangci-lint profile, fix all justified findings, and reach a clean `golangci-lint run ./...`.

**Architecture:** Fix real findings in code first (so the linters can be turned on green), then flip on the linters in `.golangci.yml` with pointwise `#nosec`/`//nolint` exceptions and test-path exclusions. One genuine behavior change: a decompression-size guard in self-update extraction.

**Tech Stack:** Go, golangci-lint v2.12, gosec/revive rule config via `.golangci.yml`.

**Spec:** `docs/2026-07-24-expanded-linting-design.md`

## Global Constraints

- golangci-lint pinned at v2.12 (matches CI and local v2.12.2).
- gosec stays strict: fix real risk; silence expected findings with **pointwise** `#nosec Gxxx -- <rationale>` (never a global rule-off); exclude perms-noise only in `_test.go`.
- `revive` limited to rules: `redefines-builtin-id`, `unused-parameter`, `exported`. Do NOT enable `revive/package-comments` (duplicates the deliberately-off staticcheck ST1000).
- Existing errcheck/staticcheck settings in `.golangci.yml` retained verbatim (errcheck exclude-functions; staticcheck `-ST1000`; the `//nolint` for QF1002 at `internal/ops/media.go` stays).
- Every task ends green: `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test ./...` pass.
- Final gate: `golangci-lint run ./...` → 0 issues.

**Local golangci-lint:** on PATH via `export PATH="$(go env GOPATH)/bin:$PATH"`. Each lint step assumes this is exported.

---

### Task 1: self-update decompression-bomb guard (G110)

**Files:**
- Modify: `internal/selfupdate/apply.go` (`downloadAndVerify`, extraction loop ~108-124)
- Test: `internal/selfupdate/apply_test.go`

**Interfaces:**
- Produces: bounded extraction — a new const `maxExtractedBinary = 500 << 20` and `io.LimitReader` wrap; returns `output.Errf("archive", ...)` when the tgc member exceeds the cap.

**Acceptance Criteria:**
- Extraction copies through `io.LimitReader(tr, maxExtractedBinary+1)`; if more than `maxExtractedBinary` bytes are produced, return an `archive` error and do not leave a completed binary.
- A unit test with a tar member larger than the cap makes `downloadAndVerify` fail with the `archive` error.
- All existing self-update tests still pass.

- [ ] **Step 1: Write the failing test**

Add to `internal/selfupdate/apply_test.go` (reuse existing test helpers there for building a gzip+tar; inspect the file first for the fixture pattern it already uses):

```go
func TestDownloadAndVerifyRejectsOversizedBinary(t *testing.T) {
	// Build a tar.gz whose "tgc" member exceeds the extraction cap.
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	big := maxExtractedBinary + 1024
	if err := tw.WriteHeader(&tar.Header{Name: "tgc", Mode: 0o755, Size: int64(big), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	// Stream zeros without allocating `big` bytes at once.
	if _, err := io.CopyN(tw, zeroReader{}, int64(big)); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	data := raw.Bytes()

	sum := sha256.Sum256(data)
	checksums := []byte(hex.EncodeToString(sum[:]) + "  bomb_linux_amd64.tar.gz\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			_, _ = w.Write(checksums)
			return
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	asset := &Asset{Name: "bomb_linux_amd64.tar.gz", URL: srv.URL + "/a.tgz"}
	_, cleanup, err := downloadAndVerify(context.Background(), srv.Client(), asset, srv.URL+"/checksums.txt")
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("expected archive error for oversized binary, got nil")
	}
	if !strings.Contains(err.Error(), "archive") {
		t.Fatalf("expected archive error, got %v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
```

Ensure imports include `archive/tar`, `bytes`, `compress/gzip`, `context`, `crypto/sha256`, `encoding/hex`, `io`, `net/http`, `net/http/httptest`, `strings`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/selfupdate/ -run TestDownloadAndVerifyRejectsOversizedBinary -v`
Expected: FAIL — either `maxExtractedBinary` undefined (compile) or no error returned.

- [ ] **Step 3: Implement the guard**

In `internal/selfupdate/apply.go`, add the const near the top (after imports):

```go
// maxExtractedBinary bounds the extracted tgc binary to defend against a
// decompression bomb: a small .tar.gz that expands to an enormous file.
const maxExtractedBinary = 500 << 20 // 500 MiB
```

Replace the `io.Copy(f, tr)` block (currently ~115-119) with a bounded copy:

```go
			n, err := io.Copy(f, io.LimitReader(tr, maxExtractedBinary+1))
			if err != nil {
				f.Close()
				cleanup()
				return "", nil, err
			}
			if n > maxExtractedBinary {
				f.Close()
				cleanup()
				return "", nil, output.Errf("archive", "extracted binary exceeds %d bytes", maxExtractedBinary)
			}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/selfupdate/ -v`
Expected: PASS (new test + all existing).

- [ ] **Step 5: Commit**

```bash
git add internal/selfupdate/apply.go internal/selfupdate/apply_test.go
git commit -m "fix(selfupdate): bound archive extraction size (G110 decompression-bomb guard)"
```

---

### Task 2: explicit cleanup + gosec pointwise exceptions in code

**Files:**
- Modify: `internal/config/config.go` (`:84`, `:88` `tmp.Close()`; `:161` `os.ReadFile` G304)
- Modify: `internal/selfupdate/apply.go` (`:87` `os.RemoveAll`; `:157` `os.Chmod` G302)
- Modify: `internal/selfupdate/notify.go` (`:52` `exec.Command` G204)
- Modify: `internal/ops/messages.go` (`:860` uint64→int64 G115)
- Modify: `internal/cmd/rich-e2e-send/main.go` (`:140` uint64→int64 G115)
- Modify: `internal/setup/completion.go` (`:61` MkdirAll 0755 G301; `:150`,`:167` ReadFile G304)

**Interfaces:**
- No signature changes. Adds explicit `_ =` on intentionally-ignored errors and `//nosec` comments carrying rationale.

**Acceptance Criteria:**
- The two `tmp.Close()` error-branch calls in `config.go` become `_ = tmp.Close()`.
- `os.RemoveAll(tmpDir)` in the `cleanup` closure becomes `_ = os.RemoveAll(tmpDir)`.
- Each remaining gosec site carries a `//nosec Gxxx -- <rationale>` (gosec reads `//nosec`, not golangci's `//nolint`).
- `go build ./...` and `go test ./...` stay green.

- [ ] **Step 1: config.go — explicit Close + G304 marker**

`internal/config/config.go`: in the two error branches change `tmp.Close()` → `_ = tmp.Close()`. On the profile-type read (`:161`):

```go
	if b, err := os.ReadFile(filepath.Join(dir, "type")); err == nil { //nosec G304 -- dir is the resolved tgc config root, not user-controlled input
```

- [ ] **Step 2: apply.go — explicit RemoveAll + chmod marker**

`internal/selfupdate/apply.go`:

```go
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
```

```go
	if err := os.Chmod(tmpName, 0o755); err != nil { //nosec G302 -- the CLI binary must be executable; 0600 would make it non-runnable
```

- [ ] **Step 3: notify.go — exec.Command marker**

`internal/selfupdate/notify.go:52`:

```go
	cmd := exec.Command(exe, "self", "check", "--refresh-cache") //nosec G204 -- exe is os.Executable() from the Go runtime, never user input
```

(confirm `exe` derives from `os.Executable()` just above; if it's a var named differently, keep the rationale accurate.)

- [ ] **Step 4: G115 id reinterpretation markers**

`internal/ops/messages.go:860` and `internal/cmd/rich-e2e-send/main.go:140`, same line shape:

```go
		id := int64(binary.LittleEndian.Uint64(b[:])) //nosec G115 -- reinterpreting 64 bits of a Telegram id, not narrowing a computed value
```

- [ ] **Step 5: completion.go — dir perm + G304 markers**

`internal/setup/completion.go:61`:

```go
	if err := os.MkdirAll(dir, 0o755); err != nil { //nosec G301 -- completion dir must be traversable by the shell/package manager; holds no secrets
```

`:150` and `:167` `os.ReadFile(...)`:

```go
	if existing, err := os.ReadFile(target); err == nil { //nosec G304 -- target is a resolved managed completion path, not user input
```
```go
	b, err := os.ReadFile(path) //nosec G304 -- path is a resolved managed completion path, not user input
```

- [ ] **Step 6: Verify build/test**

Run: `go build ./... && go test ./...`
Expected: green.

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/selfupdate/apply.go internal/selfupdate/notify.go internal/ops/messages.go internal/cmd/rich-e2e-send/main.go internal/setup/completion.go
git commit -m "chore(security): explicit cleanup + justified gosec nosec markers"
```

---

### Task 3: revive quality fixes (builtin shadowing, unused params, exported docs)

**Files:**
- Modify: `internal/markup/renderrich.go` (`:267`, `:321` — `cap` shadows builtin)
- Modify: `internal/ops/media.go` (`:438`, `:441` — `max` shadows builtin)
- Modify: `internal/cli/self.go` (`:95` unused `args`)
- Modify: `internal/ops/messages.go` (`:654` unused `chats` param in `messageToMap`)
- Modify: `internal/config/config.go` (doc-comments for `Config`, `Profile`, `Load`, `SetProfileType`, `ListProfiles`, `DeleteProfile`)
- Modify: `internal/selfupdate/github.go` (`Asset`, `Release`), `internal/selfupdate/update.go` (`CheckResult`)
- Modify test seams with unused params: `internal/selfupdate/github_test.go:55`, `internal/selfupdate/refresh_test.go:25,53,78`, `internal/setup/completion_test.go:52`, `internal/setup/setup_test.go:556`

**Interfaces:**
- `messageToMap`: the unused `chats map[int64]tg.ChatClass` parameter is renamed to `_` (keep the type — callers pass it; changing the signature would ripple). Confirm via `rg -n 'messageToMap\(' internal/ops` that all call sites pass the same arg count.

**Acceptance Criteria:**
- No local variable or parameter named `cap`/`max` shadows the builtin (rename to `caption`/`largest` or similar).
- Unused params renamed to `_`.
- Each listed exported identifier has a `// Name ...` doc-comment.
- `revive` (redefines-builtin-id, unused-parameter, exported) reports 0 for these files.

- [ ] **Step 1: rename builtin-shadowing locals**

`internal/markup/renderrich.go:267`: `func captionText(cap tg.PageCaption, ...)` → rename param `cap`→`caption` (update body references). `:321`: `if cap := renderRichText(...)` → `if caption := renderRichText(...)` (update the `cap != ""` and usage).

`internal/ops/media.go:438-441`: rename local `max`→`largest` (update the loop body `if n > largest { largest = n }`).

- [ ] **Step 2: unused params → `_`**

`internal/cli/self.go:95`: `RunE: func(cmd *cobra.Command, args []string)` → if `args` unused in that RunE body, `_ []string`. Verify by reading the body first — only rename if truly unused.

`internal/ops/messages.go:654`: `func messageToMap(m *tg.Message, users map[int64]*tg.User, _ map[int64]tg.ChatClass, chatID int64)`.

Test seams: rename the flagged unused params (`r`, `ctx`, `shell`) to `_` at the exact lines listed.

- [ ] **Step 3: exported doc-comments**

Add concise doc-comments, e.g.:

```go
// Config is the tgc global config (default profile + API credentials).
type Config struct { ... }

// Profile is a resolved tgc account profile: its dirs, session path, and type.
type Profile struct { ... }

// Load reads the global config, returning defaults when absent.
func Load() (*Config, error) { ... }

// SetProfileType persists a profile's account type ("user"|"bot").
func SetProfileType(p *Profile, t string) error { ... }

// ListProfiles returns all profiles under the config root.
func ListProfiles() ([]Profile, error) { ... }

// DeleteProfile removes a profile's directory and session.
func DeleteProfile(name string) error { ... }

// Asset is one downloadable file attached to a GitHub release.
type Asset struct { ... }

// Release is a GitHub release: its tag and downloadable assets.
type Release struct { ... }

// CheckResult reports the outcome of a self-update check/apply.
type CheckResult struct { ... }
```

- [ ] **Step 4: Verify with revive only**

Run: `export PATH="$(go env GOPATH)/bin:$PATH"; golangci-lint run --enable-only revive ./... 2>&1 | tail`
Expected: 0 issues (with the Task 5 config; if config not yet in place, run against `/tmp/tgc-golangci-baseline.yml` restricted — but simplest is to defer full check to Task 5 and here just `go build ./... && go test ./...`).

Run: `go build ./... && go test ./...`
Expected: green.

- [ ] **Step 5: Commit**

```bash
git add internal/markup/renderrich.go internal/ops/media.go internal/cli/self.go internal/ops/messages.go internal/config/config.go internal/selfupdate/github.go internal/selfupdate/update.go internal/selfupdate/github_test.go internal/selfupdate/refresh_test.go internal/setup/completion_test.go internal/setup/setup_test.go
git commit -m "chore(lint): revive fixes — unshadow cap/max, unused params, exported docs"
```

---

### Task 4: errorlint + gocritic

**Files:**
- Modify: `internal/client/client_test.go` (`:39` errorlint)
- Modify: `internal/cli/completion.go` (`:49` gocritic unlambda)

**Interfaces:**
- No production signature changes.

**Acceptance Criteria:**
- The `WrapErr(orig) != orig` identity check either uses a pointwise `//nolint:errorlint` with rationale (identity IS the WrapErr passthrough contract — the test comment already says so) OR is left as-is under that nolint. Do NOT switch to `errors.Is`: the test verifies *pointer identity* passthrough, which `errors.Is` would not distinguish from wrapping.
- `completionGenerator` gocritic `unlambda`: replace `return func(shell string, w io.Writer) error { return writeCompletionScript(shell, w) }` with `return writeCompletionScript` IF `writeCompletionScript`'s signature is exactly `func(string, io.Writer) error` (matches `setup.Generator`); else keep the closure with a pointwise `//nolint:gocritic // explicit closure documents the Generator adapter`.
- `errorlint` and `gocritic` report 0.

- [ ] **Step 1: errorlint pointwise exception**

`internal/client/client_test.go:39`:

```go
	//nolint:errorlint // WrapErr's contract is pointer-identity passthrough for unmapped errors; errors.Is would mask a wrap regression here
	if got := WrapErr(orig); got != orig {
```

- [ ] **Step 2: gocritic unlambda**

Check the signature: `rg -n 'func writeCompletionScript' internal/cli internal/setup`. If it is `func writeCompletionScript(shell string, w io.Writer) error`, replace the body of `completionGenerator`:

```go
func completionGenerator() setup.Generator {
	return writeCompletionScript
}
```

If the signature differs (e.g. extra receiver/args), instead add above the closure:

```go
	//nolint:gocritic // explicit closure adapts writeCompletionScript to setup.Generator
	return func(shell string, w io.Writer) error {
		return writeCompletionScript(shell, w)
	}
```

- [ ] **Step 3: Verify**

Run: `go build ./... && go test ./...`
Expected: green.

- [ ] **Step 4: Commit**

```bash
git add internal/client/client_test.go internal/cli/completion.go
git commit -m "chore(lint): errorlint pointwise exception + gocritic unlambda"
```

---

### Task 5: enable linters in .golangci.yml + final green run

**Files:**
- Modify: `.golangci.yml`

**Interfaces:**
- Consumes: all code fixes from Tasks 1-4.

**Acceptance Criteria:**
- `.golangci.yml` enables the 7 new linters + selective revive per spec; retains existing errcheck/staticcheck settings verbatim.
- `_test.go` excluded from errcheck (existing) and from gosec `G301|G306`.
- `golangci-lint run ./...` → 0 issues.
- `gofmt -l .` empty; `go build ./...`, `go vet ./...`, `go test ./...` green.

- [ ] **Step 1: Update .golangci.yml**

Add to `linters.enable` (after `unused`): `bodyclose`, `errorlint`, `gocritic`, `gosec`, `prealloc`, `revive`, `unconvert`, `wastedassign`. Add under `linters.settings`:

```yaml
    revive:
      rules:
        - name: redefines-builtin-id
        - name: unused-parameter
        - name: exported
      # package-comments intentionally omitted — see ST1000 rationale above.
```

Add under `linters.exclusions.rules` (alongside the existing `_test.go` errcheck rule):

```yaml
      # gosec perms findings on synthetic test fixtures carry no signal.
      - path: _test\.go
        text: "G301|G306"
        linters:
          - gosec
```

- [ ] **Step 2: Full lint run**

Run: `export PATH="$(go env GOPATH)/bin:$PATH"; golangci-lint run ./...`
Expected: `0 issues.` If anything remains, fix at its site (real) or add a pointwise exception with rationale (expected) — never a global rule-off.

- [ ] **Step 3: Full verification**

Run: `gofmt -l . ; go build ./... && go vet ./... && go test ./...`
Expected: gofmt prints nothing; build/vet/test green.

- [ ] **Step 4: Commit**

```bash
git add .golangci.yml
git commit -m "chore(lint): enable gosec/errorlint/bodyclose/gocritic/prealloc/unconvert/wastedassign + selective revive"
```

---

### Task 6: README lint badge note (optional polish)

**Files:**
- Modify: `README.md`, `README.ru.md` (lint badge already present; no change needed unless wording references the linter set)

**Acceptance Criteria:**
- Confirm the existing `golangci-lint` badge still accurately represents the profile (it does — badge is tool-level, not rule-level). No doc edit required unless a CONTRIBUTING/lint section enumerates linters.
- `rg -n 'golangci' README.md README.ru.md CONTRIBUTING.md 2>/dev/null` reviewed; update only if a stale linter list exists.

- [ ] **Step 1: Check for stale linter enumerations**

Run: `rg -n 'golangci|gosec|staticcheck' README.md README.ru.md docs/*.md CONTRIBUTING.md 2>/dev/null`
If none enumerate the specific linter set, no change; close the task as verified-no-op.

- [ ] **Step 2: Commit only if changed**

```bash
git add -A && git commit -m "docs: note expanded lint profile" || echo "no doc change needed"
```

---

## Task dependency order

Task 1 → Task 2 → Task 3 → Task 4 → Task 5 → Task 6 (strictly sequential — Task 5 enables the linters only after Tasks 1-4 make the tree clean; Tasks 2/3 both touch `config.go`/`messages.go` so must not run in parallel).
