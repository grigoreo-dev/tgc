#!/usr/bin/env bash
# scripts/e2e/07-rich.sh — bot sends a rich message, user reads it rendered as Markdown.
# Proves the RichMessage->Markdown read projection: a message whose body lives in
# rich_message (empty plain text) is no longer invisible to the user.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"
# shellcheck disable=SC1091
[ -f "$HERE/.env.generated" ] && . "$HERE/.env.generated"
require_setup
BOT="@$E2E_BOT_USERNAME"; USR="$E2E_USER_ID"
OUT=$(mktemp)

# Use a bare-digit tag for the grep (the nonce contains "[e2e]"; the renderer
# correctly ESCAPES the brackets to \[e2e\] as a security measure, so we assert
# on the un-escaped numeric tag rather than the bracketed nonce).
TAG="07rich$RANDOM$RANDOM"
MD="# ${TAG}\n\n**bold line**\n\n- item one\n- item two"
b send "$USR" "rich-fallback ${TAG}" --rich "{\"type\":\"markdown\",\"markdown\":\"${MD}\"}" >/dev/null
sleep 4
# read --limit 1 emits ONE JSON object per line (not an array); jqf takes the
# first line's field — matches 01-send-read.sh convention. NO .[0].
# Retry a few times so a slow delivery doesn't flake the assertion.
# NOTE: rich text is MULTI-LINE. lib.sh's jqf pipes through `head -1`, which
# would chop a rendered rich message to just its first line — so read the .text
# with jq directly (first JSON line only, since read emits one object per line).
GOT=""
for _ in 1 2 3 4 5 6; do
  u read "$BOT" --limit 1 > "$OUT" 2>&1
  GOT=$(head -1 "$OUT" | jq -r '.text' 2>/dev/null)
  if printf '%s' "$GOT" | grep -qF -- "$TAG" && printf '%s' "$GOT" | grep -qF -- "**bold line**"; then
    break
  fi
  sleep 2
done

# text must be non-empty and contain the rendered heading + bold + list markers.
# grep uses `--` before patterns starting with '-'.
if printf '%s' "$GOT" | grep -qF -- "$TAG"; then pass "07: rich text delivered (tag present)"; else
  fail "07: rich text delivered" "tag $TAG not in text: $GOT"; fi
if printf '%s' "$GOT" | grep -qF -- "**bold line**"; then pass "07: rich bold rendered"; else
  fail "07: rich bold rendered" "missing **bold line** in: $GOT"; fi
if printf '%s' "$GOT" | grep -qF -- "- item one"; then pass "07: rich list rendered"; else
  fail "07: rich list rendered" "missing '- item one' in: $GOT"; fi
if printf '%s' "$GOT" | grep -qE -- "^# "; then pass "07: rich heading rendered"; else
  fail "07: rich heading rendered" "missing '# ' heading in: $GOT"; fi

RICH=$(jqf "$OUT" '.rich')
assert_eq "07: rich flag true" "true" "$RICH"

summary
