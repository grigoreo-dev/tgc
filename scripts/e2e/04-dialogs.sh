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

# Seed a message that embeds the bot username so dual default can hit both sections.
N_dual=$(nonce 04dual); u send "$BOT" "$N_dual $E2E_BOT_USERNAME" >/dev/null 2>&1; sleep 2
u search "$E2E_BOT_USERNAME" --limit 10 > "$T" 2>&1
assert_json "04: dual has chat" "$T" 'select(.result=="chat") | .result' "chat"
assert_json "04: dual has message" "$T" 'select(.result=="message") | .result' "message"
assert_json "04: search finds bot" "$T" 'select(.result=="chat" and .username=="'"$E2E_BOT_USERNAME"'") | .username' "$E2E_BOT_USERNAME"

# --type messages: global message search for a unique nonce (exact text match)
N=$(nonce 04msg); u send "$BOT" "$N" >/dev/null 2>&1; sleep 2
u search "$N" --type messages > "$T" 2>&1
assert_json "04: global message search" "$T" 'select(.result=="message") | .text' "$N"

# --type chats: only chat rows (no message discriminator)
u search "$E2E_BOT_USERNAME" --type chats --limit 10 > "$T" 2>&1
assert_json "04: type chats finds bot" "$T" 'select(.username=="'"$E2E_BOT_USERNAME"'") | .result' "chat"
MSG_ROW=$(jqf "$T" 'select(.result=="message") | .result')
assert_eq "04: type chats no messages" "" "$MSG_ROW"

# --type posts: invalid type → bad_args pre-connect
u search x --type posts > "$T" 2>&1; ec=$?
assert_error "04: type posts bad_args" "$T" bad_args
assert_exit "04: type posts exit 1" 1 "$ec"

u read "$BOT" --limit 3 > "$T" 2>&1
assert_nonempty "04: read limit" "$(cat "$T")"

summary
