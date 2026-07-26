# Automated Release Versioning and Changelog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use beads-superpowers:subagent-driven-development (recommended) or beads-superpowers:executing-plans to implement this plan task-by-task. Each Task becomes a bead (`bd create -t task --parent <epic-id>`). Steps within tasks use checkbox (`- [ ]`) syntax for human readability.

**Goal:** Automate pre-1.0 SemVer selection and `CHANGELOG.md` maintenance from Conventional Commits, while a reviewed Release PR triggers a GoReleaser-published tgc release in one GitHub Actions workflow.

**Architecture:** `.github/workflows/release.yml` runs Release Please on every push to `main`; its outputs conditionally start a dependent GoReleaser job in the same workflow run. Release Please owns the Release PR, version manifest, changelog, and tag, while GoReleaser exclusively creates the draft GitHub Release and uploads binaries and checksums. A CI PR-title job guarantees that squash-merged commits provide valid Conventional Commit input.

**Tech Stack:** GitHub Actions, `googleapis/release-please-action`, Release Please manifest JSON, GoReleaser v2, Go 1.25, shellcheck, `go test`.

## Global Constraints

- The single workflow is `.github/workflows/release.yml`; it triggers only from pushes to `main`, not tag pushes.
- Release Please must use manifest mode, a `v` tag prefix, `skip-github-release: true`, and `bump-minor-pre-major: true`.
- `0.1.1` is the initial release baseline; `feat!` and `BREAKING CHANGE` stay in the `0.x.0` minor stream until an explicit `release-as: 1.0.0` decision.
- `CHANGELOG.md` is generated and updated by Release Please; do not hand-edit future release sections in feature PRs.
- Ordinary PRs must have Conventional Commit titles and use squash merge; the generated Release PR is exempt from title validation.
- Use only the built-in `GITHUB_TOKEN` with `contents: write` and `pull-requests: write`; do not introduce a PAT or expose Telegram/configuration secrets.
- GoReleaser is the sole GitHub Release and asset publisher; upload to a draft first and support retrying the same tag.

---

## File Structure

- Modify: `.github/workflows/release.yml` - replace the tag-triggered workflow with ordered Release Please and GoReleaser jobs.
- Modify: `.github/workflows/ci.yml` - validate non-bot PR titles before merge.
- Create: `release-please-config.json` - root component release policy and GitHub Release ownership.
- Create: `.release-please-manifest.json` - baseline version for the repository root.
- Create: `CHANGELOG.md` - public release history and Release Please-managed release entries.
- Modify: `.goreleaser.yaml` - draft-only publishing, retry-safe upload behavior, and no independently generated changelog.
- Modify: `README.md` and `README.ru.md` - contributor-facing Conventional Commit and release procedure.
- Create: `scripts/check-release-config.sh` - deterministic validation for release configuration invariants without secrets or network publication.

### Task 1: Add Release Please Metadata and Historical Changelog

**Files:**
- Create: `release-please-config.json`
- Create: `.release-please-manifest.json`
- Create: `CHANGELOG.md`
- Create: `scripts/check-release-config.sh`

**Interfaces:**
- Consumes: existing tags `v0.1.0` and `v0.1.1`.
- Produces: manifest root entry `".": "0.1.1"`; configuration consumed by the workflow in Task 2; `scripts/check-release-config.sh` exits zero only when the policy invariants hold.

**Acceptance Criteria:**
- Release Please treats `0.1.1` as the root component's last release and uses `v`-prefixed tags.
- Breaking commits below `1.0.0` produce a minor release, and Release Please does not create a GitHub Release.
- `CHANGELOG.md` records `0.1.0` and `0.1.1` with their existing release URLs.
- The validation script rejects a missing baseline, incorrect pre-major policy, missing GitHub Release skip, or a changelog without both historical headings.

- [ ] **Step 1: Create the failing configuration-validation script**

Create `scripts/check-release-config.sh`:

```sh
#!/usr/bin/env sh
set -eu

config=release-please-config.json
manifest=.release-please-manifest.json
changelog=CHANGELOG.md

test -f "$config"
test -f "$manifest"
test -f "$changelog"

grep -F '"release-type": "simple"' "$config" >/dev/null
grep -F '"tag-separator": ""' "$config" >/dev/null
grep -F '"include-v-in-tag": true' "$config" >/dev/null
grep -F '"bump-minor-pre-major": true' "$config" >/dev/null
grep -F '"skip-github-release": true' "$config" >/dev/null
grep -F '".": "0.1.1"' "$manifest" >/dev/null
grep -F '## [0.1.1]' "$changelog" >/dev/null
grep -F '## [0.1.0]' "$changelog" >/dev/null
```

