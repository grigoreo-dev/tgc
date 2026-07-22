#!/usr/bin/env bash
# scripts/e2e/08-rich-all-types.sh — FULL-LIVE All Types rich release gate.
#
# NOT part of default run-all.sh. Sends the canonical 37-block fixture via
# rich-e2e-send, then verifies on the *recipient* (user) profile history.
#
# Why not sender message_id lookup:
#   Bot send returns a bot-local message_id; the user-visible dialog id differs
#   (live evidence: bot 257 vs user 1309). Bot profile cannot context/read the
#   dialog (bot_unsupported). Cross-profile exact-ID lookup is invalid.
#
# Gate model:
#   1. Preflight media/golden/fixture/sender
#   2. Snapshot newest recipient-side message id in bot chat (user profile)
#   3. Send via rich-e2e-send (bot-local message_id kept as diagnostic only)
#   4. Bounded poll: user `read` for messages with id > baseline
#   5. Among *only new* rows, select candidates with rich:true,
#      rich_truncated != true, and byte-exact .text == golden
#   6. Fail closed on zero or multiple matches; success requires exactly one
#   7. VERIFIED + optional cleanup use the recipient-side message id
#
# Offline mode (no Telegram): E2E_RICH_SELFTEST=1 or --selftest.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"

# shellcheck disable=SC1091
[ -f "$HERE/.env.generated" ] && . "$HERE/.env.generated"

REQUIRED_MEDIA=(
  "photo_0.jpg"
  "photo_1.jpg"
  "photo_2.jpg"
  "dubaiVideo.mp4"
  "Neon Rain Train.mp3"
)

DEFAULT_FIXTURE="$REPO_ROOT/internal/markup/testdata/richmessage_alltypes.bin"
DEFAULT_GOLDEN="$REPO_ROOT/internal/markup/testdata/richmessage_alltypes.golden.md"
DEFAULT_MEDIA_DIR="${E2E_RICH_MEDIA_DIR:-/tmp/demo_media}"
DEFAULT_BLOCKS=37
# How many newest messages to inspect per poll after baseline.
READ_LIMIT="${E2E_RICH_READ_LIMIT:-20}"
POLL_ATTEMPTS=6
POLL_SLEEP_SEC=2

die() { printf '08-rich-all-types ERROR: %s\n' "$1" >&2; exit 1; }

is_selftest() {
  [ "${E2E_RICH_SELFTEST:-}" = "1" ] || [ "${1:-}" = "--selftest" ]
}

# resolve_sender_cmd prints the argv prefix used to invoke the sender.
# Prefer RICH_E2E_SEND_BIN when set; otherwise "go run ./internal/cmd/rich-e2e-send"
# from REPO_ROOT (caller should cd or use absolute go run path).
resolve_sender_cmd() {
  if [ -n "${RICH_E2E_SEND_BIN:-}" ]; then
    if [ ! -x "$RICH_E2E_SEND_BIN" ] && [ ! -f "$RICH_E2E_SEND_BIN" ]; then
      die "sender not found: RICH_E2E_SEND_BIN=$RICH_E2E_SEND_BIN"
    fi
    printf '%s\n' "$RICH_E2E_SEND_BIN"
    return 0
  fi
  if [ -d "$REPO_ROOT/internal/cmd/rich-e2e-send" ]; then
    # go run path relative to repo root; run_sender cds there.
    printf '%s\n' "go run ./internal/cmd/rich-e2e-send"
    return 0
  fi
  die "sender not available: set RICH_E2E_SEND_BIN or provide internal/cmd/rich-e2e-send"
}

# preflight_media_dir: each required basename must be a non-empty regular file.
preflight_media_dir() {
  local dir="$1" name p
  [ -d "$dir" ] || die "media dir missing: $dir"
  for name in "${REQUIRED_MEDIA[@]}"; do
    p="$dir/$name"
    if [ ! -e "$p" ]; then
      die "media file missing: $name (under $dir)"
    fi
    if [ ! -f "$p" ]; then
      die "media file not regular: $name (under $dir)"
    fi
    if [ ! -s "$p" ]; then
      die "media file empty: $name (under $dir)"
    fi
  done
}

preflight_golden() {
  local g="$1"
  [ -f "$g" ] || die "golden missing: $g"
  [ -s "$g" ] || die "golden empty: $g"
}

preflight_fixture() {
  local f="$1"
  [ -f "$f" ] || die "fixture missing: $f"
  [ -s "$f" ] || die "fixture empty: $f"
}

# preflight: media + golden + fixture + sender resolvable (no network).
preflight() {
  local media_dir="$1" golden="$2" fixture="$3"
  preflight_media_dir "$media_dir"
  preflight_golden "$golden"
  preflight_fixture "$fixture"
  # Fail closed if sender cannot be resolved (also proves preflight runs first).
  resolve_sender_cmd >/dev/null
}

run_sender() {
  local target="$1" media_dir="$2" fixture="$3" profile="${4:-${BOT_PROFILE}}"
  local -a cmd
  local line
  line="$(resolve_sender_cmd)"
  # shellcheck disable=SC2206
  cmd=($line)
  (
    cd "$REPO_ROOT" || exit 1
    "${cmd[@]}" \
      --profile "$profile" \
      --target "$target" \
      --fixture "$fixture" \
      --media-dir "$media_dir"
  )
}

