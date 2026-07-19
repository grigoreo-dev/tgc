#!/usr/bin/env bash
# scripts/e2e/02-await.sh — await scenarios, user + bot(live-only).
# The bot's ONLY inbound path is `await` (bots cannot read/context — dialog-based,
# returns bot_unsupported), so the bot verifies receipt via its await outfile, NOT `b read`.
# Every await uses --timeout <=20; nonce-filter before count/order asserts so a
# warm-up or unrelated message can't skew results.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"
# shellcheck disable=SC1091
[ -f "$HERE/.env.generated" ] && . "$HERE/.env.generated"
require_setup
BOT="@$E2E_BOT_USERNAME"; USR="$E2E_USER_ID"
OUT=$(mktemp)

# --- user await catches a 3-message burst from the bot ---
await_bg "$USER_PROFILE" "$BOT" "--timeout 20 --debounce 3" "$OUT"; PID=$AWAIT_PID
sleep 3
Na=$(nonce 02a); b send "$USR" "${Na}-1" >/dev/null; sleep 0.4
b send "$USR" "${Na}-2" >/dev/null; sleep 0.4
b send "$USR" "${Na}-3" >/dev/null
wait "$PID" 2>/dev/null
# Filter to OUR nonce only — a warm-up or other message must not skew the check.
MINE=$(mktemp); grep -F "$Na" "$OUT" > "$MINE" || true
LINES=$(grep -c '"id"' "$MINE" || true)
assert_eq "02: user await burst count" 3 "$LINES"
# oldest->newest within our messages: first is -1, last is -3
FIRST=$(head -1 "$MINE" | jq -r '.text'); LAST=$(tail -1 "$MINE" | jq -r '.text')
assert_eq "02: burst first is -1" "${Na}-1" "$FIRST"
assert_eq "02: burst last is -3"  "${Na}-3" "$LAST"

# --- await timeout on silence ---
u await "$BOT" --timeout 4 --debounce 1 > "$OUT" 2>&1; tec=$?
assert_json "02: timeout marker" "$OUT" '.status' "timeout"
assert_exit "02: timeout exit 0" 0 "$tec"

# --- user send --await-reply round-trip (bot catches + replies) ---
BOUT=$(mktemp); await_bg "$BOT_PROFILE" "$USR" "--timeout 20 --debounce 1" "$BOUT"; BPID=$AWAIT_PID
sleep 3
Nr=$(nonce 02r)
( u send "$BOT" "$Nr ping" --await-reply --await-timeout 20 --await-debounce 2 > "$OUT" 2>&1 ) &
UPID=$!
wait "$BPID" 2>/dev/null              # bot got the user's ping
RMID=$(jqf "$BOUT" '.id')
if [ -n "${RMID:-}" ]; then
  b send "$USR" "$Nr pong" >/dev/null # bot replies
fi
wait "$UPID" 2>/dev/null
# user's --await-reply output has the send line then the pong line
assert_json "02: await-reply got pong" "$OUT" 'select(.text != null) | select(.text|test("pong")) | .text' "$Nr pong"

# --- bot await (live-only) catches a user message ---
BOUT2=$(mktemp); await_bg "$BOT_PROFILE" "$USR" "--timeout 20 --debounce 1" "$BOUT2"; BPID2=$AWAIT_PID
sleep 3
Nb=$(nonce 02b); u send "$BOT" "$Nb" >/dev/null
wait "$BPID2" 2>/dev/null
if grep -q "$Nb" "$BOUT2"; then pass "02: bot await catches user"; else
  # mandatory retry once with a fresh nonce
  await_bg "$BOT_PROFILE" "$USR" "--timeout 20 --debounce 1" "$BOUT2"; BPID3=$AWAIT_PID; sleep 3
  Nb2=$(nonce 02b-r); u send "$BOT" "$Nb2" >/dev/null; wait "$BPID3" 2>/dev/null
  if grep -q "$Nb2" "$BOUT2"; then
    pass "02: bot await catches user (retry)"
  else
    fail "02: bot await catches user" "missed twice"
  fi
fi

summary