- [ ] **Step 2: Run it to verify it fails before the metadata exists**

Run: `sh scripts/check-release-config.sh`

Expected: non-zero exit because `release-please-config.json` does not exist.

- [ ] **Step 3: Add the root manifest and Release Please configuration**

Create `.release-please-manifest.json`:

```json
{
  ".": "0.1.1"
}
```

Create `release-please-config.json`:

```json
{
  "packages": {
    ".": {
      "release-type": "simple",
      "component": "tgc",
      "include-v-in-tag": true,
      "tag-separator": "",
      "bump-minor-pre-major": true,
      "skip-github-release": true
    }
  }
}
```

- [ ] **Step 4: Add the historical changelog**

Create `CHANGELOG.md`:

```markdown
# Changelog

All notable changes to tgc are documented in this file.

## [0.1.1] - 2026-07-18

### Added

- Local `./.tgc` project configuration, `tgc init`, and `tgc config path`.

### Fixed

- Local configuration discovery stops before `$HOME`.

## [0.1.0] - 2026-07-18

### Added

- First public release of the agent-first Telegram CLI, including installation and self-update support.

[0.1.1]: https://github.com/grigoreo-dev/tgc/releases/tag/v0.1.1
[0.1.0]: https://github.com/grigoreo-dev/tgc/releases/tag/v0.1.0
```

- [ ] **Step 5: Run the configuration validation successfully**

Run: `sh scripts/check-release-config.sh`

Expected: exit 0 with no output.

- [ ] **Step 6: Commit the release metadata**

```bash
git add release-please-config.json .release-please-manifest.json CHANGELOG.md scripts/check-release-config.sh
git commit -m "chore(release): add release please metadata"
```

### Task 2: Replace the Tag Workflow with the Unified Release Pipeline

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `.goreleaser.yaml`
- Test: `scripts/check-release-config.sh`

**Interfaces:**
- Consumes: `release-please-config.json`, `.release-please-manifest.json`, `steps.release.outputs.release_created`, and `steps.release.outputs.tag_name` from Release Please.
- Produces: a `vX.Y.Z` tag, a draft GitHub Release, `tgc_*.tar.gz` assets, and `checksums.txt` published only after GoReleaser succeeds.

**Acceptance Criteria:**
- Only one workflow file manages release creation and is triggered on pushes to `main`.
- GoReleaser runs only when Release Please reports a new release, checks out the exact `tag_name`, and has full tag history.
- A workflow retry reuses a draft release and replaces conflicting assets rather than producing a second version.
- Release notes come from the Release Please-maintained changelog rather than GoReleaser's commit scraper.

- [ ] **Step 1: Write an expected workflow contract in the validation script**

Append to `scripts/check-release-config.sh`:

```sh
workflow=.github/workflows/release.yml
goreleaser=.goreleaser.yaml

grep -F 'branches: [main]' "$workflow" >/dev/null
grep -F 'googleapis/release-please-action@v5' "$workflow" >/dev/null
grep -F 'release_created:' "$workflow" >/dev/null
grep -F 'tag_name:' "$workflow" >/dev/null
grep -F "needs.release-please.outputs.release_created == 'true'" "$workflow" >/dev/null
grep -F 'ref: ${{ needs.release-please.outputs.tag_name }}' "$workflow" >/dev/null
grep -F 'draft: true' "$goreleaser" >/dev/null
grep -F 'use_existing_draft: true' "$goreleaser" >/dev/null
grep -F 'replace_existing_artifacts: true' "$goreleaser" >/dev/null
```

- [ ] **Step 2: Verify the expanded script fails against the old tag-triggered workflow**

Run: `sh scripts/check-release-config.sh`

Expected: non-zero exit because `.github/workflows/release.yml` still triggers on tags and does not contain the Release Please job.

- [ ] **Step 3: Replace the release workflow with two ordered jobs**

Replace `.github/workflows/release.yml` with:

