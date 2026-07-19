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

## Running

Build + set up once, then run the whole suite or a single scenario:

```sh
go build -o /tmp/tgc-e2e ./cmd/tgc
set -a; . .env; set +a
export TGC_BIN=/tmp/tgc-e2e TGC_CONFIG_DIR=/path/to/config
export E2E_USER_PROFILE=default E2E_ALLOW_DEFAULT=1 E2E_BOT_PROFILE=e2ebot

# whole suite (takes an exclusive lock, runs setup then 01..07, aggregates):
bash scripts/e2e/run-all.sh

# one scenario (run setup.sh first so .env.generated exists):
bash scripts/e2e/setup.sh
bash scripts/e2e/03-media.sh
```

`run-all.sh` exits `0` when every scenario is green, `1` if any assertion failed,
`2` if `setup.sh` failed, and `3` if another run already holds the lock.

## Scenarios

| File | Covers |
|------|--------|
| `01-send-read.sh` | send / read / reply / context / edit / forward, both directions. |
| `02-await.sh` | `await` burst coalescing, timeout (exit 0), `send --await-reply`, bot live-only inbound. |
| `03-media.sh` | document sha256 round-trip, photo mime/non-empty, album (media-group) sharing one `grouped_id`. |
| `04-dialogs.sh` | user `chats` / `search` / global message search / `read` shape. |
| `05-bot-limits.sh` | bot-forbidden dialog commands (`bot_unsupported` / `not_found` + exit 1). |
| `06-meta.sh` | `auth list` / `config path` / `version` / `self check` (soft) / `--pretty` is non-JSON. |
| `07-rich.sh` | rich message (Bot API 10.1) read renders to Markdown + `rich:true`. |

## Notes on the bot profile

- **Bots are live-only inbound.** A bot cannot `read`/`context`/`chats`/`search`
  (dialog-based → `bot_unsupported`). Its only way to receive a message is
  `await`, which watches live updates.
- **One await per profile.** Only one `await` may run per profile at a time —
  parallel scenarios on the same bot profile would steal each other's updates, so
  the runner executes scenarios **sequentially** (also avoids FLOOD_WAIT).
- **`me` selector, not `@self`.** Self / Saved Messages is the bare `me` selector;
  `@self` parses as a username and resolves to the wrong peer.
