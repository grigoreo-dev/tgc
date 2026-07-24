# Expanded Security & Quality Linting — Design

**Date:** 2026-07-24
**Bead:** tgc-1gye (brainstorming session)
**Status:** Approved pending spec review

## Problem

The current lint profile (`errcheck`, `govet`, `ineffassign`, `misspell`,
`staticcheck`, `unused`) is solid baseline hygiene but misses the classes most
relevant to a CLI that handles network I/O, secret tokens, session files, and
archive extraction: security scanning, error-wrapping correctness, and HTTP
body lifecycle.

## Baseline (measured)

Enabling the candidate linters on the current tree produced **49 signals**:

| Linter | Count | Nature |
|---|---|---|
| revive | 30 | style/readability (comments, unused params, builtin shadowing, package-comments) |
| gosec | 17 | security (perms, G104/G110/G115/G204/G304) |
| errorlint | 1 | error identity compare (`!=` vs `errors.Is`) |
| gocritic | 1 | factory callback simplification |
| bodyclose / prealloc / unconvert / wastedassign | 0 | clean — kept as regression guards |

## Decision — lint profile

Enable, on top of the existing six:

- `gosec`, `errorlint`, `bodyclose`, `gocritic`, `prealloc`, `unconvert`, `wastedassign`
- `revive`, restricted to rules: `redefines-builtin-id`, `unused-parameter`, `exported`

Explicitly NOT enabled:

- `revive/package-comments` — duplicates the deliberately-disabled staticcheck
  `ST1000`; tgc is an application whose `internal/` packages are never published
  to pkg.go.dev, so per-package doc headers have no reader.

## Decision — gosec policy

Keep gosec strict. Fix genuine risk; silence expected findings with **pointwise**
`#nosec Gxxx` carrying a rationale (never a global rule-off); exclude noise-only
findings in test files at the `_test.go` path level.

### Real security fixes

**F1. Decompression-bomb guard (G110)** — `internal/selfupdate/apply.go`
The downloaded `.tar.gz` is capped at 200 MiB, but `io.Copy(f, tr)` during
extraction is unbounded. Add an explicit extracted-size ceiling and copy through
`io.LimitReader`, returning a structured `archive` error if the binary exceeds
it. Unit-tested with an oversized member.

**F2. Explicit best-effort cleanup (G104)** — `internal/config/config.go`,
`internal/selfupdate/apply.go`
Deferred/cleanup `Close()` and `RemoveAll()` whose errors are intentionally
ignored become explicit `_ =` assignments — same behavior, visible intent.

### Pointwise justified `#nosec` exceptions

| Rule | Site(s) | Rationale |
|---|---|---|
| G115 uint64→int64 | `internal/ops/messages.go:860`, `internal/cmd/rich-e2e-send/main.go:140` | reinterpreting 64 bits of a Telegram id, not arithmetic narrowing of user input |
| G302 chmod 0755 | `internal/selfupdate/apply.go:157` | the CLI binary must be executable; 0600 is functionally wrong |
| G204 exec.Command | `internal/selfupdate/notify.go:52` | argv[0] is `os.Executable()` from the Go runtime, not user input |
| G301 dir 0755 | `internal/setup/completion.go:61` | completion dir must be traversable by the shell/package manager; holds no secrets |
| G304 read path | `internal/config/config.go:161`, `internal/setup/completion.go:150,167` | trusted managed/user-owned paths in the existing path model |

### Test-file exclusions

`G301`, `G306` (dir/file perms) on synthetic 0644/0755 fixtures in `*_test.go`
carry no security signal — exclude those rules for test paths only.

## Decision — quality fixes

- **errorlint** (`internal/client/client_test.go:39`): first confirm the
  `WrapErr` contract. If WrapErr is contractually identity-preserving for
  non-mapped errors, use a pointwise exception with rationale; otherwise switch
  the test to `errors.Is`.
- **gocritic** (`internal/setup/completion.go`): simplify the factory callback
  if it reads cleanly, else pointwise `//nolint:gocritic` with rationale.
- **revive**:
  - `redefines-builtin-id`: rename locals shadowing `cap`/`max`
    (`internal/markup/renderrich.go:267,321`, `internal/ops/media.go:438,441`).
  - `unused-parameter`: rename unused callback params to `_`
    (self.go, messages.go, and several `_test.go` seams).
  - `exported`: add short doc-comments to exported internal contracts
    (`Config`, `Profile`, `Load`, `SetProfileType`, `ListProfiles`,
    `DeleteProfile`, `Asset`, `Release`, `CheckResult`). These are
    cross-package contracts, not vanity headers.

## Config shape (.golangci.yml)

```yaml
linters:
  enable:
    - errcheck
    - govet
    - ineffassign
    - misspell
    - staticcheck
    - unused
    - bodyclose
    - errorlint
    - gocritic
    - gosec
    - prealloc
    - revive
    - unconvert
    - wastedassign
  settings:
    revive:
      rules:
        - name: redefines-builtin-id
        - name: unused-parameter
        - name: exported
      # package-comments intentionally omitted (see ST1000 rationale)
  exclusions:
    rules:
      - path: _test\.go
        linters: [errcheck]
      - path: _test\.go
        text: "G301|G306"
        linters: [gosec]
```

(existing errcheck/staticcheck settings retained verbatim)

## Verification

- New unit test for the extraction-size limit; existing self-update tests unchanged.
- `golangci-lint run ./...` → 0 issues
- `gofmt -l .`, `go build ./...`, `go vet ./...`, `go test ./...` all green
- CI keeps the pinned golangci-lint v2.12 already added in the prior epic.

## Out of scope

- No new runtime behavior beyond the extraction limit (F1).
- No test-coverage expansion for network paths (tracked separately).
- No adoption of `revive/package-comments` or staticcheck `ST1000`.
