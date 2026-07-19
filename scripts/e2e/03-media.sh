#!/usr/bin/env bash
# scripts/e2e/03-media.sh — media round-trips: document sha256 integrity, photo
# (re-encoded) mime/non-empty, and album (media-group) delivered as N messages
# sharing ONE grouped_id. Bot receives via await (its only inbound path).
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/e2e/lib.sh disable=SC1091
. "$HERE/lib.sh"
# shellcheck disable=SC1091
[ -f "$HERE/.env.generated" ] && . "$HERE/.env.generated"
require_setup
BOT="@$E2E_BOT_USERNAME"; USR="$E2E_USER_ID"
T=$(mktemp); TMP=$(mktemp -d)

# known document blob
head -c 2048 /dev/urandom > "$TMP/doc.bin"
SENT_SHA=$(sha256sum "$TMP/doc.bin" | awk '{print $1}')

# valid 2x2 PNG (python3 struct/zlib — Telegram re-encodes to JPEG on download).
python3 - "$TMP/px.png" <<'PY'
import struct, zlib, sys
w = h = 2
raw = b''.join(b'\x00' + b'\xff\x00\x00' * w for _ in range(h))
def chunk(t, d):
    c = t + d
    return struct.pack('>I', len(d)) + c + struct.pack('>I', zlib.crc32(c) & 0xffffffff)
png  = b'\x89PNG\r\n\x1a\n'
png += chunk(b'IHDR', struct.pack('>IIBBBBB', w, h, 8, 2, 0, 0, 0))
png += chunk(b'IDAT', zlib.compress(raw))
png += chunk(b'IEND', b'')
open(sys.argv[1], 'wb').write(png)
PY

# --- document round-trip user -> bot, download + sha ---
# await_bg sets the global AWAIT_PID (NEVER call via $(...) — the subshell would
# leave the outfile empty).
BOUT=$(mktemp); await_bg "$BOT_PROFILE" "$USR" "--timeout 20 --debounce 1" "$BOUT"; BPID=$AWAIT_PID; sleep 3
u send "$BOT" --file "$TMP/doc.bin" --caption "$(nonce 03doc)" >/dev/null 2>&1
wait "$BPID" 2>/dev/null
DMID=$(jqf "$BOUT" 'select(.media!=null) | .id')
if [ -n "$DMID" ]; then
  b download "$USR" "$DMID" -o "$TMP/got.bin" > "$T" 2>&1
  GOT="$(jqf "$T" '.path')"; [ -n "$GOT" ] && GOT_SHA=$(sha256sum "$GOT" | awk '{print $1}') || GOT_SHA=""
  assert_eq "03: document sha256 integrity" "$SENT_SHA" "$GOT_SHA"
else
  skip "03: document download" "bot did not receive the document"
fi

# --- photo round-trip user -> bot ---
BOUT2=$(mktemp); await_bg "$BOT_PROFILE" "$USR" "--timeout 20 --debounce 1" "$BOUT2"; BPID2=$AWAIT_PID; sleep 3
u send "$BOT" --file "$TMP/px.png" --caption "$(nonce 03png)" >/dev/null 2>&1
wait "$BPID2" 2>/dev/null
PMID=$(jqf "$BOUT2" 'select(.media!=null) | .id')
if [ -n "$PMID" ]; then
  b download "$USR" "$PMID" -o "$TMP/got.img" > "$T" 2>&1
  GP="$(jqf "$T" '.path')"; MIME="$(jqf "$T" '.mime')"
  if [ -n "$GP" ] && [ -s "$GP" ]; then pass "03: photo non-empty"; else fail "03: photo non-empty" "empty/missing"; fi
  case "$MIME" in image/*) pass "03: photo mime image/*";; *) fail "03: photo mime image/*" "got [$MIME]";; esac
else
  skip "03: photo download" "bot did not receive the photo"
fi

# --- album (media-group) user -> bot: >=2 members, all one grouped_id ---
BOUT3=$(mktemp); await_bg "$BOT_PROFILE" "$USR" "--timeout 25 --debounce 4" "$BOUT3"; BPID3=$AWAIT_PID; sleep 3
head -c 512 /dev/urandom > "$TMP/a.bin"; head -c 512 /dev/urandom > "$TMP/b.bin"; head -c 512 /dev/urandom > "$TMP/c.bin"
u send "$BOT" --file "$TMP/a.bin" --file "$TMP/b.bin" --file "$TMP/c.bin" --caption "$(nonce 03album)" >/dev/null 2>&1
wait "$BPID3" 2>/dev/null
AN=$(grep -c '"media"' "$BOUT3" || true)
if [ "${AN:-0}" -ge 2 ]; then pass "03: album delivered ($AN media)"; else fail "03: album delivered" "want >=2 media, got ${AN:-0}"; fi
# all album members must share exactly ONE grouped_id (jq: group the grouped_id-bearing lines).
# NOTE: do NOT name the group-count var GROUPS — that is a special/read-only bash
# variable (process-group IDs) and assignment to it silently no-ops.
NGRP=$(jq -s 'map(select(.grouped_id)) | group_by(.grouped_id) | length' "$BOUT3" 2>/dev/null || echo "")
GSIZE=$(jq -s 'map(select(.grouped_id)) | length' "$BOUT3" 2>/dev/null || echo 0)
if [ "$NGRP" = "1" ] && [ "${GSIZE:-0}" -ge 2 ]; then
  pass "03: album shares one grouped_id ($GSIZE members)"
else
  fail "03: album grouped_id" "want 1 group of >=2, got groups=$NGRP size=$GSIZE"
fi

summary
