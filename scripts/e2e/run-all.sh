#!/usr/bin/env bash
# scripts/e2e/run-all.sh — run the full bidirectional e2e suite.
# Takes an exclusive flock (one run per bot profile), runs setup (fail-fast),
# then all scenarios, aggregating PASS/FAIL/SKIP into a final total.
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
for s in 01-send-read 02-await 03-media 04-dialogs 05-bot-limits 06-meta 07-rich; do
  echo "===== $s ====="
  out="$(bash "$HERE/$s.sh")"; echo "$out"
  p=$(echo "$out" | sed -n 's/.*PASSED: \([0-9]*\).*/\1/p' | tail -1)
  f=$(echo "$out" | sed -n 's/.*FAILED: \([0-9]*\).*/\1/p' | tail -1)
  k=$(echo "$out" | sed -n 's/.*SKIPPED: \([0-9]*\).*/\1/p' | tail -1)
  TP=$((TP+${p:-0})); TF=$((TF+${f:-0})); TS=$((TS+${k:-0}))
done
printf '\n======== TOTAL ========\nPASSED: %d, FAILED: %d, SKIPPED: %d\n' "$TP" "$TF" "$TS"
[ "$TF" -eq 0 ]