# require_positive_message_id: release-gate IDs must be positive decimal integers.
# Rejects empty, 0, leading zeros, negatives, floats, and non-digits.
require_positive_message_id() {
  local mid="${1:-}"
  case "$mid" in
    ''|*[!0-9]*|0|0[0-9]*)
      die "invalid message_id (want positive decimal integer ^[1-9][0-9]*\$): ${mid:-<empty>}"
      ;;
  esac
  if ! printf '%s' "$mid" | grep -Eq '^[1-9][0-9]*$'; then
    die "invalid message_id (want positive decimal integer ^[1-9][0-9]*\$): $mid"
  fi
}

# require_baseline_id: recipient baseline is 0 (empty dialog) or a positive decimal.
# Used after guarded snapshot assignment under top-level set -uo pipefail (no -e).
require_baseline_id() {
  local bid="${1:-}"
  if [ "$bid" = "0" ]; then
    return 0
  fi
  if ! printf '%s' "$bid" | grep -Eq '^[1-9][0-9]*$'; then
    die "invalid recipient baseline (want 0 or ^[1-9][0-9]*\$): ${bid:-<empty>}"
  fi
}

# parse_sender_message_id: extract bot-local message_id from sender stdout (diagnostic).
# Still must be a positive decimal integer; never used as recipient lookup key.
parse_sender_message_id() {
  local send_out="$1"
  local mid
  mid="$(grep -E '^\{' "$send_out" | tail -1 | jq -r '.message_id // empty' 2>/dev/null || true)"
  if [ -z "$mid" ] || [ "$mid" = "null" ]; then
    die "could not parse message_id from sender output"
  fi
  require_positive_message_id "$mid"
  printf '%s\n' "$mid"
}

# snapshot_recipient_baseline: newest message id in bot chat as seen by user profile.
# Prints 0 when the dialog is successfully empty (exit 0, no positive id).
# Nonzero u read (auth/network/bot_unsupported) aborts — never treat as baseline 0
# (that would allow stale pre-baseline rows to match after a blind send).
snapshot_recipient_baseline() {
  local bot="$1"
  local out mid rc
  out="$(mktemp)"
  set +e
  u read "$bot" --limit 1 >"$out" 2>&1
  rc=$?
  set +e
  if [ "$rc" -ne 0 ]; then
    printf '08-rich-all-types ERROR: baseline u read failed (exit %s); refusing to send\n' "$rc" >&2
    cat "$out" >&2 || true
    rm -f "$out"
    die "baseline recipient read failed (exit $rc) — abort before send"
  fi
  mid="$(head -1 "$out" | jq -r '.id // empty' 2>/dev/null || true)"
  rm -f "$out"
  if [ -z "$mid" ] || [ "$mid" = "null" ] || [ "$mid" = "0" ]; then
    # Successful empty history (or no id field): baseline 0 is intentional.
    printf '0\n'
    return 0
  fi
  if ! printf '%s' "$mid" | grep -Eq '^[1-9][0-9]*$'; then
    die "baseline recipient message id is not a positive integer: ${mid:-<empty>}"
  fi
  printf '%s\n' "$mid"
}

# row_is_canonical_match: stdin/file row is rich:true, not truncated, text == golden.
row_is_canonical_match() {
  local row="$1" golden="$2"
  local text_tmp rich trunc
  text_tmp="$(mktemp)"
  # Flags first (cheap).
  rich="$(jq -r '.rich // false' "$row" 2>/dev/null || printf 'false')"
  trunc="$(jq -r '.rich_truncated // false' "$row" 2>/dev/null || printf 'false')"
  if [ "$rich" != "true" ] || [ "$trunc" = "true" ]; then
    rm -f "$text_tmp"
    return 1
  fi
  jq -j '.text // empty' "$row" >"$text_tmp" 2>/dev/null || {
    rm -f "$text_tmp"
    return 1
  }
  if cmp -s "$text_tmp" "$golden"; then
    rm -f "$text_tmp"
    return 0
  fi
  rm -f "$text_tmp"
  return 1
}

# select_new_canonical_rows: from JSONL read output, keep rows with id > baseline
# that match canonical golden/flags. Writes matching rows (compact JSONL) to matches_out.
# Prints match count on stdout.
select_new_canonical_rows() {
  local read_out="$1" baseline="$2" golden="$3" matches_out="$4"
  local row_tmp n id
  : >"$matches_out"
  n=0
  row_tmp="$(mktemp)"
  # read emits one JSON object per line (newest first).
  while IFS= read -r line || [ -n "$line" ]; do
    [ -n "$line" ] || continue
    # Skip non-JSON noise lines.
    printf '%s\n' "$line" | jq -e . >/dev/null 2>&1 || continue
    printf '%s\n' "$line" >"$row_tmp"
    id="$(jq -r '.id // empty' "$row_tmp" 2>/dev/null || true)"
    # Only messages strictly newer than baseline (recipient-side id space).
    if ! printf '%s' "$id" | grep -Eq '^[1-9][0-9]*$'; then
      continue
    fi
    if [ "$id" -le "$baseline" ]; then
      continue
    fi
    if row_is_canonical_match "$row_tmp" "$golden"; then
      jq -c . "$row_tmp" >>"$matches_out"
      n=$((n + 1))
    fi
  done <"$read_out"
  rm -f "$row_tmp"
  printf '%s\n' "$n"
}

