# Design: bidirectional live e2e suite (bot-side + user-side, all via tgc)

**Date:** 2026-07-19
**Bead:** brainstorming session (reverse e2e)
**Status:** Approved (pending spec review)

## Problem

The await feature was live-verified only from the user side (user runs `tgc`,
bot driven via raw Bot API `curl`). We have no coverage of tgc from the **bot
side**, and no repeatable e2e harness. We want live end-to-end scripts that
exercise tgc **from both a user profile and a bot profile**, driving both sides
through `tgc` itself (no Bot API curl), covering every user command and every
bot-permitted command, in both directions.

## Goals

- User side: exercise **every** tgc command.
- Bot side: exercise every bot-permitted command; for bot-forbidden commands,
  assert tgc returns a clean structured error (not a crash/hang).
- Both directions (user→bot and bot→user) for the conversational commands.
- Committed, re-runnable script suite with per-scenario files + a runner.
- Secrets only from env; nothing sensitive in the repo.

## Non-goals (YAGNI)

- Parallel scenario execution, JUnit-XML output, CI wiring (live + secrets →
  manual run for now; a CI hook is a separate future bead).
- Automating interactive user login (setup reuses an existing logged-in user
  profile; bot login is non-interactive via `--bot-token`).

## 1. Coverage matrix

Profiles: `e2euser` (user account) and `e2ebot` (bot). Partner for user commands
= the bot; partner for bot commands = the user.

### User side — ALL tgc commands

