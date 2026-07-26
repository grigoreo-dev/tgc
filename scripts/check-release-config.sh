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
grep -F 'workflow_dispatch:' "$workflow" >/dev/null
grep -F 'tag_name:' "$workflow" >/dev/null
grep -F '  validate-retry:' "$workflow" >/dev/null
grep -F '^v[0-9]+\.[0-9]+\.[0-9]+$' "$workflow" >/dev/null
grep -F '  verify:' "$workflow" >/dev/null
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
