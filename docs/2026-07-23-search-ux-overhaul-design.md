# Search UX Overhaul — Design

**Date:** 2026-07-23
**Bead:** tgc-bja (brainstorming session: tgc-60t)
**Status:** Approved pending spec review

## Problem

Search is spread across three surfaces that confuse agent users:

- `search <q>` → chats/contacts (`SearchChats`)
- `search <q> --messages` → message search (`SearchMessages`)
- `read <chat> --search <q>` → in-chat message search (`ops.Read` with `Search`)

The original bead assumed "search across my dialogs" was impossible in one
call. That premise is false: `messages.searchGlobal`, despite the name,
searches messages **in chats known to the user** (TDLib: "messages in all
chats known to the user") — it *is* the search-my-chats primitive. No client
filtering or fan-out is needed.

## Evidence (model UX experiments)

Three rounds of `opencode run` experiments (sonnet, haiku, grok-4.5; blind
runs sequential, others parallel; artifacts in `/tmp/opencode/search-ux-exp/`):

1. **Help-based A/B/C** (18 runs): `search <q>` for messages and an in-chat
   restriction flag scored 18/18 across all variants. Boolean inversion flag
   (`search --chats`, variant C) failed on sonnet (2/6 misrouted to `read`).
2. **Adversarial help-based A/B** (12 runs): cross-worded tasks ("Search for
   the channel…" / "Find all messages…") — both `find` (B) and
   `chats --find` (A) routed 12/12 semantically; A provoked one invented
   `--find` flag on `search` (would fail at runtime).
3. **Blind priors, no help** (15 runs): no model ever guessed a `find`
   command (0/15). Consensus grammar models invent: `search messages <q>`,
   `search <category> <q>` (category: chats/user/group/channel), `--chat
   <peer>` for in-chat (never `--in`), `info <chat>` (15/15).

Conclusion: a single `search` command matching blind priors, with an
optional entity/kind filter, beats both a separate `find` command and any
boolean mode flag.

## Design

### CLI surface

```
tgc search <query>                    # DEFAULT: peers + messages (both sections)
tgc search <query> --type chats      # peers only (contacts.Search: own + global public)
tgc search <query> --type messages   # messages only (messages.SearchGlobal)
tgc search <query> --type user       # both sections, private-chat kind only
tgc search <query> --type group      # both sections, group kind only
tgc search <query> --type channel    # both sections, channel kind only
tgc search <query> --chat <peer>     # in-chat search (messages.Search); peer selector as in read
tgc search <query> --from <user>     # sender filter; requires --chat (API limitation)
tgc search <query> --since <date> --until <date>   # YYYY-MM-DD or RFC3339; both modes
tgc search <query> --limit N         # default 20, per section
```

- Default (no `--type`, no `--chat`): two RPCs — `contacts.Search` and
  `messages.SearchGlobal`. Mirrors Telegram Desktop's main search (sections:
  contacts & chats / global / messages).
- `--type user|group|channel` filters **coherently across both sections**:
  peers of that kind, plus messages via SearchGlobal's
  `users_only`/`groups_only`/`broadcasts_only` flags.
- No subcommands, no reserved words: any query string is safe
  (`tgc search chats` searches for the literal word "chats").

### Flag validation (bad_args)

- `--type` with `--chat` → error: `--type` applies to global search only
- `--from` without `--chat` → error: global search cannot filter by sender
- `--type` outside `chats|messages|user|group|channel` → error listing values
- `--since`/`--until` parse errors → existing date-parse error path

### Output

JSONL, one row per result. Every row gains a discriminator field:

- `result: "chat"` — peer rows (shape of today's `SearchChats` output)
- `result: "message"` — message rows (shape of today's `SearchMessages`
  output: per-peer chat_id, rich auto-fetch, `stripRichMapKeys`)

Section order: chats first, then messages. `--limit` applies per section.

### Removed surfaces (clean break, no aliases)

- `read --search` flag (and `ops.ReadOpts.Search` routing into
  `MessagesSearch`) — `read` keeps `--from/--since/--until` as read filters.
- `search --messages` flag.
- Peer-search-as-default `search` behavior.

Rationale: users are agents; they re-read `--help` every session. A crisp
unknown-flag error is a better migration than a deprecation period.
This is a breaking CLI change and ships as such.

### Bot accounts

`search` becomes bot-unsupported in all modes (existing `bot_unsupported`
preflight): bots cannot call `contacts.Search`, `messages.SearchGlobal`, or
search history. Single preflight on the command.

## Internals

- `internal/cli/chats.go`: `searchCmd` rewritten (flags: `--type`, `--chat`,
  `--from`, `--since`, `--until`, `--limit`); `--messages` deleted.
- `internal/ops/chats.go`:
  - `SearchChats` retained as the peers-section engine (kind filter added).
  - `SearchMessages` extended: SearchGlobal kind flags
    (`UsersOnly`/`GroupsOnly`/`BroadcastsOnly`), `MinDate`/`MaxDate`.
- `internal/ops/messages.go`: new in-chat search op (extracted from `Read`'s
  `MessagesSearch` branch — reuse, not duplicate: `--from` maps to `FromID`);
  `ReadOpts.Search` removed.
- `internal/cli/read.go`: `--search` flag removed.
- New `ops.Search(conn, query, opts)` orchestrator returning tagged rows.

## Error handling

- Default mode: if one section's RPC fails, emit the other section plus a
  `warning` line (`output.Warnf`) — partial results beat hard failure.
  Explicit `--type`/`--chat` modes fail hard (single RPC, nothing to salvage).
- FLOOD_WAIT: existing `isFloodWait` handling in rich auto-fetch unchanged.

## Testing

- Unit: flag validation matrix (bad_args cases), `--type` → RPC flag
  mapping, tagged-output shape, per-section limit.
- e2e (own profile, serialized as usual): default two-section search,
  `--type chats`, `--type messages`, `--chat` in-chat search, `--from`
  with `--chat`, literal-word queries (`tgc search chats`).
- Regression: `read` without `--search` still passes existing e2e.

## Docs

- `README.md` / `README.ru.md`: command tables (`search` row rewritten,
  `read --search` removed), bot limitations section.
- `docs/integration-checklist.md`: search entries updated.
- Completion scripts regenerate from cobra automatically.

## Out of scope

- `channels.searchPosts` (true all-of-Telegram public post search) — possible
  future `--type posts`.
- Pagination/offsets for search results (tracked separately if needed).
- `tgc find` or any peer-search alias — rejected by blind-prior evidence.
