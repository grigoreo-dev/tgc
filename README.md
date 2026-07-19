<div align="center">

<img src="docs/assets/banner.png" alt="tgc — agent-first Telegram CLI" width="100%">

# tgc

**An agent-first Telegram CLI, written in Go.**

Speaks MTProto through [gotgproto](https://github.com/celestix/gotgproto) and
[gotd/td](https://github.com/gotd/td) — writes compact JSONL to stdout and
structured JSON errors to stderr, so an agent can pipe and parse every result.

<p>
  <a href="https://github.com/grigoreo-dev/tgc/releases"><img alt="Release" src="https://img.shields.io/github/v/release/grigoreo-dev/tgc?style=for-the-badge&logo=github&color=39ff14&labelColor=0d1117"></a>
  <a href="https://go.dev"><img alt="Go" src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white&labelColor=0d1117"></a>
  <a href="https://core.telegram.org/mtproto"><img alt="MTProto" src="https://img.shields.io/badge/MTProto-229ED9?style=for-the-badge&logo=telegram&logoColor=white&labelColor=0d1117"></a>
  <img alt="Output" src="https://img.shields.io/badge/output-JSONL-39ff14?style=for-the-badge&logo=json&logoColor=black&labelColor=0d1117">
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/grigoreo-dev/tgc?style=for-the-badge&color=8b949e&labelColor=0d1117"></a>
</p>

<p>
  <a href="https://github.com/grigoreo-dev/tgc/actions"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/grigoreo-dev/tgc/ci.yml?style=flat-square&logo=githubactions&logoColor=white&label=CI&labelColor=0d1117&color=39ff14"></a>
  <a href="https://goreportcard.com/report/github.com/grigoreo-dev/tgc"><img alt="Go Report" src="https://goreportcard.com/badge/github.com/grigoreo-dev/tgc?style=flat-square"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-linux%20%7C%20macOS%20%7C%20windows-8b949e?style=flat-square&labelColor=0d1117">
  <img alt="Agent-first" src="https://img.shields.io/badge/agent--first-%E2%9C%93-22d3ee?style=flat-square&labelColor=0d1117">
</p>

[Русская версия](README.ru.md) · [Install](#install) · [Quick start](#quick-start) · [Commands](#commands) · [Docs](#documentation)

</div>

```console
$ tgc send @user "**hi**" --await-reply
{"id":42,"chat_id":123,"date":"2026-07-19T12:00:00Z","text":"hi","sender_id":123}
{"id":43,"chat_id":123,"date":"2026-07-19T12:00:04Z","text":"hey!","sender_id":456}
```

Pass `--pretty` when a human is reading.

## Install

Recommended (no Go required — downloads a release binary):

```sh
curl -fsSL https://raw.githubusercontent.com/grigoreo-dev/tgc/main/install.sh | sh
```

Env knobs: `TGC_VERSION=vX.Y.Z` for a specific version, `TGC_INSTALL_DIR` for a
custom directory (default `~/.local/bin`), `GITHUB_TOKEN`/`GH_TOKEN` to avoid API
rate limits.

Prefer to inspect first? Download `install.sh`, read it, then run it.

Alternative (requires Go 1.25+):

```sh
go install github.com/grigoreo-dev/tgc/cmd/tgc@latest
```

> A `go install` build reports version `dev` and does **not** auto-check for
> updates or self-update. For automatic update checks and `tgc self update`,
> install via the script above.

### Updating

```sh
tgc self update    # download and install the latest release
tgc self check     # report {"update_available":...} without installing
```

While a newer release is available, tgc prints a one-line
`{"warning":"update_available",...}` to stderr on each run. Set
`TGC_NO_UPDATE_CHECK=1` to disable the check entirely.

> Security: releases are verified by sha256 checksum over HTTPS. Publisher
> signature verification (cosign) is planned for a future release.

## Quick start

Get an `api_id` and `api_hash` at [my.telegram.org](https://my.telegram.org),
then log in:

```sh
tgc auth login --phone +NNN        # user account
tgc auth login --bot-token $TOKEN  # bot account
```

Once logged in:

```sh
tgc chats
tgc send @user "**hi**"
tgc read @user --limit 10
```

## Commands

Run `tgc --help` for the full list; `tgc <command> --help` for each command's
flags.

| Command | Purpose |
|---------|---------|
| `auth`     | Manage Telegram sessions (login, list, export, import, logout). |
| `chats`    | List dialogs (cached 5m; `--fresh` to refresh). |
| `info`     | Show a chat or user card. |
| `members`  | List the members of a group. |
| `search`   | Search chats and contacts; `--messages` for global message search. |
| `read`     | Read chat history, newest first. |
| `await`    | Block until incoming messages arrive, print them, mark them read. |
| `context`  | Show a message with the messages surrounding it. |
| `send`     | Send a message (Markdown by default; `--file` for media; `--await-reply` to wait for the reply). |
| `edit`     | Edit a message you sent. |
| `delete`   | Delete messages (for everyone by default; `--for-me` to keep for others). |
| `forward`  | Forward a message to another chat. |
| `download` | Download media from a message. |

## Output contract

- **stdout** carries results only, as compact JSONL — one JSON object per line.
  This is the default so agents can pipe output straight into a parser.
- **stderr** carries errors, as a single structured JSON line
  (`{"error":"<code>","message":"..."}`), and the process exits 1.
- `--pretty` switches stdout to human-readable tables instead of JSONL.

### Error codes

Errors use a stable set of machine-readable codes:

| Code | Meaning |
|------|---------|
| `ambiguous`          | A selector matched more than one chat; candidates are in the error body. |
| `bad_args`           | Invalid arguments or flags. |
| `bad_config`         | The config file or an env var could not be parsed. |
| `bot_unsupported`    | The command is not available for bot accounts. |
| `flood_wait`         | Telegram rate-limited the request; retry after the wait. |
| `io_error`           | A local filesystem operation failed. |
| `no_api_credentials` | `api_id`/`api_hash` are not set. |
| `no_media`           | The target message has no downloadable media. |
| `not_authenticated`  | No session for the selected profile. |
| `not_found`          | The chat, user, or message could not be resolved. |
| `rich_unsupported`   | The account rejected a `--rich` message. |
| `upload_failed`      | An uploaded file did not come back usable. |

## Environment variables

| Variable | Effect |
|----------|--------|
| `TGC_PROFILE`      | The active profile (same as `--profile`). |
| `TGC_API_ID`       | Telegram `api_id`. |
| `TGC_API_HASH`     | Telegram `api_hash`. |
| `TGC_SESSION`      | Import a session string directly, without a session file. |
| `TGC_CONFIG_DIR`   | Override the config/profile root. `XDG_CONFIG_HOME` is honored too. |
| `TGC_DOWNLOAD_DIR` | Download root; defaults to `~/.tgc/downloads`. |
| `NO_COLOR`         | Disable `--pretty` colors. |

## Profiles

tgc keeps named profiles, selected with `--profile` or `TGC_PROFILE`. Each
profile stores its session under the config directory, and a profile is either a
user login or a bot login. This lets you keep, say, a personal account and a bot
side by side and switch between them per command.

## Local project config (`./.tgc`)

Run `tgc init` in a project directory to create a local `./.tgc` config, so that
directory (and its subdirectories) uses its own default account:

    tgc init --profile work
    tgc auth login

tgc discovers config in this order: `TGC_CONFIG_DIR` → the nearest `./.tgc`
walking up from the current directory (not past your home directory) →
`$XDG_CONFIG_HOME/tgc` → `~/.config/tgc`. A shared parent `workspace/.tgc`
covers all subprojects; a nearer `./.tgc` overrides it. A `.tgc` directly in
your home directory is not used as local config (that is what `~/.config/tgc`
is for).

`tgc init` writes `.tgc/.gitignore` (`*`) so sessions are never committed, and
inherits `api_id`/`api_hash` from your global config if set. Inspect the active
config with:

    tgc config path

## Bot-mode limits

A bot account can't list dialogs or run a global chat search — both return
`bot_unsupported`, because Telegram doesn't expose those to bots. A bot can send
and read in chats it belongs to: address a user by `@username`, or by numeric
user id for someone who has already messaged the bot.

## Await incoming (agent conversations)

When an agent needs the other side's reply rather than a snapshot of history,
`tgc await` blocks until unread messages arrive, prints them, marks them read,
and exits — the wait-for-reply primitive, no polling loop required.

```sh
tgc await @user                       # block until a message arrives
tgc await @user --timeout 30          # give up after 30s
tgc send @bot "ping" --await-reply    # send, then wait for the reply on one connection
```

`await` debounces: it waits for a quiet gap before returning, so a burst (or a
media group) is coalesced into a single batch. Messages print oldest→newest, one
compact JSON object per line — the same shape as `read`:

```json
{"id":42,"chat_id":123,"date":"2026-07-19T12:00:00Z","text":"hi","sender_id":123, ...}
```

On silence until the timeout, `await` prints a marker and exits 0 (a normal
outcome, not an error):

```json
{"status":"timeout","chat_id":123,"waited":30}
```

| Flag | Command | Default | Effect |
|------|---------|---------|--------|
| `--timeout`   | `await` | `300` | Max seconds to wait before emitting the timeout marker. |
| `--debounce`  | `await` | `2`   | Seconds of quiet before returning a batch (`0` = off). |
| `--from`      | `await` | —     | Only messages from this sender (selector). |
| `--await-reply`    | `send` | `false` | After sending, wait for the reply on the same connection. |
| `--await-timeout`  | `send` | `300` | `--await-reply`: max seconds to wait. |
| `--await-debounce` | `send` | `2`   | `--await-reply`: quiet seconds before returning a batch. |
| `--await-from`     | `send` | —     | `--await-reply`: only messages from this sender. |

> **Read-receipt side effect:** `await` marks the awaited messages read, which is
> a **visible action to the contact** (they see your "read" state advance). This
> is intentional — the messages are consumed — but it is not a silent peek.

> **One await per profile:** a profile holds a single MTProto session, so you
> cannot run two `await`s (or a `send --await-reply` alongside another `await`)
> concurrently on the same profile. Parallel agents must use **separate
> profiles** (`--profile` / `TGC_PROFILE`, or a per-project `./.tgc`).

> Bots are **live-only**: no unread backfill and no mark-read, so a bot `await`
> catches only messages that arrive while it is waiting. `--await-reply` is not
> supported together with `--file`.

A dev-only bot-side harness for exercising these paths lives at
[scripts/await-e2e.sh](scripts/await-e2e.sh).

## Rich messages

By default, Markdown renders through Telegram message entities, which every
account supports. A server-side rich-message path also exists, but user accounts
reject it with `RICH_MESSAGE_UNSUPPORTED`, so tgc falls back to entities
transparently — no custom PageBlock AST ships in v1. See
[docs/rich-spike.md](docs/rich-spike.md) for the full investigation and the live
result.

## Contributing

Contributions are welcome. The workflow mirrors CI, so what passes locally passes
in the pipeline.

**Setup**

```sh
git clone https://github.com/grigoreo-dev/tgc
cd tgc
go build ./...
```

**Before you open a PR**, run the same checks CI does:

```sh
go build ./...
go vet ./...
go test ./...
shellcheck install.sh   # if you touched the install script
```

Keep changes focused, follow the existing package layout under `internal/`, and
preserve the [output contract](#output-contract) — results as JSONL on stdout,
structured errors on stderr. User-facing changes should update the README (and
`README.ru.md`).

### Issue tracking with beads (`bd`)

This repo tracks work with [beads](https://github.com/gastownhall/beads) (`bd`),
a git-native, dependency-aware issue tracker. You don't need it to send a PR, but
if you want to pick up existing work or coordinate a larger change:

```sh
bd ready                     # list unblocked, claimable work
bd show <id>                 # read an issue and its dependencies
bd update <id> --claim       # atomically take it (sets you as assignee)
bd close <id> --reason "..." # mark it done
```

As an outside contributor, initialise beads in **contributor mode** so your
planning issues route to a separate local repo and stay out of your PR:

```sh
bd init --contributor
```

Reference the bead id in your commit messages (e.g. `tgc-a1b2: fix flood-wait
backoff`) so the change is traceable back to its issue.

## Documentation

- Design spec: [docs/superpowers/specs/2026-07-13-tgc-telegram-cli-design.md](docs/superpowers/specs/2026-07-13-tgc-telegram-cli-design.md)
- Implementation plan: [docs/superpowers/plans/2026-07-14-tgc-v1-implementation.md](docs/superpowers/plans/2026-07-14-tgc-v1-implementation.md)
- Live integration checklist: [docs/integration-checklist.md](docs/integration-checklist.md)
- RichMessage spike: [docs/rich-spike.md](docs/rich-spike.md)