| Command | Assertion |
|---|---|
| `auth list` | profiles present |
| `config path` | source/profile fields |
| `version` / `self check` | version JSON; `self check` asserts **softly** — accept either a normal update JSON OR `{"error":"rate_limited"}` as a valid "command ran" outcome (GitHub's unauthenticated 60/hr cap is a known flake source, cf. tgc-1zm/a5w); the assertion checks the command executed and emitted structured JSON, not that GitHub answered |
| `info @bot` | card, `bot: true` |
| `send` text/markdown | `message_id` returned |
| `send --reply` | `reply_to` on the stored msg |
| `send --file` photo/doc | `media` present |
| `send --file` album (media-group) | multiple media msgs |
| `send --await-reply` | send line then reply line, one connection |
| `await` (burst) | 3 msgs coalesced, oldest→newest |
| `await` timeout | `{"status":"timeout",...}` exit 0 |
| `await --from` | sender filter |
| `read` (limit/since/from/search) | history shape |
| `context <id>` | message + surrounding |
| `edit` | `edited: true` |
| `delete` / `--for-me` | delete result |
| `forward` | forwarded to another chat (e.g. Saved Messages) |
| `download` | media bytes/file received from bot |
| `chats` (--type/--fresh) | dialogs list |
| `search` (chats + `--messages`) | results |
| `members <group>` | members list (only if a test group exists; else skipped) |

### Bot side — bot-permitted commands

| Command | Expectation |
|---|---|
| `send` / `--reply` / `--file` / album | works |
| `await` (LIVE-ONLY) | catches the user; must start the await BEFORE the user sends (bots have no read-pointer/backfill) |
| `send --await-reply` | works (live-only) |
| `read` (its conversation with the user) | works |
| `edit` / `delete` / `forward` / `download` | works |
| `info <user_id>` | works (the user has messaged the bot) |
| `context <id>` | works |

### Deliberate skips (documented)

**A — Telegram forbids for bots (not a tgc bug); assert a clean structured error:**

Assertions use the ACTUAL codes, verified empirically against the live bot (not
"any error"):

| Bot-side command | Why | Empirically-confirmed result → assertion |
|---|---|---|
| `chats` | `messages.getDialogs` → `BOT_METHOD_INVALID`, mapped by `client.WrapErr` | `{"error":"bot_unsupported"}`, exit 1 → assert `error==bot_unsupported` && exit 1 |
| `search` (dialogs mode, no `--messages`) | relies on dialogs | same `bot_unsupported`, exit 1 → assert `error==bot_unsupported` && exit 1 |
| `members <group>` | bot not admin of the probe group | `{"error":"not_found",...unknown to this bot}`, exit 1 → assert `error==not_found` && exit 1 (or skip if a bot-admin group is provided, then assert a members list) |

If a future run returns a bare `internal` instead of these mapped codes, that
signals a `WrapErr` coverage gap → file a bead, do not loosen the assertion.

**B — setup / destructive / non-Telegram (not conversational scenarios):**

| Command | Why skipped from scenarios |
|---|---|
| `auth login/import/export/logout` | setup/teardown, not a chat scenario; `login --bot-token` is called once in setup |
| `init` | creates `./.tgc` — setup |
| `self update` | downloads/replaces the binary — destructive; only `self check` (read-only) is run |
| `completion` / `help` | generators, not Telegram |

`download` is exercised both directions: each side sends a file, the other
downloads it. Integrity assertion is two-tier (Telegram re-compresses photos but
not documents):
- **Document** (sent as a file, non-photo): `sha256(sent) == sha256(downloaded)`
  — strict integrity. Use a known small random blob generated in setup
  (`head -c 2048 /dev/urandom > doc.bin`).
- **Photo** (photo mode): Telegram re-encodes, so assert only that the download
  is non-empty, `mime == image/*`, and size > 0 — not byte equality. Use a small
  known PNG.

`await` for bots is **not** a skip — it is run, but the script must start the
bot-side await before the user sends (live-only). This is a test requirement.

## 2. File structure

```
scripts/e2e/
├── lib.sh          # TGC bin, profiles, u()/b() wrappers, assert helpers, JSONL/jq, await_bg, cleanup, counters
├── setup.sh        # jq/bin/.env checks; bot login if needed; mutual-reachability check; export ids/usernames
├── 01-send-read.sh # send/read/context/info/edit/delete/forward — both directions
├── 02-await.sh     # await burst/timeout/--from, send --await-reply, media-group; user + bot(live-only)
├── 03-media.sh     # send --file photo/doc/album + download — both directions
├── 04-dialogs.sh   # chats/search/members/read-filters — user side
├── 05-bot-limits.sh# bot chats/members/search → assert bot_unsupported/structured error
├── 06-meta.sh      # auth list, config path, version, self check, --pretty render
└── run-all.sh      # source lib, run setup, run 01..06, summarize PASS/FAIL, exit 0/1
```

lib.sh core:

```bash
TGC="${TGC_BIN:-tgc}"
USER_PROFILE="${E2E_USER_PROFILE:-e2euser}"
BOT_PROFILE="${E2E_BOT_PROFILE:-e2ebot}"

u(){ "$TGC" --profile "$USER_PROFILE" "$@"; }   # user command
b(){ "$TGC" --profile "$BOT_PROFILE"  "$@"; }   # bot command

assert_json <jsonl-line> <jq-filter> <expected>
assert_error <jsonl-line> <code>
assert_nonempty <value>
assert_exit_code <expected> <actual>
pass "<name>"   / fail "<name>: <detail>"      # counters + colored output
await_bg <profile> <chat> "<flags>" <outfile>  # start await in background, echo PID
```

Properties:
- Idempotent: each script runs standalone (`bash scripts/e2e/02-await.sh`) or via `run-all.sh`.
- Secrets only from env (`TGC_BOT_TOKEN`; the user session already lives in its profile). Nothing in the repo.
- Bot-await always via `await_bg` BEFORE the user sends (live-only).
- Each scenario best-effort deletes its own test messages at the end.
- `run-all.sh` exits non-zero on any FAIL (CI-ready later).
- Requires `jq` (checked in setup.sh).

## 3. Profile setup & preconditions

`setup.sh`:
1. Check `jq`, the `TGC` binary, and `.env` (`TGC_API_ID/HASH/BOT_TOKEN`).
2. User profile `e2euser`: setup **only checks** for an existing session and never
   attempts an interactive user login (that would hang a non-interactive run). If
   there is no session, it fails fast with instructions to log in manually once,
   or to reuse an already-logged-in profile via `E2E_USER_PROFILE`.
3. Bot profile `e2ebot`: if not logged in,
   `tgc --profile e2ebot auth login --bot-token "$TGC_BOT_TOKEN"` (non-interactive,
   idempotent).
4. Mutual reachability: `u info @<bot>` and `b info <user_id>` must both resolve.
   If the bot can't see the user, instruct: the user must press Start / send the
   bot a message once.
5. Resolve and export `E2E_BOT_USERNAME`, `E2E_BOT_ID`, `E2E_USER_ID` so scenarios
   address `@bot` (user side) and numeric `user_id` (bot side — bots can't resolve
   usernames via dialogs).

Preconditions (documented in an e2e README section):
- The user pressed Start / messaged the bot once (bots can't initiate).
- `.env` populated.
- Accounts are test accounts (messages are really sent).

Isolation:
- `e2euser`/`e2ebot` are **separate** from `default` to avoid touching a real
  session. The suite sends real messages and runs `delete`/mark-read, so pointing
  it at a real `default` account is a footgun.
- **Default-collision guard:** the default user profile is the dedicated
  `e2euser`. Reusing `default` requires BOTH an explicit
  `E2E_USER_PROFILE=default` AND `E2E_ALLOW_DEFAULT=1`; otherwise `setup.sh` stops
  with a warning. Before any scenario runs, `setup.sh` prints which account
  (username/id) will act as "user" and which as "bot", so a wrong target is caught
  before messages are sent.
- Test messages are prefixed `[e2e] <scenario> <nonce>` and cleaned up where
  possible.

Key constraint (from the await design): one `await` per profile at a time. User
and bot are different profiles, so their awaits are safe in parallel; within one
side, awaits are sequential.

Concurrency guard (shared workspace): two `run-all.sh` invocations against the
same `e2ebot`/`e2euser` would run two awaits on one profile → they'd steal each
other's updates (flaky) and risk FLOOD_WAIT. `run-all.sh` takes an exclusive
`flock` on `/tmp/tgc-e2e-<botprofile>.lock`; if held, it exits with a clear "an
e2e run is already active on this profile — wait or use another profile". No
queue/orchestration (YAGNI) — just refuse when busy.

## 4. Assertions, output, robustness

Assertion model (lib.sh):
- Each scenario is a set of steps; a step runs a `tgc` command, parses JSONL via
  `jq`, compares to an expectation.
- Helpers: `assert_eq`, `assert_json <line> <jq-filter> <expected>`,
  `assert_error <line> <code>`, `assert_nonempty`, `assert_exit_code`.
- Each assertion → `pass`/`fail` with counters. A FAIL does not abort the file
  (collect all), but marks the run failed.

Output & run semantics (two-tier):
- **setup — fail-fast:** if `setup.sh` fails (missing profile/token/reachability),
  `run-all.sh` stops immediately with **exit 2** (a setup error is not a test
  failure) and runs no scenarios.
- **scenarios — collect-all:** each scenario file is self-contained (nonce-keyed);
  run them all, aggregate pass/fail. `run-all.sh` exits **1** if any assertion
  FAILed, **0** if all green.
- **cascade within a file:** if an early step in a file fails, later steps in that
  same file that depend on it are marked `SKIP`, not `FAIL` (no false cascade of
  failures from one root cause).
- Human-readable: `✓ 02-await: burst coalesced (3 msgs)` / `✗ 05-bot-limits: chats returned no error` / `⊘ 01-send-read: edit (skipped: send failed)`.
- `run-all.sh` end: `PASSED: N, FAILED: M, SKIPPED: K` + failed list.
- `E2E_VERBOSE=1` prints raw command JSONL (auth/login excluded).

Robustness (flakiness is the main enemy of live e2e):
- Awaits always carry a short timeout (15–20s) so nothing hangs.
- Synchronization (bot-await is live-only — the main flakiness risk): where the
  user sends and the bot waits, the bot await starts in the background
  (`await_bg`) with a base `sleep` for the dispatcher to settle, then the send;
  capture the PID and `wait`. A blind `sleep` alone is insufficient — so:
  - Every bot-await receive step is wrapped in a **mandatory** retry (up to 2
    re-sends with a fresh nonce if the bot didn't catch within its timeout), not
    an optional one.
  - `setup.sh` performs a **warm-up** throwaway round-trip first, so the first
    (slowest) bot connect/dispatcher spin-up doesn't happen inside a measured
    scenario.
- Unique markers: each test message carries a nonce (`[e2e] <scenario> <RANDOM>`)
  so the assertion matches its own text, not stale/other messages.
- Retry wrapper for "arrived / not arrived": one retry of the step on a live
  race, then fail.
- Rate-limit: small `sleep` between heavy steps to avoid FLOOD_WAIT on the test
  account.
- Correctness rests on **nonce matching, not cleanup**: every assertion matches
  its own unique nonce, so leftover messages from earlier scenarios never break a
  check (each step only looks for its own text). No assertion depends on whether
  cleanup deleted anything.
- Cleanup is best-effort cosmetic only: end of each scenario, `delete` (revoke —
  NOT `--for-me`) its own messages, so nothing dangles on the partner side for
  another scenario's await to catch. A cleanup failure never affects pass/fail.
- Scenarios that specifically test `delete` use a dedicated message and assert the
  delete result explicitly — never conflated with cleanup.

Security:
- No secret is written to the repo; `TGC_BOT_TOKEN` is read from the environment
  (or `.env`, which is gitignored) at runtime only.
- Scripts do not echo the token. `setup.sh` wraps the `auth login --bot-token`
  call with a local `set +x` / restore, so even under bash debug (`set -x`) the
  token never lands in a trace.
- `E2E_VERBOSE=1` dumps JSONL for non-auth commands only — the login step is
  explicitly excluded from the verbose dump. (`auth login` output is
  `{status,profile,type,user_id}` with no token, but it is excluded anyway.)
- The token is never written to any file/outfile; `await_bg` outfiles hold await
  output only.
- Test accounts only; scripts never touch the `default` profile unless explicitly
  pointed at it via env. The e2e README section warns that scripts send real
  messages and require a throwaway test account.

## 5. Testing (of the harness itself)

- `bash -n` (syntax) + `shellcheck` clean on every script.
- `setup.sh` is safe to re-run (idempotent bot login, non-destructive checks).
- A dry smoke: `run-all.sh` against the real test accounts should end with a
  PASS/FAIL summary and a correct exit code.
- Each scenario file is independently runnable and self-cleans.

## Stress Test Results: bidirectional e2e design

### Resolved Decisions
- **Bot-await flakiness:** await_bg + base sleep + MANDATORY per-step retry (2 re-sends, fresh nonce) + setup warm-up round-trip. Blind sleep alone insufficient (bot live-only, no backfill).
- **Cleanup vs correctness:** correctness rests on nonce-matching, never on delete. Cleanup = best-effort revoke-delete (not --for-me), never affects pass/fail.
- **Secrets:** set +x guard around auth login; verbose excludes login; token never to files; README warns real messages / test account only.
- **Profile collision:** default = dedicated e2euser; reusing `default` needs E2E_USER_PROFILE=default AND E2E_ALLOW_DEFAULT=1; setup prints the chosen user/bot accounts before sending.
- **Bot-forbidden assertions:** empirically confirmed — chats/search → error=bot_unsupported exit1; members(non-admin) → error=not_found exit1. Assert exact codes; a bare `internal` would signal a WrapErr gap → bead, not a loosened assertion.
- **Download integrity:** document → sha256(sent)==sha256(downloaded); photo → non-empty + mime image/* + size>0 (Telegram re-compresses).
- **run-all semantics:** setup fail-fast exit 2; scenarios collect-all exit 1-on-any-FAIL; in-file cascade → SKIP not FAIL.
- **Concurrency:** flock on /tmp/tgc-e2e-<botprofile>.lock; refuse when busy (no queue).

### Changes Made
- All 8 branches above edited into §§1–4.
- Reflexion: setup only checks user session (never interactive login); `self check` asserted softly (accept rate_limited as a valid "ran" outcome).

### Deferred / Parking Lot
- CI wiring (live + secrets → manual for now); separate future bead.
- WrapErr coverage bead only if a bot command returns bare `internal`.

### Confidence Assessment
- Overall: High. Empirically grounded (bot login + forbidden-command codes verified live).
- Areas of concern: live-network flakiness is inherent; mitigated by retry+warm-up+nonce+timeouts. Not eliminable for a real-Telegram suite.
