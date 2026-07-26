# Final review fix report — release automation blockers

**Date:** 2026-07-26  
**Workspace:** `/root/workspace/projects/tgc`  
**Branch:** `main` (explicitly authorized)  
**Beads:** not used (explicitly authorized)

## Review findings addressed

| # | Finding | Fix |
|---|---------|-----|
| 1 | `"component": "tgc"` forces component-scoped RP branches vs plain `vX.Y.Z` | Removed `component` from package config; checker fails if `"component"` key appears |
| 2 | Grouped Release PR titles default without `${version}` → silent tag/skip on merge (RP #2712) | Root `"group-pull-request-title-pattern": "chore${scope}: release${component} ${version}"` + checker assertion |
| 3 | `workflow_dispatch` verify checked out default ref (main), not the draft tag | `verify` needs `validate-retry`, checks out `needs.validate-retry.outputs.tag_name \|\| github.sha`; raw `inputs.tag_name` only in validate-retry `env` |
| 4 | Rapid main pushes can race publish | Workflow `concurrency` with `cancel-in-progress: false` |
| 5 | Docs/ADR retry wording | EN/RU/ADR: retry only for draft **not yet published**; verify ref semantics documented |
| 6 | Stale plan: `goreleaser:v2`, GoReleaser sole publisher, `component`/`skip-github-release` | Plan corrected to `v2.12.7`, draft ownership (RP draft + GR assets + `gh` publish), componentless config |

## Release Please v5 verification (sources)

- Manifest root option `group-pull-request-title-pattern` is documented in [manifest-releaser.md](https://github.com/googleapis/release-please/blob/main/docs/manifest-releaser.md); default is `chore: release ${branch}` (no version).
- Issue [googleapis/release-please#2712](https://github.com/googleapis/release-please/issues/2712): missing `${version}` in group title can skip tag/release creation on merge.
- Pattern `chore${scope}: release${component} ${version}` matches default non-group PR title pattern in `src/util/pull-request-title.ts`.
- Componentless root package + `include-component-in-tag: false` keeps plain `vX.Y.Z` tags/branches.

## Files changed

- `release-please-config.json`
- `.github/workflows/release.yml`
- `scripts/check-release-config.sh`
- `README.md`, `README.ru.md`
- `docs/decisions/ADR-0006-release-please-and-goreleaser.md`
- `.internal/plans/2026-07-26-automated-release-versioning-and-changelog.md`
- `.internal/sdd/final-review-fix-report.md` (this file)

## Verification evidence (fresh)

| Command | Exit |
|---------|------|
| `sh scripts/check-release-config.sh` (after RED assertions, GREEN) | 0 |
| `python3 -c 'import json; json.load(open("release-please-config.json"))'` | 0 |
| `actionlint` v1.7.12 on `release.yml` + `ci.yml` | 0 |
| `docker run … goreleaser/goreleaser:v2.12.7 check --config .goreleaser.yaml` | 0 (`1 configuration file(s) validated`) |
| `go build ./...` | 0 |
| `go vet ./...` | 0 |
| `go test ./...` | 0 |
| `GOLANGCI_LINT_CACHE=… golangci-lint run --path-mode=abs ./...` | 0 (`0 issues`) |
| `shellcheck install.sh scripts/check-release-config.sh` | 0 |

**RED→GREEN:** Checker updated first; with old config still containing `"component": "tgc"`, `check_exit=1`. After config/workflow/docs fixes, `check_exit=0`.

**Not run (by design):** live release / `npx release-please` dry-run requiring GitHub token.

## Concerns / residual risk

1. First live Release Please PR merge remains the true integration test (group title + draft + GoReleaser handoff).
2. Admin settings (squash, required checks, skipped title job non-blocking) still external to YAML.
3. Permissions left at workflow-level `contents: write` + `pull-requests: write` with `verify` job `contents: read`; no further tighten without live Actions proof.
4. No follow-up bead filed (user: do not use bd; token dry-run already a known out-of-scope gap).