# fetch_new_canonical_row: bounded poll user read for messages after baseline;
# require exactly one new canonical golden match. Writes that row to row_out.
# Prints recipient message id on stdout.
fetch_new_canonical_row() {
  local bot="$1" baseline="$2" golden="$3" row_out="$4"
  local out matches n id _
  out="$(mktemp)"
  matches="$(mktemp)"
  : >"$row_out"

  for _ in $(seq 1 "$POLL_ATTEMPTS"); do
    # Prefer --after baseline when baseline > 0 (MinID semantics).
    if [ "$baseline" -gt 0 ] 2>/dev/null; then
      u read "$bot" --after "$baseline" --limit "$READ_LIMIT" >"$out" 2>&1 || true
    else
      u read "$bot" --limit "$READ_LIMIT" >"$out" 2>&1 || true
    fi
    n="$(select_new_canonical_rows "$out" "$baseline" "$golden" "$matches")"
    if [ "$n" -gt 1 ]; then
      printf '08-rich-all-types ERROR: %s new canonical golden matches after baseline=%s (want exactly 1)\n' \
        "$n" "$baseline" >&2
      cat "$matches" >&2 || true
      rm -f "$out" "$matches"
      die "ambiguous: multiple new canonical matches (refuse to pick)"
    fi
    if [ "$n" -eq 1 ]; then
      head -1 "$matches" >"$row_out"
      id="$(jq -r '.id' "$row_out")"
      require_positive_message_id "$id"
      rm -f "$out" "$matches"
      printf '%s\n' "$id"
      return 0
    fi
    sleep "$POLL_SLEEP_SEC"
  done

  rm -f "$out" "$matches"
  die "no new canonical golden match after baseline=$baseline (bounded poll; refuse pre-existing/forwarded rows)"
}

assert_golden_and_flags() {
  local row="$1" golden="$2" text_out="$3"
  local rich trunc
  # jq -j: raw, no trailing newline (golden has no trailing newline).
  jq -j '.text' "$row" >"$text_out" || die "failed to extract .text from message row"
  if ! cmp -s "$text_out" "$golden"; then
    printf '08-rich-all-types ERROR: golden mismatch (byte-exact .text)\n' >&2
    diff -u "$golden" "$text_out" >&2 || true
    die "golden byte compare failed"
  fi
  rich="$(jq -r '.rich // false' "$row")"
  [ "$rich" = "true" ] || die "expected rich:true, got rich=$rich"
  trunc="$(jq -r '.rich_truncated // false' "$row")"
  [ "$trunc" != "true" ] || die "rich_truncated:true is not allowed for full All Types gate"
}

# --- offline self-test (no Telegram) -----------------------------------------
selftest_main() {
  local st media_dir golden fixture stub_sender stub_tgc marker poll_marker
  local name rc out
  st="$(mktemp -d)"
  media_dir="$st/media"
  golden="$st/golden.md"
  fixture="$st/fixture.bin"
  stub_sender="$st/stub-sender"
  stub_tgc="$st/stub-tgc"
  marker="$st/sender-called"
  poll_marker="$st/poll-called"
  mkdir -p "$media_dir"

  # Minimal non-empty golden / fixture stand-ins (real paths checked in live path).
  printf '# All Types Demo\nselftest-body' >"$golden"
  printf 'fake-fixture' >"$fixture"

  # Stub sender: records invocation and emits bot-local message_id JSON on stdout.
  # RICH_E2E_SEND_MID overrides message_id (invalid-id + cross-profile cases).
  cat >"$stub_sender" <<'EOF'
#!/usr/bin/env bash
set -uo pipefail
MARKER_FILE="${RICH_E2E_SEND_MARKER:-}"
[ -n "$MARKER_FILE" ] && printf 'called\n' >>"$MARKER_FILE"
MID="${RICH_E2E_SEND_MID:-257}"
# Emit as JSON number only for strict positive decimals so jq does not
# rewrite shapes under test (e.g. 042 must remain "042", not 42).
if printf '%s' "$MID" | grep -Eq '^[1-9][0-9]*$'; then
  printf '{"message_id":%s,"chat_id":99,"blocks":37}\n' "$MID"
else
  jq -nc --arg m "$MID" '{message_id:$m,chat_id:99,blocks:37}'
fi
exit 0
EOF
  chmod +x "$stub_sender"

  # Stub tgc driven by RICH_E2E_STUB_MODE / feed files:
  #   - baseline: newest id before send
  #   - poll feeds: JSONL files listed in RICH_E2E_READ_FEEDS (one path per poll)
  # Records every read/context invocation in poll_marker.
  cat >"$stub_tgc" <<'EOF'
#!/usr/bin/env bash
set -uo pipefail
if [ "${1:-}" = "--profile" ]; then shift 2; fi
cmd="${1:-}"
shift || true
POLL_MARKER="${RICH_E2E_POLL_MARKER:-}"
[ -n "$POLL_MARKER" ] && printf '%s\n' "$cmd $*" >>"$POLL_MARKER"

case "$cmd" in
  read)
    # Optional hard failure (auth/network simulation) — gate must abort before send.
    if [ "${RICH_E2E_READ_FAIL:-}" = "1" ]; then
      printf '{"error":"not_authenticated","message":"simulated baseline read failure"}\n' >&2
      exit 1
    fi
    # Consume feed files in order (one per successful open).
    FEEDS="${RICH_E2E_READ_FEEDS:-}"
    STATE="${RICH_E2E_READ_STATE:-/tmp/rich-e2e-read-state}"
    idx=0
    if [ -f "$STATE" ]; then
      idx="$(cat "$STATE" 2>/dev/null || echo 0)"
    fi
    if [ -z "$FEEDS" ]; then
      # Default empty history (exit 0, no rows) → baseline 0.
      exit 0
    fi
    # FEEDS is space-separated paths.
    # shellcheck disable=SC2086
    set -- $FEEDS
    n=$#
    if [ "$idx" -ge "$n" ]; then
      # Repeat last feed if exhausted.
      idx=$((n - 1))
    fi
    i=0
    for f in "$@"; do
      if [ "$i" -eq "$idx" ]; then
        cat "$f"
        echo $((idx + 1)) >"$STATE"
        exit 0
      fi
      i=$((i + 1))
    done
    exit 0
    ;;
  context)
    # Bot/user context must not be the assertion path; fail closed if used for
    # cross-profile lookup of bot-local ids.
    printf '{"error":"bot_unsupported","message":"context not used by gate"}\n' >&2
    exit 1
    ;;
  *)
    printf '{"error":"stub_unsupported","cmd":"%s"}\n' "$cmd" >&2
    exit 1
    ;;
