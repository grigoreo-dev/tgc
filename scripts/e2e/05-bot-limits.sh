#!/usr/bin/env bash
# scripts/e2e/05-bot-limits.sh — bot-profile forbidden surface (dialog-based
# commands are user-only for bots). Documents the exact error contract.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"
# shellcheck disable=SC1091
[ -f "$HERE/.env.generated" ] && . "$HERE/.env.generated"
require_setup
T=$(mktemp)

b chats > "$T" 2>&1; ec=$?
assert_error "05: bot chats bot_unsupported" "$T" bot_unsupported
assert_exit "05: bot chats exit 1" 1 "$ec"

b search "x" > "$T" 2>&1; ec=$?
assert_error "05: bot search bot_unsupported" "$T" bot_unsupported
assert_exit "05: bot search exit 1" 1 "$ec"

if [ -n "${E2E_BOT_GROUP:-}" ]; then
  b members -- "$E2E_BOT_GROUP" > "$T" 2>&1
  assert_nonempty "05: bot members (admin group)" "$(cat "$T")"
else
  b members -- "-1002260849662" > "$T" 2>&1; ec=$?
  assert_error "05: bot members not_found" "$T" not_found
  assert_exit "05: bot members exit 1" 1 "$ec"
fi

summary
