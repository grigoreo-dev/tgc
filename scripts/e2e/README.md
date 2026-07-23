# tgc bidirectional live e2e suite

These scenarios exercise `tgc` end-to-end against **real Telegram accounts** — a
user account and a bot — sending and reading actual messages. They are not
hermetic unit tests; they prove the CLI works against the live MTProto API.

## ⚠️ Real messages / test accounts only

Every scenario sends and reads live messages between the two configured
profiles. **Use dedicated test accounts** — never a personal account or a chat
you care about. Messages (including random blobs and albums) land in the real
user↔bot dialog.

## Preconditions

1. **User profile logged in.** A user session must exist for the user profile
   (default `e2euser`, or `default` with `E2E_ALLOW_DEFAULT=1`).
2. **User pressed Start on the bot.** The user must have started the bot once, so
   the bot may DM the user (Telegram requires it).
3. **`.env` with credentials.** `TGC_API_ID`, `TGC_API_HASH`, `TGC_BOT_TOKEN` —
   `source` it before running (`set -a; . .env; set +a`).
4. **`tgc` binary.** Build once: `go build -o /tmp/tgc-e2e ./cmd/tgc` and set
   `TGC_BIN=/tmp/tgc-e2e` (scenarios call `$TGC`).
5. **`python3`, `jq`, `sha256sum`, `flock`** on PATH (present on the dev box).

## Environment variables

| Var | Purpose | Default |
|-----|---------|---------|
| `TGC_BIN` | Path to the tgc binary the scenarios call. | `tgc` |
| `E2E_USER_PROFILE` | User-side profile name. | `e2euser` |
| `E2E_BOT_PROFILE`  | Bot-side profile name. | `e2ebot` |
| `E2E_ALLOW_DEFAULT` | Allow using the `default` profile as the user (guard). | unset |
| `E2E_BOT_GROUP` | A group id where the bot is admin (enables the `members` list assert in 05). | unset (uses the not_found negative case) |
| `E2E_VERBOSE` | `1` echoes each composed `tgc` invocation to stderr. | unset |
| `TGC_CONFIG_DIR` | Config dir holding the profiles' sessions. | — |
| `RICH_E2E_SEND_BIN` | Path to the maintained All Types sender (`rich-e2e-send`). | unset → `go run ./internal/cmd/rich-e2e-send` |
| `E2E_RICH_MEDIA_DIR` | Directory with the five required media basenames for full-live. | `/tmp/demo_media` |
| `E2E_RICH_FIXTURE` | TL fixture path for full-live. | `internal/markup/testdata/richmessage_alltypes.bin` |
| `E2E_RICH_GOLDEN` | Golden Markdown path for full-live. | `internal/markup/testdata/richmessage_alltypes.golden.md` |
| `E2E_RICH_CLEANUP` | `1` deletes the **recipient-side** verified message after full-live. | unset (message kept) |
| `E2E_RICH_SELFTEST` | `1` runs `08-rich-all-types.sh` offline (no Telegram). | unset |
| `TGC_NO_UPDATE_CHECK` | Silence tgc update checks during long live runs. | unset |

## Two verification tiers

| Tier | Entry point | Role |
|------|-------------|------|
| **Routine smoke** | `scripts/e2e/run-all.sh` | Default bidirectional suite: setup + selftest + `01`…`07`. Cheap live checks; no full-media upload. |
| **Full-live release gate** | `scripts/e2e/08-rich-all-types.sh` | Canonical 37-block All Types send/read/golden. **Not** invoked by `run-all.sh`. |

- **`07-rich.sh`** (inside `run-all.sh`): **reduced rich smoke** — bot sends a small Markdown rich payload; user asserts heading/bold/list + `rich:true`. Not a golden or full fixture contract.
- **`08-rich-all-types.sh`**: **canonical full-live release gate** — preflight, recipient baseline, one send of the maintained fixture with remapped media, unique new-row golden match on the **user** profile.

## Profiles

| Role | Typical profile | How addressed |
|------|-----------------|---------------|
| User (recipient / reader) | `e2euser`, or `default` with `E2E_ALLOW_DEFAULT=1` | Reads `@$E2E_BOT_USERNAME` |
| Bot (sender) | `e2ebot` | Sends to numeric `E2E_USER_ID` |