esac
EOF
  chmod +x "$stub_tgc"

  make_all_media() {
    local n
    for n in "${REQUIRED_MEDIA[@]}"; do
      printf 'x' >"$media_dir/$n"
    done
  }

  # Capture preflight in a subshell so die()/exit from preflight does not abort selftest.
  capture_preflight() {
    local outfile="$1" md="$2" g="$3" f="$4"
    set +e
    (
      preflight "$md" "$g" "$f"
    ) >"$outfile" 2>&1
    rc=$?
    set +e
    return "$rc"
  }

  # Run send → parse mid; invalid mid must fail before recipient poll.
  assert_bad_sender_mid_fails_before_poll() {
    local label="$1" bad_mid="$2"
    local send_out mid_err
    rm -rf "$media_dir"; mkdir -p "$media_dir"; make_all_media
    : >"$marker"
    : >"$poll_marker"
    export RICH_E2E_SEND_MID="$bad_mid"
    export RICH_E2E_POLL_MARKER="$poll_marker"
    TGC="$stub_tgc"
    send_out="$st/send-$label.out"
    mid_err="$st/mid-$label.err"
    set +e
    run_sender "1001" "$media_dir" "$fixture" "e2ebot" >"$send_out" 2>/dev/null
    (
      set -e
      mid="$(parse_sender_message_id "$send_out")"
      # Must not reach recipient poll helpers with a bad mid.
      fetch_new_canonical_row "@selftestbot" 0 "$golden" "$st/row-$label" || true
    ) >"$mid_err" 2>&1
    rc=$?
    set +e
    unset RICH_E2E_SEND_MID
    if [ "$rc" -eq 0 ]; then
      die "selftest: message_id=$bad_mid should fail validation before poll"
    fi
    if grep -q '^read ' "$poll_marker" 2>/dev/null; then
      die "selftest: recipient poll ran despite invalid message_id=$bad_mid"
    fi
    if ! grep -Eqi 'invalid message_id|could not parse message_id' "$mid_err"; then
      die "selftest: expected message_id validation error for '$bad_mid'; got: $(tr '\n' ' ' <"$mid_err")"
    fi
    printf '✓ selftest: invalid message_id fails before poll (%s)\n' "$label"
  }

  # Helpers to write feed JSONL rows.
  write_row() {
    # id text_file rich trunc -> stdout JSON
    local id="$1" textf="$2" rich="$3" trunc="$4"
    jq -nc --argjson id "$id" --rawfile t "$textf" --argjson rich "$rich" --argjson trunc "$trunc" \
      '{id:$id, text:$t, rich:$rich, rich_truncated:$trunc}'
  }

  export RICH_E2E_SEND_BIN="$stub_sender"
  export RICH_E2E_SEND_MARKER="$marker"
  export RICH_E2E_POLL_MARKER="$poll_marker"
  export RICH_E2E_READ_STATE="$st/read-state"
  TGC="$stub_tgc"
  export E2E_BOT_USERNAME="selftestbot"
  export E2E_USER_ID="1001"
  BOT_PROFILE="e2ebot"
  USER_PROFILE="e2euser"
  # Speed up selftest polls.
  POLL_SLEEP_SEC=0

  # Require new helpers exist (RED if still on old fetch_message_row path only).
  if ! declare -F snapshot_recipient_baseline >/dev/null 2>&1 \
    || ! declare -F fetch_new_canonical_row >/dev/null 2>&1 \
    || ! declare -F select_new_canonical_rows >/dev/null 2>&1; then
    die "selftest: recipient-baseline helpers missing (snapshot_recipient_baseline / fetch_new_canonical_row / select_new_canonical_rows)"
  fi

  # --- missing each media basename: fail before sender ---
  for name in "${REQUIRED_MEDIA[@]}"; do
    rm -rf "$media_dir"
    mkdir -p "$media_dir"
    make_all_media
    rm -f "$media_dir/$name"
    : >"$marker"
    out="$st/preflight.out"
    capture_preflight "$out" "$media_dir" "$golden" "$fixture"
    rc=$?
    if [ "$rc" -eq 0 ]; then
      die "selftest: expected non-zero when missing media '$name' (exit 0)"
    fi
    if [ -s "$marker" ]; then
      die "selftest: sender was invoked despite missing media '$name'"
    fi
    if ! grep -Fq -- "$name" "$out"; then
      die "selftest: missing-media error should mention '$name'; got: $(tr '\n' ' ' <"$out")"
    fi
    printf '✓ selftest: missing media fails before sender (%s)\n' "$name"
  done

  # --- missing golden ---
  rm -rf "$media_dir"; mkdir -p "$media_dir"; make_all_media
  : >"$marker"
  out="$st/preflight.out"
  capture_preflight "$out" "$media_dir" "$st/no-such-golden.md" "$fixture"
  rc=$?
  [ "$rc" -ne 0 ] || die "selftest: missing golden should fail"
  [ ! -s "$marker" ] || die "selftest: sender invoked on missing golden"
  printf '✓ selftest: missing golden fails before sender\n'

  # --- missing fixture ---
  rm -rf "$media_dir"; mkdir -p "$media_dir"; make_all_media
  : >"$marker"
  out="$st/preflight.out"
  capture_preflight "$out" "$media_dir" "$golden" "$st/no-such-fixture.bin"
  rc=$?
  [ "$rc" -ne 0 ] || die "selftest: missing fixture should fail"
  [ ! -s "$marker" ] || die "selftest: sender invoked on missing fixture"
  if ! grep -Eqi 'fixture missing' "$out"; then
    die "selftest: missing-fixture error should mention fixture; got: $(tr '\n' ' ' <"$out")"
  fi
  printf '✓ selftest: missing fixture fails before sender\n'

  # --- missing sender ---
  rm -rf "$media_dir"; mkdir -p "$media_dir"; make_all_media
  out="$st/preflight.out"
  RICH_E2E_SEND_BIN="/nonexistent/rich-e2e-send-$$"
  export RICH_E2E_SEND_BIN
  capture_preflight "$out" "$media_dir" "$golden" "$fixture"
  rc=$?
  [ "$rc" -ne 0 ] || die "selftest: missing sender should fail"
  export RICH_E2E_SEND_BIN="$stub_sender"
  printf '✓ selftest: missing sender fails at preflight\n'

  # --- invalid sender message_id (diagnostic parse still strict) ---
  if ! declare -F require_positive_message_id >/dev/null 2>&1 \
    || ! declare -F parse_sender_message_id >/dev/null 2>&1; then
    die "selftest: require_positive_message_id/parse_sender_message_id missing"
  fi
  assert_bad_sender_mid_fails_before_poll "zero" "0"
  assert_bad_sender_mid_fails_before_poll "malformed" "not-an-id"
  assert_bad_sender_mid_fails_before_poll "leading-zero" "042"
  assert_bad_sender_mid_fails_before_poll "negative" "-7"

  # --- C1: live_main control flow under actual no-set-e regime ---
  # Top-level script is set -uo pipefail (NO -e). Unguarded
  #   baseline="$(snapshot_recipient_baseline …)"
  # only fails the command-substitution subshell when die() runs; the caller
  # continues and would invoke the sender. This selftest mirrors live_main
  # (no set -e) so a missing `if !` guard is a hard RED.
  if ! declare -F require_baseline_id >/dev/null 2>&1; then
    die "selftest: require_baseline_id missing"
  fi
  rm -rf "$media_dir"; mkdir -p "$media_dir"; make_all_media
  : >"$marker"
  : >"$poll_marker"
  rm -f "$RICH_E2E_READ_STATE"
  unset RICH_E2E_READ_FEEDS
  export RICH_E2E_READ_FAIL=1
  export RICH_E2E_SEND_MID=257
  set +e
  (
    # Intentionally NO set -e — same regime as live_main / script top-level.
    set +e
    preflight "$media_dir" "$golden" "$fixture" || exit 1
    # live_main shape (must be guarded):
    if ! baseline="$(snapshot_recipient_baseline "@selftestbot")"; then
      die "baseline snapshot failed — abort before send"
    fi
    require_baseline_id "$baseline"
    # If unguarded assignment were used, control would reach here after die in
    # the subshell with empty baseline and send anyway.
    run_sender "1001" "$media_dir" "$fixture" "e2ebot" >/dev/null 2>&1
    exit 0
  ) >"$st/baseline-fail.out" 2>&1
  rc=$?
  set +e
  unset RICH_E2E_READ_FAIL
  if [ "$rc" -eq 0 ]; then
    die "selftest: no-set-e baseline failure should abort live_main flow (exit 0)"
  fi
  if [ -s "$marker" ]; then
    die "selftest: sender invoked despite baseline failure under no-set-e live_main flow"
  fi
  if ! grep -Eqi 'baseline snapshot failed|baseline recipient read failed|baseline u read failed|refusing to send' "$st/baseline-fail.out"; then
    die "selftest: baseline-fail diagnostic missing; got: $(tr '\n' ' ' <"$st/baseline-fail.out")"
  fi
  printf '✓ selftest: no-set-e live_main baseline failure aborts before send\n'

  # --- I1: unguarded recipient match must not mask no-match under no-set-e ---
  rm -rf "$media_dir"; mkdir -p "$media_dir"; make_all_media
  : >"$marker"
  : >"$poll_marker"
  rm -f "$RICH_E2E_READ_STATE"
  export RICH_E2E_SEND_MID=257
  write_row 1300 "$golden" true false >"$st/feed-base-i1.jsonl"
  write_row 1300 "$golden" true false >"$st/feed-stale-i1.jsonl"
  export RICH_E2E_READ_FEEDS="$st/feed-base-i1.jsonl $st/feed-stale-i1.jsonl $st/feed-stale-i1.jsonl $st/feed-stale-i1.jsonl $st/feed-stale-i1.jsonl $st/feed-stale-i1.jsonl $st/feed-stale-i1.jsonl"
  set +e
  (
    set +e
    preflight "$media_dir" "$golden" "$fixture" || exit 1
    if ! baseline="$(snapshot_recipient_baseline "@selftestbot")"; then
      die "baseline snapshot failed"
    fi
    require_baseline_id "$baseline"
    send_out="$(mktemp)"
    if ! run_sender "1001" "$media_dir" "$fixture" "e2ebot" >"$send_out" 2>/dev/null; then
      die "sender failed"
    fi
    if ! bot_mid="$(parse_sender_message_id "$send_out")"; then
      die "parse sender mid failed"
    fi
    row="$(mktemp)"
    # live_main shape for recipient match (must be guarded):
    if ! recipient_mid="$(fetch_new_canonical_row "@selftestbot" "$baseline" "$golden" "$row")"; then
      die "recipient canonical match failed after baseline=$baseline (no VERIFIED)"
    fi
    # Must not reach VERIFIED on zero matches.
    printf 'VERIFIED bogus message_id=%s\n' "${recipient_mid:-empty}"
    exit 0
  ) >"$st/i1-nomatch.out" 2>&1
  rc=$?
  set +e
  if [ "$rc" -eq 0 ]; then
    die "selftest: no-set-e zero-match should abort before VERIFIED"
  fi
  if grep -q '^VERIFIED ' "$st/i1-nomatch.out"; then
    die "selftest: VERIFIED printed despite no recipient match under no-set-e"
  fi
  if ! grep -Eqi 'recipient canonical match failed|no new canonical' "$st/i1-nomatch.out"; then
    die "selftest: I1 error missing; got: $(tr '\n' ' ' <"$st/i1-nomatch.out")"
  fi
  printf '✓ selftest: no-set-e recipient no-match aborts without VERIFIED\n'

  # --- unit: select_new_canonical_rows ignores baseline / non-canonical ---
  local unit_in unit_matches unit_n
  unit_in="$st/unit-in.jsonl"
  unit_matches="$st/unit-matches.jsonl"
  printf 'not golden' >"$st/other.txt"
  {
    write_row 1300 "$golden" true false   # old canonical (id <= baseline)
    write_row 1305 "$golden" true false   # new canonical
    write_row 1306 "$st/other.txt" true false
    write_row 1307 "$golden" true true    # truncated
  } >"$unit_in"
  unit_n="$(select_new_canonical_rows "$unit_in" 1300 "$golden" "$unit_matches")"
  [ "$unit_n" = "1" ] || die "selftest: select should keep only id>1300 exact match (got n=$unit_n)"
  [ "$(jq -r '.id' "$unit_matches")" = "1305" ] || die "selftest: select kept wrong id"
  printf '✓ selftest: select_new_canonical_rows ignores old/truncated/non-golden\n'

  # --- zero new matches fails (only stale pre-existing canonical present) ---
  rm -rf "$media_dir"; mkdir -p "$media_dir"; make_all_media
  : >"$marker"
  : >"$poll_marker"
  rm -f "$RICH_E2E_READ_STATE"
  export RICH_E2E_SEND_MID=257
  # baseline feed + poll feeds: only old id 1300 with golden body
  write_row 1300 "$golden" true false >"$st/feed-baseline.jsonl"
  write_row 1300 "$golden" true false >"$st/feed-stale-only.jsonl"
  export RICH_E2E_READ_FEEDS="$st/feed-baseline.jsonl $st/feed-stale-only.jsonl $st/feed-stale-only.jsonl $st/feed-stale-only.jsonl $st/feed-stale-only.jsonl $st/feed-stale-only.jsonl $st/feed-stale-only.jsonl"
  set +e
  (
    set -e
    preflight "$media_dir" "$golden" "$fixture"
    baseline="$(snapshot_recipient_baseline "@selftestbot")"
    [ "$baseline" = "1300" ] || die "expected baseline 1300 got $baseline"
    send_out="$(mktemp)"
    run_sender "1001" "$media_dir" "$fixture" "e2ebot" >"$send_out" 2>/dev/null
    bot_mid="$(parse_sender_message_id "$send_out")"
    [ "$bot_mid" = "257" ] || die "expected bot-local 257"
    row="$(mktemp)"
    # Must fail: only stale golden row, no id > baseline
    fetch_new_canonical_row "@selftestbot" "$baseline" "$golden" "$row"
  ) >"$st/zero.out" 2>&1
  rc=$?
  set +e
  [ "$rc" -ne 0 ] || die "selftest: zero new matches should fail"
  if ! grep -Eqi 'no new canonical|refuse pre-existing' "$st/zero.out"; then
    die "selftest: zero-match error missing; got: $(tr '\n' ' ' <"$st/zero.out")"
  fi
  printf '✓ selftest: zero new matches fails (stale pre-existing ignored)\n'

  # --- multiple new exact matches fails ---
  rm -f "$RICH_E2E_READ_STATE"
  : >"$poll_marker"
  {
    write_row 1300 "$golden" true false
  } >"$st/feed-baseline2.jsonl"
  {
    write_row 1309 "$golden" true false
    write_row 1310 "$golden" true false
  } >"$st/feed-multi.jsonl"
  export RICH_E2E_READ_FEEDS="$st/feed-baseline2.jsonl $st/feed-multi.jsonl"
  set +e
  (
    set -e
    baseline="$(snapshot_recipient_baseline "@selftestbot")"
    [ "$baseline" = "1300" ] || die "baseline want 1300 got $baseline"
    row="$(mktemp)"
    fetch_new_canonical_row "@selftestbot" "$baseline" "$golden" "$row"
  ) >"$st/multi.out" 2>&1
  rc=$?
  set +e
  [ "$rc" -ne 0 ] || die "selftest: multiple new matches should fail"
  if ! grep -Eqi 'multiple new canonical|ambiguous' "$st/multi.out"; then
    die "selftest: multi-match error missing; got: $(tr '\n' ' ' <"$st/multi.out")"
  fi
  printf '✓ selftest: multiple new exact matches fails closed\n'

  # --- cross-profile mismatch happy path: bot-local 257, recipient 1309 ---
  # Stale 1300 golden must be ignored; only new 1309 counts; VERIFIED uses 1309.
  rm -rf "$media_dir"; mkdir -p "$media_dir"; make_all_media
  : >"$marker"
  : >"$poll_marker"
  rm -f "$RICH_E2E_READ_STATE"
  export RICH_E2E_SEND_MID=257
  {
    write_row 1300 "$golden" true false
  } >"$st/feed-base-x.jsonl"
  {
    # Include stale golden + the new one; selection must pick only 1309.
    write_row 1309 "$golden" true false
    write_row 1300 "$golden" true false
  } >"$st/feed-new-x.jsonl"
  export RICH_E2E_READ_FEEDS="$st/feed-base-x.jsonl $st/feed-new-x.jsonl"

  preflight "$media_dir" "$golden" "$fixture"
  local baseline bot_mid recipient_mid send_out row text_out
  baseline="$(snapshot_recipient_baseline "@selftestbot")"
  [ "$baseline" = "1300" ] || die "selftest: cross-profile baseline want 1300 got $baseline"

  send_out="$(mktemp)"
  run_sender "1001" "$media_dir" "$fixture" "e2ebot" >"$send_out" 2>/dev/null \
    || die "selftest: stub sender failed"
  [ -s "$marker" ] || die "selftest: stub sender was not invoked"
  bot_mid="$(parse_sender_message_id "$send_out")"
  [ "$bot_mid" = "257" ] || die "selftest: bot-local mid want 257 got $bot_mid"

  row="$(mktemp)"; text_out="$(mktemp)"
  recipient_mid="$(fetch_new_canonical_row "@selftestbot" "$baseline" "$golden" "$row")"
  [ "$recipient_mid" = "1309" ] || die "selftest: recipient mid want 1309 got $recipient_mid"
  [ "$recipient_mid" != "$bot_mid" ] || die "selftest: recipient id must differ from bot-local in mismatch case"
  # Must not have used context for assertion path.
  if grep -q '^context ' "$poll_marker" 2>/dev/null; then
    die "selftest: gate used context (bot_unsupported path) for assertion"
  fi
  assert_golden_and_flags "$row" "$golden" "$text_out"

  printf 'VERIFIED All Types Demo chat=%s message_id=%s blocks=%s sender_message_id=%s\n' \
    "1001" "$recipient_mid" "$DEFAULT_BLOCKS" "$bot_mid"
  printf '✓ selftest: cross-profile mismatch (sender 257, recipient 1309) + stale ignored\n'

  # --- happy path: one new exact match, sender id unused for lookup ---
  rm -rf "$media_dir"; mkdir -p "$media_dir"; make_all_media
  : >"$marker"
  : >"$poll_marker"
  rm -f "$RICH_E2E_READ_STATE"
  export RICH_E2E_SEND_MID=9999
  printf '' >"$st/feed-empty.jsonl"
  {
    write_row 42 "$golden" true false
  } >"$st/feed-one.jsonl"
  # baseline empty (id 0), then one new match
  export RICH_E2E_READ_FEEDS="$st/feed-empty.jsonl $st/feed-one.jsonl"

  preflight "$media_dir" "$golden" "$fixture"
  baseline="$(snapshot_recipient_baseline "@selftestbot")"
  [ "$baseline" = "0" ] || die "selftest: empty baseline want 0 got $baseline"
  send_out="$(mktemp)"
  run_sender "1001" "$media_dir" "$fixture" "e2ebot" >"$send_out" 2>/dev/null
  bot_mid="$(parse_sender_message_id "$send_out")"
  [ "$bot_mid" = "9999" ] || die "selftest: bot mid want 9999 got $bot_mid"
  row="$(mktemp)"; text_out="$(mktemp)"
  recipient_mid="$(fetch_new_canonical_row "@selftestbot" "$baseline" "$golden" "$row")"
  [ "$recipient_mid" = "42" ] || die "selftest: recipient want 42 got $recipient_mid"
  assert_golden_and_flags "$row" "$golden" "$text_out"
  printf 'VERIFIED All Types Demo chat=%s message_id=%s blocks=%s sender_message_id=%s\n' \
    "1001" "$recipient_mid" "$DEFAULT_BLOCKS" "$bot_mid"
  printf '✓ selftest: happy path one new exact match returns recipient id\n'

  rm -rf "$st"
  printf '\nPASSED: selftest complete\n'
  exit 0
}

