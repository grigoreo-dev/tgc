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
grep -F '"skip-github-release": true' "$config" >/dev/null
grep -F '".": "0.1.1"' "$manifest" >/dev/null
grep -F '## [0.1.1]' "$changelog" >/dev/null
grep -F '## [0.1.0]' "$changelog" >/dev/null

workflow=.github/workflows/release.yml
goreleaser=.goreleaser.yaml

grep -F 'branches: [main]' "$workflow" >/dev/null
grep -F '  verify:' "$workflow" >/dev/null
grep -F 'needs: verify' "$workflow" >/dev/null
grep -F 'googleapis/release-please-action@v5' "$workflow" >/dev/null
grep -F 'release_created:' "$workflow" >/dev/null
grep -F 'tag_name:' "$workflow" >/dev/null
grep -F "needs.release-please.outputs.release_created == 'true'" "$workflow" >/dev/null
grep -F 'ref: ${{ needs.release-please.outputs.tag_name }}' "$workflow" >/dev/null
grep -F 'draft: true' "$goreleaser" >/dev/null
grep -F 'use_existing_draft: true' "$goreleaser" >/dev/null
grep -F 'replace_existing_artifacts: true' "$goreleaser" >/dev/null