```yaml
name: Release

on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  release-please:
    runs-on: ubuntu-latest
    outputs:
      release_created: ${{ steps.release.outputs.release_created }}
      tag_name: ${{ steps.release.outputs.tag_name }}
    steps:
      - id: release
        uses: googleapis/release-please-action@v5
        with:
          command: manifest
          token: ${{ secrets.GITHUB_TOKEN }}
          config-file: release-please-config.json
          manifest-file: .release-please-manifest.json
          target-branch: main

  goreleaser:
    needs: release-please
    if: needs.release-please.outputs.release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0
          ref: ${{ needs.release-please.outputs.tag_name }}
      - uses: actions/setup-go@v6
        with:
          go-version: '1.25'
      - uses: goreleaser/goreleaser-action@v7
        with:
          distribution: goreleaser
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      - name: Publish completed release
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: gh release edit "${{ needs.release-please.outputs.tag_name }}" --draft=false
```

- [ ] **Step 4: Make GoReleaser draft and retry-safe**

Replace the end of `.goreleaser.yaml` with:

```yaml
release:
  github:
    owner: grigoreo-dev
    name: tgc
  draft: true
  use_existing_draft: true
  replace_existing_artifacts: true
  mode: keep-existing
changelog:
  disable: true
```

`draft: true` prevents `releases/latest` and `tgc self update` from observing a partially uploaded release. The `gh release edit --draft=false` step above is required after GoReleaser succeeds; without it, every successful release would remain a draft.

- [ ] **Step 5: Verify the release contracts**

Run: `sh scripts/check-release-config.sh && docker run --rm -v "$PWD":/work -w /work goreleaser/goreleaser:v2 check --config .goreleaser.yaml`

Expected: exit 0. The configuration checker prints nothing; the GoReleaser v2 container reports a valid configuration.

- [ ] **Step 6: Commit the unified pipeline**

```bash
git add .github/workflows/release.yml .goreleaser.yaml scripts/check-release-config.sh
git commit -m "ci(release): unify release please and goreleaser"
```

### Task 3: Enforce Conventional Commit Inputs on Pull Requests

**Files:**
- Modify: `.github/workflows/ci.yml`
- Test: `.github/workflows/ci.yml` job output through `actionlint`

**Interfaces:**
- Consumes: `github.event.pull_request.title`, `github.event.pull_request.user.login`, and `github.event_name`.
- Produces: a required CI job named `conventional-commit-title` that rejects invalid ordinary PR titles and skips Release Please bot PRs.

**Acceptance Criteria:**
- PR titles accept Conventional Commit types with an optional scope and optional breaking `!` marker.
- Invalid titles fail before merge.
- Pushes to `main` and PRs authored by `release-please[bot]` do not fail because they have no ordinary contributor title to validate.

- [ ] **Step 1: Add a failing static expectation to the release configuration script**

Append to `scripts/check-release-config.sh`:

```sh
ci=.github/workflows/ci.yml
grep -F 'conventional-commit-title:' "$ci" >/dev/null
grep -F 'release-please[bot]' "$ci" >/dev/null
grep -F '^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\\([^)]+\\))?!:' "$ci" >/dev/null
```

- [ ] **Step 2: Verify the expectation fails before adding the job**

Run: `sh scripts/check-release-config.sh`

Expected: non-zero exit because `conventional-commit-title` does not exist.

- [ ] **Step 3: Add the title-validation job to CI**

Append this job to `.github/workflows/ci.yml`:

```yaml
  conventional-commit-title:
    if: github.event_name == 'pull_request' && github.event.pull_request.user.login != 'release-please[bot]'
    runs-on: ubuntu-latest
    steps:
      - name: Validate Conventional Commit title
        env:
          TITLE: ${{ github.event.pull_request.title }}
        run: |
          printf '%s' "$TITLE" | grep -Eq '^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\([^)]+\))?!: .+|^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\([^)]+\))?: .+'
```

- [ ] **Step 4: Validate workflow syntax and both accepted title forms locally**

Run:

```bash
printf '%s' 'feat: add release automation' | grep -Eq '^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\([^)]+\))?!: .+|^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\([^)]+\))?: .+'
printf '%s' 'feat(cli)!: change output contract' | grep -Eq '^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\([^)]+\))?!: .+|^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\([^)]+\))?: .+'
! printf '%s' 'update readme' | grep -Eq '^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\([^)]+\))?!: .+|^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\([^)]+\))?: .+'
sh scripts/check-release-config.sh
```

Expected: the first two checks succeed, the invalid title check returns non-zero, and the configuration script exits 0.

- [ ] **Step 5: Commit PR input validation**

```bash
git add .github/workflows/ci.yml scripts/check-release-config.sh
git commit -m "ci: validate conventional commit PR titles"
```

