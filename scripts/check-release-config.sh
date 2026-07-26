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
grep -F '"include-component-in-tag": false' "$config" >/dev/null
grep -F '"bump-minor-pre-major": true' "$config" >/dev/null
grep -F '"draft": true' "$config" >/dev/null
grep -F '"force-tag-creation": true' "$config" >/dev/null
# Component field forces component-scoped PR branches; keep componentless vX.Y.Z tags/branches.
if grep -Eq '"component"[[:space:]]*:' "$config"; then
  echo '"component" must not be set (use componentless root package for plain vX.Y.Z)' >&2
  exit 1
fi
# Grouped manifest PR titles default without ${version}; include it so merge can parse the release.
# shellcheck disable=SC2016 # literal ${...} tokens in Release Please title pattern
grep -F '"group-pull-request-title-pattern": "chore${scope}: release${component} ${version}"' "$config" >/dev/null
if grep -Fq 'skip-github-release' "$config"; then
  echo "skip-github-release must not be set (blocks tags/releases; see release-please#1561)" >&2
  exit 1
fi
grep -F '".": "0.1.1"' "$manifest" >/dev/null
grep -F '## [0.1.1]' "$changelog" >/dev/null
grep -F '## [0.1.0]' "$changelog" >/dev/null

workflow=.github/workflows/release.yml
goreleaser=.goreleaser.yaml

grep -F 'branches: [main]' "$workflow" >/dev/null
if grep -Fq "tags: ['v*.*.*']" "$workflow"; then
  echo "release workflow must not use the old tag trigger" >&2
  exit 1
fi
if grep -Fq 'command: manifest' "$workflow"; then
  echo "command: manifest is invalid for release-please-action@v5; use config/manifest files only" >&2
  exit 1
fi
if grep -Fq 'skip-github-release' "$workflow"; then
  echo "skip-github-release must not appear in release workflow" >&2
  exit 1
fi
# Serialize rapid main pushes; never cancel an in-flight publish.
grep -F 'concurrency:' "$workflow" >/dev/null
grep -F 'cancel-in-progress: false' "$workflow" >/dev/null
grep -F 'workflow_dispatch:' "$workflow" >/dev/null
grep -F 'tag_name:' "$workflow" >/dev/null
grep -F '  validate-retry:' "$workflow" >/dev/null
grep -F '^v[0-9]+\.[0-9]+\.[0-9]+$' "$workflow" >/dev/null
grep -F '  verify:' "$workflow" >/dev/null
# Dispatch tag must pass validate-retry before verify; never shell-expand raw inputs.tag_name.
grep -F 'needs: [validate-retry]' "$workflow" >/dev/null
grep -F 'needs.validate-retry.result == '\''success'\''' "$workflow" >/dev/null
# shellcheck disable=SC2016 # literal GitHub Actions expression, not shell expansion
grep -F 'ref: ${{ needs.validate-retry.outputs.tag_name || github.sha }}' "$workflow" >/dev/null
# Raw inputs.tag_name may only feed the validate-retry env gate (never shell run:).
tag_input_lines=$(grep -n 'inputs\.tag_name' "$workflow" || true)
if [ -z "$tag_input_lines" ]; then
  echo "validate-retry must read inputs.tag_name via env" >&2
  exit 1
fi
if printf '%s\n' "$tag_input_lines" | grep -vE 'TAG: \$\{\{ inputs\.tag_name \}\}'; then
  echo "inputs.tag_name must only appear as validate-retry env TAG; use outputs elsewhere" >&2
  exit 1
fi
if [ "$(printf '%s\n' "$tag_input_lines" | wc -l)" -ne 1 ]; then
  echo "inputs.tag_name must appear exactly once (validate-retry env)" >&2
  exit 1
fi
grep -F 'needs: verify' "$workflow" >/dev/null
grep -F 'googleapis/release-please-action@v5' "$workflow" >/dev/null
grep -F 'release_created:' "$workflow" >/dev/null
grep -F "needs.release-please.outputs.release_created == 'true'" "$workflow" >/dev/null
grep -F 'needs.validate-retry.outputs.tag_name || needs.release-please.outputs.tag_name' "$workflow" >/dev/null
grep -F 'github.event_name == '\''workflow_dispatch'\''' "$workflow" >/dev/null
grep -F 'gh release edit' "$workflow" >/dev/null
grep -F -- '--draft=false' "$workflow" >/dev/null
grep -F 'draft: true' "$goreleaser" >/dev/null
grep -F 'use_existing_draft: true' "$goreleaser" >/dev/null
grep -F 'replace_existing_artifacts: true' "$goreleaser" >/dev/null
grep -F 'mode: keep-existing' "$goreleaser" >/dev/null
grep -F 'disable: true' "$goreleaser" >/dev/null

ci=.github/workflows/ci.yml
grep -F 'conventional-commit-title:' "$ci" >/dev/null
grep -F 'release-please[bot]' "$ci" >/dev/null
# Re-run title validation when the PR title is edited (not only on code push).
grep -F 'types: [opened, synchronize, reopened, edited]' "$ci" >/dev/null
# Job skips non-PR events and Release Please bot PRs (do not weaken without decision).
grep -F "github.event_name == 'pull_request' && github.event.pull_request.user.login != 'release-please[bot]'" "$ci" >/dev/null
# Needle matches the workflow run-step ERE (single-backslash scope group).
grep -F '^(feat|fix|deps|docs|chore|test|refactor|perf|build|ci)(\([^)]+\))?!:' "$ci" >/dev/null
# Failure UX: print expected format before non-zero exit.
grep -F 'error: PR title must be a Conventional Commit' "$ci" >/dev/null
grep -F 'expected: type(scope)!: subject' "$ci" >/dev/null

# Contributor docs must describe the automated release process (Task 4).
grep -F 'Release Please' README.md >/dev/null
grep -F 'Release PR' README.md >/dev/null
grep -F 'Conventional Commits' README.md >/dev/null
grep -F 'Release Please' README.ru.md >/dev/null
grep -F 'Release PR' README.ru.md >/dev/null
# Correct ownership: RP creates tag + draft release; GoReleaser attaches assets; gh publishes.
grep -F 'draft' README.md >/dev/null
grep -F 'GoReleaser' README.md >/dev/null
grep -F 'v1.0.0' README.md >/dev/null
grep -F 'squash' README.md >/dev/null
grep -F 'conventional-commit-title' README.md >/dev/null
# Retry is only for an existing draft that is not yet published.
grep -F 'not yet published' README.md >/dev/null
grep -F 'draft' README.ru.md >/dev/null
grep -F 'GoReleaser' README.ru.md >/dev/null
grep -F 'v1.0.0' README.ru.md >/dev/null
grep -F 'squash' README.ru.md >/dev/null
grep -F 'conventional-commit-title' README.ru.md >/dev/null
grep -F 'ещё не опубликован' README.ru.md >/dev/null