`setup.sh` exports `E2E_BOT_USERNAME`, `E2E_BOT_ID`, and `E2E_USER_ID`. Reusing
`default` as the user requires **both** `E2E_USER_PROFILE=default` and
`E2E_ALLOW_DEFAULT=1`.

## Running — routine suite (`run-all.sh`)

Build + set up once, then run the whole suite or a single scenario:

```sh
go build -o /tmp/tgc-e2e ./cmd/tgc
set -a; . .env; set +a
export TGC_BIN=/tmp/tgc-e2e TGC_CONFIG_DIR=/path/to/config
export E2E_USER_PROFILE=default E2E_ALLOW_DEFAULT=1 E2E_BOT_PROFILE=e2ebot
export TGC_NO_UPDATE_CHECK=1

# whole suite (exclusive lock, setup, selftest + 01..07; does NOT run 08):
bash scripts/e2e/run-all.sh

# one scenario (run setup.sh first so .env.generated exists):
bash scripts/e2e/setup.sh
bash scripts/e2e/03-media.sh
bash scripts/e2e/07-rich.sh
```

`run-all.sh` exits `0` when every scenario is green, `1` if any assertion failed,
`2` if `setup.sh` failed, and `3` if another run already holds the lock.

End-of-run summary shape:

```text
======== TOTAL ========
PASSED: N, FAILED: M, SKIPPED: K
```

## Running — full-live All Types gate (`08-rich-all-types.sh`)

This is the **upload-heavy release gate**. Run it explicitly after building both
binaries and placing media under the media directory.

### Required media basenames (exact)

Under `E2E_RICH_MEDIA_DIR` (default `/tmp/demo_media`), each name must be a
**non-empty regular file**:

1. `photo_0.jpg`
2. `photo_1.jpg`
3. `photo_2.jpg`
4. `dubaiVideo.mp4`
5. `Neon Rain Train.mp3`

Preflight fails closed (no send) if any basename is missing, not a regular file,
or empty; also if golden, fixture, or sender cannot be resolved.

### Gate model (recipient-side)

Cross-profile exact message-id lookup is **invalid**: bot send returns a
bot-local id; the user-visible dialog id differs (live evidence has shown e.g.
bot `257` vs user `1309`). Bot profiles cannot `context`/`read` the dialog
(`bot_unsupported`). Bot-local sender id is **diagnostic only**.

1. Preflight media ×5, golden, fixture, sender.
2. Snapshot newest recipient-side id in the bot chat via **user**
   `read @bot --limit 1` → `baseline` (empty success → `0`; read failure aborts
   **before** send).
3. Send once via `rich-e2e-send` (bot profile → user id).
4. Bounded poll: user `read` for messages with **id > baseline**.
5. Among only those new rows, require exactly one candidate with `rich:true`,
   `rich_truncated != true`, and byte-exact `.text` vs golden.
6. Fail closed on zero or multiple matches.
7. Success line uses the **recipient-side** `message_id`; optional cleanup
   deletes that same recipient id on the user profile.

### Build + run (exact)

Live `08` requires `scripts/e2e/.env.generated` from `setup.sh` (exports
`E2E_BOT_USERNAME`, `E2E_BOT_ID`, `E2E_USER_ID`). Without that env, the script
calls `require_setup`, **SKIPs**, and exits `0` with **no** `VERIFIED` line —
that is **not** a release-gate pass. Offline selftest (`E2E_RICH_SELFTEST=1` /
`--selftest`) does not need setup.

```sh
go build -o /tmp/tgc-e2e ./cmd/tgc
go build -o /tmp/rich-e2e-send ./internal/cmd/rich-e2e-send
set -a; . /path/to/.env; set +a
export TGC_CONFIG_DIR=/path/to/.tgc
export TGC_BIN=/tmp/tgc-e2e RICH_E2E_SEND_BIN=/tmp/rich-e2e-send
export E2E_USER_PROFILE=default E2E_ALLOW_DEFAULT=1 E2E_BOT_PROFILE=e2ebot
export E2E_RICH_MEDIA_DIR=/tmp/demo_media TGC_NO_UPDATE_CHECK=1

# required for live 08 (writes scripts/e2e/.env.generated):
bash scripts/e2e/setup.sh

bash scripts/e2e/08-rich-all-types.sh
```