### Task 4: Document the Maintainer Release Procedure and Validate the Repository

**Files:**
- Modify: `README.md`
- Modify: `README.ru.md`
- Modify: `scripts/check-release-config.sh`
- Test: `README.md`, `README.ru.md`, GitHub workflow YAML, GoReleaser config, full Go checks.

**Interfaces:**
- Consumes: generated Release PR process from Tasks 1-3.
- Produces: maintainer instructions that say to merge the bot-generated Release PR rather than create a tag manually.

**Acceptance Criteria:**
- English and Russian contributor documentation state that `CHANGELOG.md` and release version are generated from Conventional Commits.
- Documentation gives the exact release sequence and says `v1.0.0` is an explicit decision.
- Repository validation covers release configuration, workflow syntax, Go build/vet/test/lint, and `install.sh` shellcheck.

- [ ] **Step 1: Add failing documentation anchors to the validation script**

Append to `scripts/check-release-config.sh`:

```sh
grep -F 'Release Please' README.md >/dev/null
grep -F 'Release PR' README.md >/dev/null
grep -F 'Conventional Commits' README.md >/dev/null
grep -F 'Release Please' README.ru.md >/dev/null
grep -F 'Release PR' README.ru.md >/dev/null
```

- [ ] **Step 2: Verify it fails before the README updates**

Run: `sh scripts/check-release-config.sh`

Expected: non-zero exit because the README files do not yet document Release Please.

- [ ] **Step 3: Add the English release process to `README.md`**

Add this subsection under `## Contributing`:

```markdown
### Releases

Release versions and `CHANGELOG.md` are generated from [Conventional Commits](https://www.conventionalcommits.org/). Use squash merge and give ordinary PRs a title such as `fix: handle empty chat`, `feat: add search`, or `feat(cli)!: change the output contract`.

After releasable commits reach `main`, Release Please opens or updates a Release PR with the next version and changelog. Review that PR, then merge it when the release is ready. The workflow creates the `vX.Y.Z` tag and GoReleaser publishes the archives and `checksums.txt`; do not create release tags manually.

While tgc is below `v1.0.0`, breaking changes release the next `0.x.0` version. `v1.0.0` is an explicit maintainer decision, not an automatic consequence of a breaking-change commit.
```

- [ ] **Step 4: Add an equivalent Russian subsection to `README.ru.md`**

Add this subsection under the Russian contributing section:

```markdown
### Релизы

Версия релиза и `CHANGELOG.md` формируются из [Conventional Commits](https://www.conventionalcommits.org/). Используйте squash merge и Conventional Commit в заголовке обычного PR: `fix: handle empty chat`, `feat: add search` или `feat(cli)!: change the output contract`.

После появления релизных коммитов в `main` Release Please создаёт или обновляет Release PR с очередной версией и changelog. Проверьте этот PR и влейте его, когда релиз готов. Workflow сам создаёт тег `vX.Y.Z`, а GoReleaser публикует архивы и `checksums.txt`; вручную теги релиза не создавайте.

Пока tgc ниже `v1.0.0`, breaking changes выпускаются как следующая версия `0.x.0`. Переход на `v1.0.0` - отдельное решение мейнтейнера, а не автоматическое следствие `feat!`.
```

- [ ] **Step 5: Run all release and project validation**

Run:

```bash
sh scripts/check-release-config.sh
docker run --rm -v "$PWD":/work -w /work goreleaser/goreleaser:v2 check --config .goreleaser.yaml
go build ./...
go vet ./...
go test ./...
golangci-lint run
shellcheck install.sh scripts/check-release-config.sh
```

Expected: every command exits 0.

- [ ] **Step 6: Run the first Release Please dry-run before enabling publication**

Run from a clean checkout with a non-production GitHub token that can read the repository but cannot publish:

```bash
npx release-please manifest-pr --repo-url=grigoreo-dev/tgc --target-branch=main --config-file=release-please-config.json --manifest-file=.release-please-manifest.json --dry-run
```

Expected: dry-run reports a root release proposal based on `0.1.1`, retains the pre-1.0 policy, and previews changelog entries without creating a PR, tag, or GitHub Release. Review the proposed version and changelog before merging the workflow changes.

- [ ] **Step 7: Commit documentation and validation**

```bash
git add README.md README.ru.md scripts/check-release-config.sh
git commit -m "docs: explain automated release process"
```
