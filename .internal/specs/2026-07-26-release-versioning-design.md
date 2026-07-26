# Automated Release Versioning and Changelog Design

## Goal

Automate release version selection from Conventional Commits while preserving a human approval point before a public tgc release. A release tag remains the single source of truth for the version embedded in the binary, GitHub release, release assets, and self-update metadata.

## Current State

The repository already publishes releases from tags. `.github/workflows/release.yml` runs on a pushed `v*.*.*` tag and invokes GoReleaser. GoReleaser uses the tag version to set `internal/version.Version`, generate platform archives and `checksums.txt`, and publish the GitHub Release. `tgc version`, Cobra's `--version`, `tgc self check`, and `tgc self update` therefore already consume a consistent release version.

The manual step is choosing and pushing the tag. There is no maintained `CHANGELOG.md`.

## Chosen Release Model

Use Release Please in manifest mode on pushes to `main`.

Release Please examines Conventional Commits after the latest release and opens one Release PR. That PR automatically updates `CHANGELOG.md` and the release manifest with the proposed version. It remains open and is updated as further eligible changes land in `main`. Merging the Release PR is the explicit approval to publish.

Release Please creates the annotated `vX.Y.Z` tag after the merge, but is configured with `skip-github-release: true`. In the same workflow run, a dependent GoReleaser job checks out that tag, creates the GitHub Release, and uploads its archives and checksum. This prevents two automations from competing to create the same GitHub Release and avoids relying on a tag push by `GITHUB_TOKEN` to trigger another workflow.

## Versioning Policy

The initial manifest records `0.1.1` as the current released version. Until an intentional stable launch, tgc follows pre-1.0 SemVer:

- `fix: ...` produces a patch release: `0.1.1` to `0.1.2`.
- `feat: ...` produces a minor release: `0.1.1` to `0.2.0`.
- `feat!: ...` or a `BREAKING CHANGE:` footer also produces a minor release while the version is below `1.0.0`, for example `0.2.0` to `0.3.0`.
- `chore:`, `docs:`, `test:`, and other non-release commits do not cause a release unless explicitly configured later.

Release Please's `bump-minor-pre-major: true` configuration implements the breaking-change rule. `v1.0.0` is never inferred merely because a breaking change exists; it is a deliberate release decision, made by setting an explicit release-as version in a reviewed Release PR configuration change.

## Files and Workflows

The implementation adds:

- `.github/workflows/release.yml`: the single release workflow, triggered by pushes to `main`. Its `release-please` job grants `contents: write` and `pull-requests: write`, invokes Release Please manifest mode, and exposes `release_created` plus `tag_name` as job outputs. Its `goreleaser` job needs the first job and runs only when `release_created` is true. It checks out `tag_name`, then runs GoReleaser.
- `release-please-config.json`: declares the root `simple` component, `v` tag prefix, automatic `CHANGELOG.md`, pre-1.0 breaking-change behavior, and `skip-github-release`.
- `.release-please-manifest.json`: records the root component's current released version as `0.1.1`.
- `CHANGELOG.md`: begins as the maintained public changelog. It includes concise historical entries for `0.1.0` and `0.1.1`, linked to their existing GitHub releases; Release Please appends all later release sections.
- A PR validation workflow or job for Conventional Commit-compatible titles, paired with the repository policy of squash-merging feature PRs. The validated PR title becomes the single commit message on `main`, which is what Release Please analyzes. Release Please's own generated PR is exempted from this title check.

`.goreleaser.yaml` retains responsibility for artifact build, checksum generation, binary version stamping, and GitHub Release publishing. Its release configuration creates a draft while assets upload, supports a retry against an existing draft, and replaces matching assets when a retry follows a partial upload.

## Release Procedure

