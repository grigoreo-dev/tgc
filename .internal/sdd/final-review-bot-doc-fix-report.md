# Final review bot-doc fix report — Release Please title / CI accuracy

**Date:** 2026-07-26  
**Workspace:** `/root/workspace/projects/tgc`  
**Branch:** `main` (explicitly authorized)  
**Beads:** not used (explicitly authorized)

## Review finding addressed

| Severity | Finding | Fix |
|----------|---------|-----|
| Important | Docs/ADR claimed `conventional-commit-title` is skipped for `release-please[bot]` and that rulesets must treat **Skipped** as non-blocking. With built-in `GITHUB_TOKEN`, Release Please PRs are authored by `github-actions[bot]`, so the skip condition does not apply. Generated title `chore(main): release X.Y.Z` matches the CI regex. | Docs/ADR corrected: title is Conventional-Commit-compatible; CI must pass on Release PRs. CI `if` left unchanged so required checks stay reliable when the job actually runs. |

## Decision (minimal, correct)

- **Do not change** `.github/workflows/ci.yml` skip condition (`!= 'release-please[bot]'`).
  - It is a no-op for the current GITHUB_TOKEN author (`github-actions[bot]`).
  - Changing it is unnecessary; keeping required title validation on bot Release PRs is desirable.
- **Do change** README (EN/RU) and ADR-0006:
  - Remove “title job skipped / ruleset must accept Skipped” as operational guidance.
  - State that the generated Release PR title is Conventional-Commit-compatible and that `conventional-commit-title` must pass.
  - Note that with built-in `GITHUB_TOKEN` the PR author is `github-actions[bot]`.

## Files changed

- `README.md`
- `README.ru.md`
- `docs/decisions/ADR-0006-release-please-and-goreleaser.md`
- `.internal/sdd/final-review-bot-doc-fix-report.md` (this file)

**Unchanged (intentionally):** `.github/workflows/ci.yml`, `scripts/check-release-config.sh`, `release-please-config.json`.

## Verification evidence (fresh)

| Command | Result |
|---------|--------|
| Title regex vs `chore(main): release 0.2.0` | PASS (matches CI ERE) |
| Title regex vs `release 0.2.0` (negative) | correctly rejects |
| `sh scripts/check-release-config.sh` | exit 0 |
| `actionlint` on `ci.yml` + `release.yml` | exit 0 |
| `go test ./...` | exit 0 |

CI condition still present (contract preserved by `check-release-config.sh`):

```yaml
if: github.event_name == 'pull_request' && github.event.pull_request.user.login != 'release-please[bot]'
```

## Residual risk

1. If the project later switches to a PAT that opens PRs as `release-please[bot]`, the title job will skip again; docs should then be revisited together with ruleset policy. Current GITHUB_TOKEN path is documented accurately.
2. Live Release Please PR merge remains the integration proof that Actions + required checks behave as described.

## Out of scope

- No CI condition change
- No release config or workflow YAML change
- No beads / no push (commit only, per request)
