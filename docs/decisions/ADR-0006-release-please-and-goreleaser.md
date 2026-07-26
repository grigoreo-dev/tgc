# ADR-0006: Release Please manages versions; GoReleaser publishes artifacts

## Context

tgc already builds release binaries with GoReleaser from a manually pushed semver tag. The project needs automatic version calculation from Conventional Commits and an automatically maintained `CHANGELOG.md`, but it is still pre-1.0 and needs a human approval point before public releases.

GitHub's built-in `GITHUB_TOKEN` does not trigger a separate tag-push workflow when Release Please creates a tag. Letting both Release Please and GoReleaser create the GitHub Release would also create ambiguous ownership of release publication.

## Decision

Use one GitHub Actions release workflow with two ordered jobs:

- Release Please runs on pushes to `main`, maintains a Release PR and `CHANGELOG.md`, calculates the next version from Conventional Commits, and creates the release tag when the Release PR merges.
- A dependent GoReleaser job runs only when Release Please reports `release_created`. It checks out the resulting tag, creates the GitHub Release, and uploads release archives and `checksums.txt`.

Release Please is configured to skip GitHub Release creation. GoReleaser is the sole publisher of GitHub Releases and assets. The workflow uses only the least-privilege built-in `GITHUB_TOKEN` with `contents: write` and `pull-requests: write`.

While tgc remains below `v1.0.0`, `feat!` and `BREAKING CHANGE` produce a minor `0.x.0` release. Releasing `v1.0.0` requires an explicit maintainer decision.

## Rationale

This keeps release preparation reviewable while eliminating manual tag selection and changelog editing. Keeping both jobs in one workflow avoids a token-created tag failing to trigger a second workflow. GoReleaser remains responsible for the work it is built for: compiling platform binaries, checksums, and asset publication.

## Consequences

- Maintainers release by reviewing and merging the generated Release PR, not by manually pushing a tag.
- Ordinary PRs must use validated Conventional Commit titles and squash merge so the resulting `main` history accurately drives versioning and changelog entries.
- The first automated release is based on `v0.1.1` and requires a dry-run review against accumulated commits.
- A failed artifact publication is retried against the same tag and draft release; a second version is not created merely to retry the build.