**Gate pass** means the process exits `0` **and** prints a terminal success
line matching the shape below (recipient id authoritative). SKIP-only /
missing-setup / missing `VERIFIED` is a non-result for release evidence.

Expected success line:

```text
VERIFIED All Types Demo chat=<E2E_USER_ID> message_id=<recipient_id> blocks=37 sender_message_id=<bot_local>
```

Example shape after a successful live gate (ids are environment-specific):

```text
VERIFIED All Types Demo chat=<id> message_id=1310 blocks=37 sender_message_id=<bot_local>
```

### Retention / cleanup

- **Default:** the verified recipient-side message is **left visible** in the
  user↔bot dialog (useful for human inspection after a release gate).
- **Optional delete:** set `E2E_RICH_CLEANUP=1` so the gate deletes the verified
  **recipient** `message_id` via the user profile after success.
- Cleanup never uses the bot-local sender id for deletion.

### Offline selftest (no Telegram)

```sh
E2E_RICH_SELFTEST=1 bash scripts/e2e/08-rich-all-types.sh
# or:
bash scripts/e2e/08-rich-all-types.sh --selftest
```

## Scenarios

| File | Covers |
|------|--------|
| `01-send-read.sh` | send / read / reply / context / edit / forward, both directions. |
| `02-await.sh` | `await` burst coalescing, timeout (exit 0), `send --await-reply`, bot live-only inbound. |
| `03-media.sh` | document sha256 round-trip, photo mime/non-empty, album (media-group) sharing one `grouped_id`. |
| `04-dialogs.sh` | user `chats` / dual `search` / `--type messages|chats` / bad `--type` / `read` shape. |
| `05-bot-limits.sh` | bot-forbidden dialog commands (`bot_unsupported` / `not_found` + exit 1). |
| `06-meta.sh` | `auth list` / `config path` / `version` / `self check` (soft) / `--pretty` is non-JSON. |
| `07-rich.sh` | **Reduced rich smoke** (not full golden): heading + bold + list + `rich:true`. Part of `run-all.sh`. |
| `08-rich-all-types.sh` | **Full-live All Types release gate** (37-block fixture, media remap, recipient golden). **Not** in `run-all.sh`. |
| `selftest.sh` | Offline harness checks (nonces, aggregation); run via `run-all.sh`. |

## Fixture / golden lifecycle

Canonical assets:

- `internal/markup/testdata/richmessage_alltypes.bin`
- `internal/markup/testdata/richmessage_alltypes.golden.md`

See `internal/markup/testdata/README.md` for recapture after a gotd upgrade,
`UPDATE_GOLDEN=1`, and the requirement to review both the 37-block structural
tests and the golden diff before accepting a recapture.

Do **not** commit exploratory helper trees (`internal/richcap`,
`internal/richjson`, `internal/richmd`, `internal/richbuild`). Maintained send
path is `internal/cmd/rich-e2e-send` + `internal/richfixture`.

## Notes on the bot profile

- **Bots are live-only inbound.** A bot cannot `read`/`context`/`chats`/`search`
  (dialog / global-search surfaces → `bot_unsupported`). Its only way to receive
  a message is `await`, which watches live updates. Full-live therefore verifies
  on the **user** profile history, never via bot-local message id lookup.
  Message lookup uses `search <q> --chat <peer>` (not removed `read --search`).
- **One await per profile.** Only one `await` may run per profile at a time —
  parallel scenarios on the same bot profile would steal each other's updates, so
  the runner executes scenarios **sequentially** (also avoids FLOOD_WAIT).
- **`me` selector, not `@self`.** Self / Saved Messages is the bare `me` selector;
  `@self` parses as a username and resolves to the wrong peer.

## Nonces

Test message markers use Markdown-neutral text
`e2e <scenario> <random>` (no square brackets). Assertions match fixed strings,
not bracket regexes.