1. Contributors merge feature or fix PRs into `main` using a Conventional Commit title and squash merge.
2. The Release Please workflow collects the resulting commits since the latest tag and opens or refreshes a Release PR.
3. The Release PR contains the proposed `CHANGELOG.md` entries and next version. For the changes accumulated since `v0.1.1`, this is the reviewable place to verify whether the next pre-1.0 version should be `0.x.0`.
4. A maintainer reviews the generated changelog, version, and CI, then merges the Release PR when ready to publish.
5. Release Please records the version as released and pushes the `vX.Y.Z` tag.
6. The dependent GoReleaser job in the same workflow checks out that tag, creates the GitHub Release, and attaches Darwin/Linux amd64/arm64 archives plus `checksums.txt`.
7. The tagged version is embedded in each binary. Existing `tgc self check` and `tgc self update` then discover and install it without code changes.

No maintainer manually creates or pushes a release tag in the normal process.

## Failure Handling and Safety

- A failed Release Please run must not create an unreviewed release; it only leaves the Release PR absent or stale until the workflow is corrected and rerun.
- A failed GoReleaser run leaves the version tag intact but without a complete public binary release. The failure must be repaired and the same workflow run retried; a new version must not be minted merely to retry a failed build.
- The self-update client already requires an asset and `checksums.txt`; incomplete GitHub releases fail safely rather than replacing the binary with an unchecked download.
- The Release PR is the changelog approval gate. User-visible changes must be represented by their Conventional Commit type and must not be released until its generated changelog is accepted.
- The workflow uses the scoped built-in `GITHUB_TOKEN` only: `contents: write` for tags and release assets, and `pull-requests: write` for the Release PR. It needs no personal access token and receives no Telegram or user-config secrets.

## Testing and Verification

- Add tests or fixture-based checks for the Release Please configuration: current baseline, patch/minor/breaking pre-1.0 bumps, and no-release commit types.
- Verify the Conventional Commit PR check accepts `fix:`, `feat:`, and breaking-change syntax while rejecting invalid feature PR titles.
- Run Release Please in dry-run mode against the current history before enabling it, confirming the first Release PR's version and generated changelog content.
- Validate workflow YAML and run the existing Go test, build, vet, lint, and shell checks before merge.
- After the first production release, confirm the GitHub tag, GoReleaser assets, `tgc version`, `tgc --version`, `tgc self check`, and `tgc self update` all report or use the same version.

## Out of Scope

- Automatic publication immediately after every push to `main`.
- npm or package-manager publishing.
- Prerelease channels such as alpha, beta, or release candidates.
- A transition to `v1.0.0` without an explicit maintainer decision.

## Stress Test Results: release versioning and changelog

### Resolved Decisions

- Release ownership: use one GitHub Actions workflow with two ordered jobs. Release Please owns the Release PR, version, changelog, and tag; GoReleaser is the only GitHub Release and artifact publisher.
- Pre-1.0 versioning: `feat!` and `BREAKING CHANGE` raise the minor version below `1.0.0`; `v1.0.0` requires an explicit `release-as: 1.0.0` decision.
- Commit source: ordinary PRs use validated Conventional Commit titles and squash merge. Release Please analyzes those resulting commits; the generated Release PR is exempt from title validation.
- Migration: the manifest baseline is `0.1.1`, historical changelog entries cover `0.1.0` and `0.1.1`, and a dry-run must validate the first automated Release PR before merge.
- Failure recovery: GoReleaser publishes through a draft and supports retried runs for the same tag; incomplete assets never become the latest public release.
- Security: the workflow uses only the built-in, least-privilege `GITHUB_TOKEN`; no PAT and no Telegram or user-config secrets are exposed.

### Changes Made

- Replaced the separate tag-triggered GoReleaser workflow design with the established single-workflow, dependent-job pattern: `release_created` and `tag_name` explicitly connect Release Please to GoReleaser.
- Added draft publishing and retry requirements to prevent an incomplete release from being discoverable by self-update.

### Deferred / Parking Lot

- Prerelease channels, signing/attestations, package-manager publishing, and the explicit `v1.0.0` release decision remain out of scope.

### Confidence Assessment

- Overall: High.
- Areas of concern: the first Release Please dry-run must be reviewed against the accumulated history before any Release PR is merged.
