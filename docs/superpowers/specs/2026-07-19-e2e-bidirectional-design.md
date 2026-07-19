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
| `version` / `self check` | version JSON / update JSON (read-only) |
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

| Bot-side command | Why | What we do |
|---|---|---|
| `chats` | `messages.getDialogs` → `BOT_METHOD_INVALID` | run it, assert tgc returns a structured error (e.g. `bot_unsupported`) with exit 1, no crash/hang |
| `search` (dialogs mode, no `--messages`) | relies on dialogs | assert error/empty |
| `members <group>` | needs a group where the bot is admin | skip unless such a group is provided |

**B — setup / destructive / non-Telegram (not conversational scenarios):**

| Command | Why skipped from scenarios |
|---|---|
| `auth login/import/export/logout` | setup/teardown, not a chat scenario; `login --bot-token` is called once in setup |
| `init` | creates `./.tgc` — setup |
| `self update` | downloads/replaces the binary — destructive; only `self check` (read-only) is run |
| `completion` / `help` | generators, not Telegram |

`download` is exercised both directions: each side sends a file, the other
downloads it.

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
2. User profile `e2euser`: if no session, error with instructions (user login is
   interactive — not automated; an existing logged-in profile can be reused via
   `E2E_USER_PROFILE`, e.g. `default`).
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
  session (a logged-in profile may be reused as `E2E_USER_PROFILE=default`
  deliberately).
- Test messages are prefixed `[e2e] <scenario> <nonce>` and cleaned up where
  possible.

Key constraint (from the await design): one `await` per profile at a time. User
and bot are different profiles, so their awaits are safe in parallel; within one
side, awaits are sequential.

## 4. Assertions, output, robustness

Assertion model (lib.sh):
- Each scenario is a set of steps; a step runs a `tgc` command, parses JSONL via
  `jq`, compares to an expectation.
- Helpers: `assert_eq`, `assert_json <line> <jq-filter> <expected>`,
  `assert_error <line> <code>`, `assert_nonempty`, `assert_exit_code`.
- Each assertion → `pass`/`fail` with counters. A FAIL does not abort the file
  (collect all), but marks the run failed.

Output:
- Human-readable: `✓ 02-await: burst coalesced (3 msgs)` / `✗ 05-bot-limits: chats returned no error`.
- `run-all.sh` end: `PASSED: N, FAILED: M` + failed list, exit 0/1.
- `E2E_VERBOSE=1` prints raw command JSONL.

Robustness (flakiness is the main enemy of live e2e):
- Awaits always carry a short timeout (15–20s) so nothing hangs.
- Synchronization: where the user sends and the bot waits, the bot await starts
  in the background (`await_bg`) with a `sleep` for the dispatcher to settle,
  then the send; capture the PID and `wait`.
- Unique markers: each test message carries a nonce (`[e2e] <scenario> <RANDOM>`)
  so the assertion matches its own text, not stale/other messages.
- Retry wrapper for "arrived / not arrived": one retry of the step on a live
  race, then fail.
- Rate-limit: small `sleep` between heavy steps to avoid FLOOD_WAIT on the test
  account.
- Cleanup: end of each scenario best-effort `delete` of its own messages
  (`--for-me` to avoid spamming the partner with deletions).

Security:
- No secret is written to the repo; `TGC_BOT_TOKEN` is read from the environment
  (or `.env`, which is gitignored) at runtime only.
- Scripts do not echo the token.
- Test accounts only; scripts never touch the `default` profile unless explicitly
  pointed at it via env.

## 5. Testing (of the harness itself)

- `bash -n` (syntax) + `shellcheck` clean on every script.
- `setup.sh` is safe to re-run (idempotent bot login, non-destructive checks).
- A dry smoke: `run-all.sh` against the real test accounts should end with a
  PASS/FAIL summary and a correct exit code.
- Each scenario file is independently runnable and self-cleans.
