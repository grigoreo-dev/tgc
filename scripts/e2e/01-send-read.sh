#!/usr/bin/env bash
# scripts/e2e/01-send-read.sh — messaging commands, both directions.
# user->bot: send / search --chat / reply / info @bot / context / edit / forward
# bot->user: send / read (bot_unsupported, dialog-based) / info <user_id>
# Nonce-keyed assertions; cascade to `skip` (not `fail`) when a prerequisite send fails.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"
# shellcheck disable=SC1091
[ -f "$HERE/.env.generated" ] && . "$HERE/.env.generated"
require_setup
BOT="@$E2E_BOT_USERNAME"; USR="$E2E_USER_ID"
T=$(mktemp)

# user -> bot: send
N=$(nonce 01-send); u send "$BOT" "$N" > "$T"; ec=$?
MID=$(jqf "$T" '.message_id')
assert_exit "01: user send exit" 0 "$ec"
assert_nonempty "01: user send message_id" "$MID"

if [ -n "${MID:-}" ]; then
  # search in-chat by nonce (replaces removed read --search)
  u search "$N" --chat "$BOT" --limit 5 > "$T" 2>&1
  assert_json "01: user search finds nonce" "$T" 'select(.result=="message") | .text' "$N"
  # reply
  N2=$(nonce 01-reply); u send "$BOT" "$N2" --reply "$MID" > "$T"
  RID=$(jqf "$T" '.message_id')
  if [ -n "${RID:-}" ]; then
    u read "$BOT" --limit 3 > "$T" 2>&1
    # guard: RID non-empty so select(.id==$RID) is valid jq
    assert_json "01: reply_to set" "$T" 'select(.id=='"$RID"') | .reply_to' "$MID"
  else
    skip "01: reply_to set" "reply send failed"
  fi
  # info on the bot from the user side — "bot-ness" is the .bot flag, NOT .type
  # (Telegram returns bots as User objects, so info @bot has type=="user").
  u info "$BOT" > "$T" 2>&1
  assert_json "01: user info @bot has bot flag" "$T" '.bot' true
  # context
  u context "$BOT" "$MID" > "$T" 2>&1; assert_nonempty "01: context" "$(cat "$T")"
  # edit
  N3=$(nonce 01-edit); u edit "$BOT" "$MID" "$N3" > "$T" 2>&1
  u read "$BOT" --limit 5 > "$T" 2>&1
  assert_json "01: edited=true" "$T" 'select(.id=='"$MID"') | .edited' true
  # forward to Saved Messages via the `me` selector (NOT `@self`)
  u forward "$BOT" "$MID" me > "$T" 2>&1
  assert_exit "01: forward exit" 0 $?
  # cleanup
  cleanup_msg "$USER_PROFILE" "$BOT" "$MID"; cleanup_msg "$USER_PROFILE" "$BOT" "${RID:-}"
else
  skip "01: user reply/context/edit/forward" "send failed"
fi

# bot -> user: send + read + info
Nb=$(nonce 01-bot-send); b send "$USR" "$Nb" > "$T"; ec=$?
BMID=$(jqf "$T" '.message_id')
assert_exit "01: bot send exit" 0 "$ec"
assert_nonempty "01: bot send message_id" "$BMID"
if [ -n "${BMID:-}" ]; then
  # Bots cannot read/getHistory (dialog-based) — assert the clean structured error.
  b read "$USR" --limit 3 > "$T" 2>&1; rc=$?
  assert_error "01: bot read is bot_unsupported" "$T" bot_unsupported
  assert_exit  "01: bot read exit 1" 1 "$rc"
  # bot resolves the human user — a real user, so type=="user" (the bot looking
  # at a person, not itself).
  b info "$USR" > "$T" 2>&1; assert_json "01: bot resolves user (type=user)" "$T" ".type" "user"
  cleanup_msg "$BOT_PROFILE" "$USR" "$BMID"
else
  skip "01: bot read/info/delete" "bot send failed"
fi

summary
