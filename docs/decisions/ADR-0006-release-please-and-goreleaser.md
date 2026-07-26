# ADR-0006: Release Please owns version/tag/draft; GoReleaser attaches assets

## Context

tgc already builds release binaries with GoReleaser. The project needs automatic
version calculation from Conventional Commits and an automatically maintained
`CHANGELOG.md`, but it is still pre-1.0 and needs a human approval point before
public releases.

GitHub's built-in `GITHUB_TOKEN` does not trigger a separate tag-push workflow
when Release Please creates a tag. Ownership of the GitHub Release object must
be unambiguous so asset upload and publish can retry safely.

Earlier drafts used `skip-github-release` so only GoReleaser would create the
GitHub Release. That blocks Release Please from creating tags/releases (see
[googleapis/release-please#1561](https://github.com/googleapis/release-please/issues/1561)),
so the GoReleaser handoff never sees a Release-Please-owned draft. That approach
was rejected.

## Decision

Use one GitHub Actions release workflow (`.github/workflows/release.yml`) with
ordered jobs:

1. **`verify`** — `go build` / `vet` / `test` and `shellcheck install.sh`. On
   `push` to `main`, checks out the merge commit (`github.sha`). On
   `workflow_dispatch`, checks out the **validated** tag from `validate-retry`
   (regex-gated; raw `inputs.tag_name` is never interpolated into shell).
2. **Release Please** (push to `main` only) — maintains a Release PR and
   `CHANGELOG.md`, calculates the next version from Conventional Commits, and on
   Release PR merge creates the `vX.Y.Z` git tag **and** a **draft** GitHub
   Release (`draft: true`, `force-tag-creation: true` in
   `release-please-config.json`). Root package is **componentless** (no
   `"component"` field) so tags/branches stay plain `vX.Y.Z`. Root
   `group-pull-request-title-pattern` includes `${version}` so grouped Release
   PR titles remain parseable on merge. Do **not** set `skip-github-release`.
3. **GoReleaser** — runs when `release_created` is true (or on
   `workflow_dispatch` with an existing draft `tag_name`). Checks out the tag,
   uploads/replaces archives and `checksums.txt` on the **existing** draft
   (`use_existing_draft`, `replace_existing_artifacts`, `mode: keep-existing`),
   with GoReleaser changelog scraping disabled.
4. **Publish** — `gh release edit "$TAG" --draft=false` only after assets
   succeed.

Manual tag pushes are not the release path. Rebuild of an existing draft that
is **not yet published** uses **Actions → Release → Run workflow** with
`tag_name` (no Release Please). Rapid `main` pushes are serialized via workflow
`concurrency` with `cancel-in-progress: false` so a publishing run is not
cancelled.

While tgc remains below `v1.0.0`, `feat!` / `BREAKING CHANGE` produce the next
`0.x.0` via `bump-minor-pre-major`. Shipping `v1.0.0` requires an explicit
maintainer decision.

Local config check: `sh scripts/check-release-config.sh`. GoReleaser config
validation uses a concrete image tag (e.g. `goreleaser/goreleaser:v2.12.7`);
`goreleaser/goreleaser:v2` is not a published image.

## Rationale

This keeps release preparation reviewable (merge the Release PR) while
eliminating manual tag selection and hand-edited changelogs. One workflow avoids
a token-created tag failing to fire a second workflow. Release Please owns
version, notes, tag, and the draft release object; GoReleaser owns binaries and
checksum assets; the workflow owns the draft→published flip so a failed upload
does not ship an empty release.

## Consequences

- Maintainers release by reviewing and merging the generated Release PR, not by
  manually pushing a tag.
- Ordinary PRs must use validated Conventional Commit titles and squash merge so
  `main` history drives versioning and changelog entries.
- Repository administrators must enable squash merge and required CI checks, and
  confirm that a **Skipped** `conventional-commit-title` on Release Please bot
  PRs does not block merge (settings/rulesets; not workflow YAML).
- The first automated release is based on `v0.1.1` and benefits from a dry-run
  review of accumulated commits before relying on production publish.
- A failed artifact publication is retried against the same tag and **draft**
  release (not yet published); a second version is not created merely to retry
  the build.
