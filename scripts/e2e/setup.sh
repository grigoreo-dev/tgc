#!/usr/bin/env bash
# scripts/e2e/setup.sh — prepare and validate the e2e user + bot profiles.
set -uo pipefail
set +x   # defense-in-depth: never let an operator-forced `bash -x` trace the token
HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"

die2() { printf 'SETUP ERROR: %s\n' "$1" >&2; exit 2; }

command -v jq >/dev/null || die2 "jq not found"
command -v "$TGC" >/dev/null 2>&1 || [ -x "$TGC" ] || die2 "tgc binary not found ($TGC)"
# Presence check: :+set expands to a constant, never the token (safe under xtrace).
[ -n "${TGC_BOT_TOKEN:+set}" ] || die2 "TGC_BOT_TOKEN not set (from .env / env)"

# Default-collision guard
if [ "$USER_PROFILE" = "default" ] && [ "${E2E_ALLOW_DEFAULT:-}" != "1" ]; then
  die2 "refusing to use the 'default' profile as e2e user; set E2E_ALLOW_DEFAULT=1 to override"
fi

# User session must already exist (never interactive-login here)
if ! u auth list >/dev/null 2>&1 || [ -z "$(u auth list 2>/dev/null)" ]; then
  die2 "user profile '$USER_PROFILE' has no session; log in once: tgc --profile $USER_PROFILE auth login"
fi

# Bot login (idempotent, non-interactive, token never traced).
# NOTE: use the `me` selector (NOT `@self` — the `@` makes it a *username*
# lookup that resolves to the wrong peer; `me`/`self`/`saved` map to the
# account's own self).
if ! b info me >/dev/null 2>&1; then
  set +x
  "$TGC" --profile "$BOT_PROFILE" auth login --bot-token "$TGC_BOT_TOKEN" >/dev/null 2>&1 \
    || die2 "bot login failed"
fi

# Resolve ids via the `me` selector.
BOT_CARD="$(mktemp)"; b info me > "$BOT_CARD" 2>/dev/null || true
E2E_BOT_ID="$(jqf "$BOT_CARD" '.id')"
E2E_BOT_USERNAME="$(jqf "$BOT_CARD" '.username')"
USER_CARD="$(mktemp)"; u info me > "$USER_CARD" 2>/dev/null || true
E2E_USER_ID="$(jqf "$USER_CARD" '.id')"

[ -n "$E2E_BOT_USERNAME" ] || die2 "could not resolve bot username"
[ -n "$E2E_USER_ID" ] || die2 "could not resolve user id"

# Mutual reachability
u info "@$E2E_BOT_USERNAME" >/dev/null 2>&1 || die2 "user cannot see bot @$E2E_BOT_USERNAME"
if ! b info "$E2E_USER_ID" >/dev/null 2>&1; then
  die2 "bot cannot see user $E2E_USER_ID — the user must message @$E2E_BOT_USERNAME once (press Start)"
fi

printf 'e2e accounts:\n  user: %s (id %s)\n  bot:  @%s (id %s)\n' \
  "$USER_PROFILE" "$E2E_USER_ID" "$E2E_BOT_USERNAME" "$E2E_BOT_ID"

# Warm-up round-trip (absorbs first-connect latency)
WU="$(mktemp)"; await_bg "$BOT_PROFILE" "$E2E_USER_ID" "--timeout 15 --debounce 1" "$WU"; PID=$AWAIT_PID
sleep 3
u send "@$E2E_BOT_USERNAME" "$(nonce warmup)" >/dev/null 2>&1 || true
wait "$PID" 2>/dev/null || true

# Export for scenarios
{
  echo "export E2E_BOT_USERNAME='$E2E_BOT_USERNAME'"
  echo "export E2E_BOT_ID='$E2E_BOT_ID'"
  echo "export E2E_USER_ID='$E2E_USER_ID'"
} > "$HERE/.env.generated"
printf 'setup OK\n'
