# Live integration checklist

A hands-on checklist for verifying tgc against a real Telegram server with real
credentials. Run each item and compare the output against the expected JSON
shape. Every command below writes compact JSONL to stdout; add `--pretty` when
you want to read it yourself.

> All items in this checklist were executed live during Task 13. Six bugs were
> found and fixed along the way: 2FA password echo, `AUTH_RESTART` self-heal,
> `edit` returning a null message id/date, the `me`/`saved` selector, sending
> from a bot by numeric user id, and the `rich_unsupported` error mapping.

## Prereqs

- `export TGC_CONFIG_DIR=$(mktemp -d)` — run against a throwaway profile dir so
  the live test never touches your real sessions.
- `export TGC_API_ID=...` and `export TGC_API_HASH=...` from
  [my.telegram.org](https://my.telegram.org).
- A test **user** account (phone + login code, optionally a 2FA password).
- A test **bot** token from [@BotFather](https://t.me/BotFather).

## Auth

- [ ] `tgc auth login --phone +NNN` — interactive login. Enter the code when
  prompted. If the account has 2FA, the password prompt reads without echoing
  what you type. On success: `{"status":"ok","profile":"default","type":"user","user_id":...,"username":"..."}`.
- [ ] `tgc auth login --profile bot --bot-token $TOKEN` — bot login. On success:
  `{"status":"ok","profile":"bot","type":"bot","user_id":...,"username":"..."}`.
- [ ] `tgc auth list` — two lines, one per profile, each with the correct
  `type` (`user` / `bot`) and a `default` flag.
- [ ] Export/import round-trip: `tgc auth export` prints `{"session":"..."}`;
  feed that string back via `TGC_SESSION` or a file to `tgc auth import` and
  confirm the profile logs in again.
- [ ] `tgc auth logout` — `{"status":"ok","profile":"default"}`, and the
  session is gone.

Auth self-heals on `AUTH_RESTART`: if the server asks for a restart mid-login,
tgc clears the partial session and retries rather than leaving a broken profile.

## Chats and resolve

- [ ] `tgc chats --limit 5` — up to five dialogs as JSONL. Run it again
  immediately: the second call returns instantly from the 5-minute dialog
  cache. `tgc chats --fresh` bypasses the cache and refetches.
- [ ] `tgc info @user`, `tgc info <id>`, `tgc info me` — a chat/user card.
  `me`, `self`, and `saved` all resolve to your Saved Messages.
- [ ] `tgc search <partial>` — candidate chats/contacts. An ambiguous fuzzy
  match returns an `ambiguous` error whose body carries the candidate list.
- [ ] `tgc members <group>` — the member list for a group you belong to.

## Messages

- [ ] Send plain text, Markdown (`**bold** \`code\``), and `--plain`; reply with
  `--reply <id>`; read from stdin with `echo hi | tgc send <chat> -`.
- [ ] `tgc read <chat> --limit N` — newest first, each message carrying the full
  field contract.
- [ ] Filters: `--search <term>`, `--from @user`, `--since YYYY-MM-DD`,
  `--before <id>`.
- [ ] `tgc context <chat> <id> --radius N` — the message plus N messages on each
  side.
- [ ] `tgc edit <chat> <id> "..."` — returns a real `{message_id, chat_id,
  date}` (not null), and re-reading the message shows the `edited` flag set.
- [ ] `tgc forward <chat> <id> <other>`, and `tgc forward <chat> <id> me`.
- [ ] `tgc delete <chat> <id>` — deletes for everyone; `--for-me` keeps the copy
  for the other side.

## Files

- [ ] `tgc send <chat> --file photo.jpg --caption "**cap**"` — sent as a photo by
  default, caption rendered as Markdown.
- [ ] `--as-document` forces the same file to be sent as a document instead.
- [ ] Album: `--file a --file b` — the messages share one `grouped_id`.
- [ ] `--file doc.pdf` — a non-image is sent as a document.
- [ ] `tgc download <chat> <id>` — writes to `~/.tgc/downloads/<file_id>/<name>`
  (or `$TGC_DOWNLOAD_DIR`) and prints `{path, size, mime, file_name}`.
- [ ] `tgc download <chat> <id> --stdout > out` — raw bytes to the file; no JSON
  on stdout.
- [ ] Eleven `--file` flags in one send returns a `bad_args` error (album max is
  10).

## Bot mode

- [ ] `tgc --profile bot chats` — `bot_unsupported` (bots can't list dialogs).
- [ ] `tgc --profile bot search foo` (without `--messages`) — `bot_unsupported`.
- [ ] `tgc --profile bot send <user_id> "hi"` — works. Bots resolve a numeric
  user id directly, provided that user has messaged the bot first.

## Rich

- [ ] Default block-markdown send (heading + list + quote, no `--rich`) — the
  server rejects `rich_message` for a user account, so the message is delivered
  via the entities fallback, formatting intact, no error.
- [ ] `tgc send <chat> --rich '{"type":"markdown","markdown":"# x"}'` on a user
  account — `rich_unsupported`.

## Output contract

- [ ] Any error puts nothing on stdout, one JSON line on stderr
  (`{"error":"<code>","message":"..."}`), and exits 1.
- [ ] `--pretty` renders human-readable blocks; piping without it gives pure
  JSONL that an agent can parse line by line.
