[Русская версия](README.ru.md)

# tgc

An agent-first Telegram CLI written in Go, speaking MTProto through
[gotgproto](https://github.com/celestix/gotgproto) and
[gotd/td](https://github.com/gotd/td). It writes compact JSONL to stdout and
structured JSON errors to stderr, so an agent can pipe and parse every result;
pass `--pretty` when a human is reading.

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
| `context`  | Show a message with the messages surrounding it. |
| `send`     | Send a message (Markdown by default; `--file` for media). |
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
walking up from the current directory (stopping at `$HOME`) → `~/.config/tgc`.
A shared parent `workspace/.tgc` covers all subprojects; a nearer `./.tgc`
overrides it.

`tgc init` writes `.tgc/.gitignore` (`*`) so sessions are never committed, and
inherits `api_id`/`api_hash` from your global config if set. Inspect the active
config with:

    tgc config path

## Bot-mode limits

A bot account can't list dialogs or run a global chat search — both return
`bot_unsupported`, because Telegram doesn't expose those to bots. A bot can send
and read in chats it belongs to: address a user by `@username`, or by numeric
user id for someone who has already messaged the bot.

## Rich messages

By default, Markdown renders through Telegram message entities, which every
account supports. A server-side rich-message path also exists, but user accounts
reject it with `RICH_MESSAGE_UNSUPPORTED`, so tgc falls back to entities
transparently — no custom PageBlock AST ships in v1. See
[docs/rich-spike.md](docs/rich-spike.md) for the full investigation and the live
result.

## Documentation

- Design spec: [docs/superpowers/specs/2026-07-13-tgc-telegram-cli-design.md](docs/superpowers/specs/2026-07-13-tgc-telegram-cli-design.md)
- Implementation plan: [docs/superpowers/plans/2026-07-14-tgc-v1-implementation.md](docs/superpowers/plans/2026-07-14-tgc-v1-implementation.md)
- Live integration checklist: [docs/integration-checklist.md](docs/integration-checklist.md)
- RichMessage spike: [docs/rich-spike.md](docs/rich-spike.md)
