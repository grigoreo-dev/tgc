#!/usr/bin/env bash
# scripts/e2e/run-all.sh — run the full bidirectional e2e suite.
# Takes an exclusive flock (one run per bot profile), runs setup (fail-fast),
# then selftest + all scenarios via run_scenario, aggregating PASS/FAIL/SKIP.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"

LOCK="/tmp/tgc-e2e-${BOT_PROFILE}.lock"
exec 9>"$LOCK"
if ! flock -n 9; then
  echo "an e2e run is already active on profile '$BOT_PROFILE' — wait or use another profile" >&2
  exit 3
fi

bash "$HERE/setup.sh" || { echo "setup failed — aborting" >&2; exit 2; }
# shellcheck disable=SC1091
. "$HERE/.env.generated"

TP=0; TF=0; TS=0
run_scenario "selftest" "$HERE/selftest.sh"
for s in 01-send-read 02-await 03-media 04-dialogs 05-bot-limits 06-meta 07-rich; do
  run_scenario "$s" "$HERE/$s.sh"
done
printf '\n======== TOTAL ========\nPASSED: %d, FAILED: %d, SKIPPED: %d\n' "$TP" "$TF" "$TS"
[ "$TF" -eq 0 ]
