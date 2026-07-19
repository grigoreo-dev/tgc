#!/usr/bin/env bash
# scripts/e2e/04-dialogs.sh — user-side dialog/search/read surface.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"
# shellcheck disable=SC1091
[ -f "$HERE/.env.generated" ] && . "$HERE/.env.generated"
require_setup
BOT="@$E2E_BOT_USERNAME"; T=$(mktemp)

u chats --type user --limit 5 > "$T" 2>&1; ec=$?
assert_exit "04: user chats exit 0" 0 "$ec"

u search "$E2E_BOT_USERNAME" > "$T" 2>&1
assert_json "04: search finds bot" "$T" 'select(.username=="'"$E2E_BOT_USERNAME"'") | .username' "$E2E_BOT_USERNAME"

N=$(nonce 04msg); u send "$BOT" "$N" >/dev/null 2>&1; sleep 2
u search --messages "$N" > "$T" 2>&1
assert_json "04: global message search" "$T" '.text' "$N"

u read "$BOT" --limit 3 > "$T" 2>&1
assert_nonempty "04: read limit" "$(cat "$T")"

summary
