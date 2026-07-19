#!/usr/bin/env bash
# scripts/e2e/06-meta.sh — meta/introspection commands (auth/config/version/
# self-check/--pretty).
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"
# shellcheck disable=SC1091
[ -f "$HERE/.env.generated" ] && . "$HERE/.env.generated"
require_setup
T=$(mktemp)

u auth list > "$T" 2>&1
assert_nonempty "06: auth list" "$(cat "$T")"

u config path > "$T" 2>&1
assert_nonempty "06: config path source" "$(jqf "$T" '.source')"

u version > "$T" 2>&1
assert_nonempty "06: version" "$(jqf "$T" '.version')"

u self check > "$T" 2>&1
if [ -n "$(jqf "$T" '.current')" ] || [ "$(jqf "$T" '.error')" = "rate_limited" ]; then
  pass "06: self check (soft)"
else
  fail "06: self check (soft)" "$(cat "$T")"
fi

# --pretty must NOT be machine JSON. Test by actually parsing: valid JSON on the
# first line = failure. (A crude '^{' check is wrong — Go's default struct
# rendering, e.g. "{8902 0 user ...}", also starts with '{' but is not JSON.)
u chats --pretty --limit 1 > "$T" 2>&1
if head -1 "$T" | jq -e . >/dev/null 2>&1; then
  fail "06: --pretty is non-JSON" "got parseable JSON: $(head -1 "$T")"
else
  pass "06: --pretty is non-JSON"
fi

summary
