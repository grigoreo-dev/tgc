#!/usr/bin/env bash
# scripts/e2e/selftest.sh — offline harness regression checks (no network).
# Runs through the same summary aggregation path as live scenarios.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"

# --- nonce is Markdown-projection-safe for every scenario tag style ---
N="$(nonce selftest)"
case "$N" in
  *['`*_[]|~=']*|'# '*|'>'*|'- '*|'+ '*|[0-9]*'. '*)
    fail "selftest: nonce is Markdown-projection-safe" "got [$N]" ;;
  *) pass "selftest: nonce is Markdown-projection-safe" ;;
esac

# --- missing/empty TGC_BOT_TOKEN: exit 2 + diagnostic, no token leak under xtrace ---
OUT="$(mktemp)"
env -u TGC_BOT_TOKEN TGC_BIN=true bash "$HERE/setup.sh" >"$OUT" 2>&1
RC=$?
assert_exit "selftest: missing bot token exits 2" 2 "$RC"
if grep -qF "SETUP ERROR: TGC_BOT_TOKEN not set" "$OUT"; then
  pass "selftest: missing bot token is diagnosed"
else
  fail "selftest: missing bot token is diagnosed" "$(tr '\n' ' ' < "$OUT")"
fi

FAKE_TOKEN="super-secret-token-value-do-not-leak"
XOUT="$(mktemp)"
# Forced xtrace must not print a non-empty token (presence uses :+set, set +x before use).
env TGC_BOT_TOKEN="$FAKE_TOKEN" TGC_BIN=/nonexistent/tgc-bin-for-selftest \
  bash -x "$HERE/setup.sh" >"$XOUT" 2>&1 || true
if grep -Fq -- "$FAKE_TOKEN" "$XOUT"; then
  fail "selftest: bot token not leaked under xtrace" "token appeared in trace"
else
  pass "selftest: bot token not leaked under xtrace"
fi

# --- run_scenario aggregation: only valid summary+rc0+FAILED0 is failure-free ---
TMPDIR_ST="$(mktemp -d)"
cleanup_st() { rm -rf "$TMPDIR_ST" "$OUT" "$XOUT"; }
trap cleanup_st EXIT

# valid summary, exit 0
cat >"$TMPDIR_ST/ok.sh" <<'EOF'
#!/usr/bin/env bash
echo "✓ ok case"
printf '\nPASSED: 2, FAILED: 0, SKIPPED: 1\n'
exit 0
EOF

# exit 1 after a summary that reports FAILED: 0 (must still add a failure)
cat >"$TMPDIR_ST/rc1-sum.sh" <<'EOF'
#!/usr/bin/env bash
printf '\nPASSED: 1, FAILED: 0, SKIPPED: 0\n'
exit 1
EOF

# exit 1 without summary
cat >"$TMPDIR_ST/rc1-nosum.sh" <<'EOF'
#!/usr/bin/env bash
echo "aborted mid-scenario"
exit 1
EOF

# exit 0 without summary (silent success must not be trusted)
cat >"$TMPDIR_ST/rc0-nosum.sh" <<'EOF'
#!/usr/bin/env bash
echo "ran but forgot summary"
exit 0
EOF

chmod +x "$TMPDIR_ST"/*.sh

# Capture run_scenario effects in a subshell so selftest counters stay clean.
run_case() {
  # $1=name $2=script-or-missing $3=expect-fail-free (0|1)
  local name="$1" script="$2" want_clean="$3"
  local sub_out sub_rc tp tf ts
  sub_out="$(mktemp)"
  (
    TP=0; TF=0; TS=0
    set +e
    run_scenario "$name" "$script"
    printf 'TP=%s TF=%s TS=%s\n' "$TP" "$TF" "$TS"
  ) >"$sub_out" 2>&1
  sub_rc=$?
  # Counters are printed on one line: TP=N TF=N TS=N
  tp=$(grep -Eo 'TP=[0-9]+' "$sub_out" | tail -1 | cut -d= -f2)
  tf=$(grep -Eo 'TF=[0-9]+' "$sub_out" | tail -1 | cut -d= -f2)
  ts=$(grep -Eo 'TS=[0-9]+' "$sub_out" | tail -1 | cut -d= -f2)
  tp=${tp:-0}; tf=${tf:-0}; ts=${ts:-0}

  if [ "$want_clean" = "1" ]; then
    if [ "$tf" -eq 0 ] && [ "$tp" -gt 0 ]; then
      pass "selftest: run_scenario $name is failure-free"
    else
      fail "selftest: run_scenario $name is failure-free" "TP=$tp TF=$tf TS=$ts rc=$sub_rc out=$(tr '\n' ' ' <"$sub_out")"
    fi
  else
    if [ "$tf" -ge 1 ]; then
      pass "selftest: run_scenario $name adds failure"
    else
      fail "selftest: run_scenario $name adds failure" "TP=$tp TF=$tf TS=$ts (expected TF>=1) out=$(tr '\n' ' ' <"$sub_out")"
    fi
  fi
  rm -f "$sub_out"
}

if ! declare -F run_scenario >/dev/null 2>&1; then
  fail "selftest: run_scenario exists" "function missing from lib.sh"
  # Still emit expected failure assertions so RED is complete and stable.
  fail "selftest: run_scenario ok is failure-free" "run_scenario missing"
  fail "selftest: run_scenario rc1-sum adds failure" "run_scenario missing"
  fail "selftest: run_scenario rc1-nosum adds failure" "run_scenario missing"
  fail "selftest: run_scenario rc0-nosum adds failure" "run_scenario missing"
  fail "selftest: run_scenario missing-path adds failure" "run_scenario missing"
else
  pass "selftest: run_scenario exists"
  run_case "ok" "$TMPDIR_ST/ok.sh" 1
  run_case "rc1-sum" "$TMPDIR_ST/rc1-sum.sh" 0
  run_case "rc1-nosum" "$TMPDIR_ST/rc1-nosum.sh" 0
  run_case "rc0-nosum" "$TMPDIR_ST/rc0-nosum.sh" 0
  run_case "missing-path" "$TMPDIR_ST/does-not-exist.sh" 0
fi

summary
