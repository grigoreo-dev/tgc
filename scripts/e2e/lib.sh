#!/usr/bin/env bash
# scripts/e2e/lib.sh — shared helpers for the bidirectional e2e suite.
# Source this from scenario scripts: . "$(dirname "$0")/lib.sh"
set -uo pipefail   # NOT -e: scenarios collect failures rather than abort

TGC="${TGC_BIN:-tgc}"
USER_PROFILE="${E2E_USER_PROFILE:-e2euser}"
BOT_PROFILE="${E2E_BOT_PROFILE:-e2ebot}"

E2E_PASS=0; E2E_FAIL=0; E2E_SKIP=0

_run() { # $1=profile, rest=args
  local prof="$1"; shift
  if [ "${E2E_VERBOSE:-}" = "1" ] && [ "$1" != "auth" ]; then
    printf '» tgc --profile %s %s\n' "$prof" "$*" >&2
  fi
  "$TGC" --profile "$prof" "$@"
}
u() { _run "$USER_PROFILE" "$@"; }
b() { _run "$BOT_PROFILE" "$@"; }

nonce() { NONCE="[e2e] $1 $RANDOM$RANDOM"; printf '%s' "$NONCE"; }

jqf() { jq -r "$2" < "$1" 2>/dev/null | head -1; }

pass() { E2E_PASS=$((E2E_PASS+1)); printf '✓ %s\n' "$1"; }
fail() { E2E_FAIL=$((E2E_FAIL+1)); printf '✗ %s: %s\n' "$1" "${2:-}"; }
skip() { E2E_SKIP=$((E2E_SKIP+1)); printf '⊘ %s (skipped: %s)\n' "$1" "${2:-}"; }

assert_eq() { if [ "$2" = "$3" ]; then pass "$1"; else fail "$1" "want [$2] got [$3]"; fi; }
assert_nonempty() { if [ -n "${2:-}" ]; then pass "$1"; else fail "$1" "empty"; fi; }
assert_exit() { if [ "$2" = "$3" ]; then pass "$1"; else fail "$1" "want exit $2 got $3"; fi; }
assert_json() { local got; got=$(jqf "$2" "$3"); if [ "$got" = "$4" ]; then pass "$1"; else fail "$1" "filter $3 want [$4] got [$got]"; fi; }
assert_error() { local got; got=$(jqf "$2" '.error'); if [ "$got" = "$3" ]; then pass "$1"; else fail "$1" "want error [$3] got [$got]"; fi; }

await_bg() { # profile chat flags outfile  -> echoes PID
  local prof="$1" chat="$2" flags="$3" out="$4"
  # shellcheck disable=SC2086
  "$TGC" --profile "$prof" await "$chat" $flags > "$out" 2>&1 &
  echo $!
}

retry_recv() { # n cmd...  ; retries cmd until it produces stdout or n exhausted
  local n="$1"; shift
  local i=0 out=""
  while [ "$i" -le "$n" ]; do
    out="$("$@")"; [ -n "$out" ] && { printf '%s' "$out"; return 0; }
    i=$((i+1)); sleep 1
  done
  return 1
}

cleanup_msg() { _run "$1" delete "$2" "$3" >/dev/null 2>&1 || true; }

# require_setup: guard a scenario so a missing setup (.env.generated not sourced)
# yields a clean SKIP instead of a `set -u` "unbound variable" abort.
require_setup() {
  if [ -z "${E2E_BOT_USERNAME:-}" ] || [ -z "${E2E_USER_ID:-}" ]; then
    skip "$(basename "$0")" "setup not run (.env.generated missing) — run setup.sh first"
    summary; exit 0
  fi
}

summary() {
  printf '\nPASSED: %d, FAILED: %d, SKIPPED: %d\n' "$E2E_PASS" "$E2E_FAIL" "$E2E_SKIP"
  [ "$E2E_FAIL" -eq 0 ]
}

# --- at bottom of lib.sh, temporary RED harness ---
if [ "${E2E_SELFTEST:-}" = "1" ]; then
  assert_eq "eq-pass" "x" "x"
  assert_eq "eq-fail" "x" "y"           # expect one ✗
  n1=$(nonce demo); n2=$(nonce demo); assert_nonempty "nonce" "$n1"
  if [ "$n1" != "$n2" ]; then pass "nonce-unique"; else fail "nonce-unique" "collision"; fi
  echo '{"error":"bot_unsupported"}' > /tmp/e2e_st.json
  assert_error "err" /tmp/e2e_st.json bot_unsupported
  echo '{"status":"timeout","waited":5}' > /tmp/e2e_st2.json
  assert_json "json" /tmp/e2e_st2.json '.waited' 5
  summary
fi