# --- live gate ---------------------------------------------------------------
live_main() {
  require_setup

  local media_dir golden fixture target bot profile
  media_dir="${E2E_RICH_MEDIA_DIR:-$DEFAULT_MEDIA_DIR}"
  golden="${E2E_RICH_GOLDEN:-$DEFAULT_GOLDEN}"
  fixture="${E2E_RICH_FIXTURE:-$DEFAULT_FIXTURE}"
  profile="${E2E_BOT_PROFILE:-$BOT_PROFILE}"
  # Bot sends to the e2e user (same direction as 07-rich smoke).
  target="${E2E_USER_ID}"
  bot="@${E2E_BOT_USERNAME}"

  command -v jq >/dev/null || die "jq not found"
  command -v cmp >/dev/null || die "cmp not found"
  command -v "$TGC" >/dev/null 2>&1 || [ -x "$TGC" ] || die "tgc binary not found ($TGC)"

  printf '08-rich-all-types: preflight (media=%s)\n' "$media_dir" >&2
  preflight "$media_dir" "$golden" "$fixture"

  # Snapshot recipient-side baseline BEFORE send so pre-existing/forwarded
  # canonical rows cannot satisfy the gate.
  # CRITICAL: top-level is set -uo pipefail (no -e). Command substitution that
  # calls die() only exits the subshell; without `if !` the live path continues
  # and would send with an empty baseline.
  printf '08-rich-all-types: snapshot recipient baseline in %s…\n' "$bot" >&2
  local baseline
  if ! baseline="$(snapshot_recipient_baseline "$bot")"; then
    die "baseline snapshot failed — abort before send"
  fi
  require_baseline_id "$baseline"
  printf '08-rich-all-types: recipient baseline message_id=%s\n' "$baseline" >&2

  local send_out bot_mid blocks recipient_mid
  send_out="$(mktemp)"
  printf '08-rich-all-types: sending All Types fixture via rich-e2e-send…\n' >&2
  if ! run_sender "$target" "$media_dir" "$fixture" "$profile" >"$send_out" 2>&1; then
    cat "$send_out" >&2
    die "rich-e2e-send failed"
  fi
  # Bot-local id is diagnostic only (cross-profile ids differ).
  if ! bot_mid="$(parse_sender_message_id "$send_out")"; then
    cat "$send_out" >&2
    die "could not parse positive bot-local message_id from sender output"
  fi
  blocks="$(grep -E '^\{' "$send_out" | tail -1 | jq -r '.blocks // empty')"
  blocks="${blocks:-$DEFAULT_BLOCKS}"
  printf '08-rich-all-types: sender bot-local message_id=%s (diagnostic; not used for lookup)\n' \
    "$bot_mid" >&2

  local row text_out
  row="$(mktemp)"; text_out="$(mktemp)"
  printf '08-rich-all-types: polling user read for new canonical after baseline=%s…\n' \
    "$baseline" >&2
  # Same no-set-e hazard: unguarded assignment would mask no-match die() and
  # fall through into golden assert / VERIFIED with empty recipient_mid.
  if ! recipient_mid="$(fetch_new_canonical_row "$bot" "$baseline" "$golden" "$row")"; then
    die "recipient canonical match failed after baseline=$baseline (no VERIFIED)"
  fi
  require_positive_message_id "$recipient_mid"
  assert_golden_and_flags "$row" "$golden" "$text_out"

  printf 'VERIFIED All Types Demo chat=%s message_id=%s blocks=%s sender_message_id=%s\n' \
    "$target" "$recipient_mid" "$blocks" "$bot_mid"

  if [ "${E2E_RICH_CLEANUP:-}" = "1" ]; then
    printf '08-rich-all-types: E2E_RICH_CLEANUP=1 — deleting recipient message_id=%s\n' \
      "$recipient_mid" >&2
    cleanup_msg "$USER_PROFILE" "$bot" "$recipient_mid"
  else
    printf '08-rich-all-types: preserving recipient message_id=%s (set E2E_RICH_CLEANUP=1 to delete)\n' \
      "$recipient_mid" >&2
  fi

  rm -f "$send_out" "$row" "$text_out"
  exit 0
}

# --- entry -------------------------------------------------------------------
if is_selftest "${1:-}"; then
  selftest_main
fi
live_main
