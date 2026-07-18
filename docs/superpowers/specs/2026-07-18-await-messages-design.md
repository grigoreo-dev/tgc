# Design: await incoming messages (`tgc await` + `send --await-reply`)

**Date:** 2026-07-18
**Bead:** tgc-8av (feature) · brainstorming session tgc-dsv
**Status:** Approved (pending spec review)

## Problem

tgc is one-shot: every command opens a connection (`NoUpdates:true`), acts, and
exits. An agent holding a back-and-forth conversation therefore has no way to
"wait for the reply" — today it must poll (`tgc read --after <id>` + `sleep` in a
bash loop), which is laggy, burns API calls, and is a hand-rolled crutch.

We want a native primitive: **send a message (or not) and block until the
contact's incoming messages arrive, return them as JSONL, mark them read, exit.**

## Non-goals (v1, YAGNI)

- Global inbox stream across all dialogs.
- Continuous `--follow` / never-exit stream mode.
- Edit / delete / typing / online events (only new inbound messages).
- mentions-only / unread-only global filters.
- Bot-profile support (bots get updates differently; deferred with webhook/gateway).

Explicitly **no infinite stream**: an agent is a "call tool → get output → think"
loop and cannot consume an open-ended stream. Every command must terminate.

## 1. Commands & UX

Two entry points, one core (`ops.Await`):

- **`tgc await <chat> [flags]`** — wait for incoming without sending.
- **`tgc send <chat> "text" --await-reply [flags]`** — send, then wait for reply.

Flags (shared):

| Flag | Default | Meaning |
|---|---|---|
| `--timeout <sec>` | `300` | deadline; on expiry, structured `{"status":"timeout"}`, exit 0 |
| `--debounce <sec>` | `2` | quiet gap after the last message before emitting the batch; `0` = off |
| `--from <selector>` | — | (optional) only messages from this sender (useful in groups) |

Behavior:

- Returns **all unread** messages from the chat: those already accumulated at
  start **plus** any that arrive during the wait.
- After emitting, **marks the chat read** (`messages.readHistory`).
- First batch (after debounce settles) → **exit 0**. Does not keep watching.
- Silence until `--timeout` → `{"status":"timeout"}`, exit 0.
- `send --await-reply`: send first (normal send semantics), then the same wait.

## 2. Output (JSONL contract)

Incoming messages reuse the existing `messageToMap` contract (same shape as
`read`/`send`), one message per line, ordered **oldest→newest** (natural reading
order for a conversation):

```json
{"id":984,"chat_id":390361413,"date":"2026-07-18T09:10:00Z","text":"reply","sender_id":390361413,"sender_name":"...","sender_username":"grigoreo","reply_to":null,"media":null,"edited":false,"fwd_from":null}
```

For `send --await-reply`, the send-result line is printed first, then the
incoming lines:

```json
{"message_id":983,"chat_id":390361413,"date":"..."}
{"id":984,...}
```

Timeout marker (single line, exit 0):

```json
{"status":"timeout","chat_id":390361413,"waited":300}
```

(For `send --await-reply` the send-result line is still printed before a timeout
marker — the message was sent regardless.)

Rules:

- If there are no unread at start, **wait** (do not exit immediately empty) until
  a message arrives or timeout.
- Lines are printed only **after** the debounce cut (as a whole batch), so the
  agent receives one coherent output — not dribbled one at a time.
- `mark-read` happens **after** successful printing, so a print failure never
  loses messages.
- `--pretty` renders via the existing `renderPretty`.

## 3. Architecture

**New connect path** (`internal/client`):

```go
func ConnectWatch(profileName string) (*Conn, error)
```

A sibling of `Connect` with `NoUpdates:false` so the gotgproto dispatcher runs.
The existing `Connect` (one-shot, `NoUpdates:true`) is untouched — all current
one-shot correctness is preserved.

**Core** (`internal/ops/await.go`, new):

```go
type AwaitOpts struct {
    Timeout  time.Duration // default 300s
    Debounce time.Duration // default 2s
    From     string        // optional sender filter
}

// Await returns (messages, timedOut, error).
func Await(conn *client.Conn, selector string, o AwaitOpts) ([]map[string]any, bool, error)
```

Algorithm:

1. Resolve peer, build InputPeer.
2. **Startup drain** (correctness, not optimization): read the accumulated unread
   via `messages.getHistory` bounded by the dialog's read-pointer / `unread_count`
   into the buffer. We do NOT rely on the dispatcher's getDifference to redeliver
   already-unread messages, because getDifference is `pts`-relative, not
   read-pointer-relative — a session whose `pts` is already current would report
   an empty gap and never replay the standing unread.
3. Register `handlers.NewMessage` on the dispatcher; inbound messages whose
   `chat_id` matches our peer (and optional `--from`) append to the same buffer.
   Filter to inbound only (`out == false`) — never echo our own messages.
4. **Debounce timer**: each new message resets a `--debounce` timer.
5. `select` over: debounce-fire (buffer non-empty) / timeout.
6. debounce-fire → stop client, return buffer.
7. timeout with empty buffer → return `(nil, true, nil)`.

**mark-read** lives in `ops.MarkRead` (`messages.readHistory`, max_id = last
message id); the CLI calls it **after** `output.Emit`.

**CLI** (`internal/cli`): new `await.go` command; `send.go` extended with
`--await-reply` (after send, call the same `Await` + `MarkRead`).

Boundaries:

- `client.ConnectWatch` — connection with updates enabled.
- `ops.Await` — buffer, debounce, filter, startup drain (pure receive logic).
- `ops.MarkRead` — read receipt.
- `cli` — flags, output ordering, mark-read after print.

gotgproto note: the dispatcher callback runs in its own goroutine, so the buffer
is mutex-guarded; the debounce fires via a channel/`time.AfterFunc`.

## 4. Errors & edge cases

| Case | Behavior |
|---|---|
| not_authenticated | structured error, exit 1 (as everywhere) |
| timeout, empty | `{"status":"timeout"}`, **exit 0** (normal outcome, not error) |
| bot profile | v1: `bad_args` "await requires a user profile" (bots deferred) |
| `--from` unresolvable | `not_found` before waiting (fail fast) |
| chat empty then new msg | wait, catch live, emit |
| mark-read fails | messages already emitted/received; best-effort, log to stderr, exit on print success (next await may re-show — acceptable) |
| SIGINT | clean stop; already-printed output stays |
| FLOOD_WAIT | existing middleware (≤30s auto-retry) covers read/getHistory |
| media-group (album) | arrives as N `UpdateNewMessage` near-simultaneously → debounce coalesces into one batch (the motivating case) |
| own message echo | dispatcher may deliver outgoing; filtered out (`out == false`) |

Security:

- `mark-read` is a visible server-side side effect (read receipts to the peer).
  Intentional and required (the agent genuinely read them). Documented explicitly.
- `--from` is a buffer filter, not a leak.
- No new secrets/tokens. AccessHash never emitted (`messageToMap` omits it).
- Relation to future read-only mode (tgc-ayp): `await` reads but mark-read
  mutates; when read-only lands, `await` stays allowed, possibly with
  `--no-mark-read`. Not built now, only noted.

## 5. Testing

MTProto is hard to mock (no fake server), so test in layers, maximizing pure
logic that needs no network:

**Unit (no network, the bulk):**

- **debounce buffer** — extract buffer+timer+filter into a testable component
  (`awaitBuffer`) that takes events as structs (no `*client.Conn`): a burst
  within `<debounce` coalesces into one batch; silence `>debounce` fires;
  ordering oldest→newest.
- **timeout** — timer fires before any message → `(nil, true, nil)`.
- **filter** — messages not from the peer / outgoing are dropped; `--from` filter.
- **output ordering** — `send --await-reply` prints the send line before incoming.
- **timeout marker** — JSON shape, exit 0.

`messageToMap` is already covered — reused, not duplicated.

**Automated live e2e** (harness enabled by the bot token in `.env`):

- User side = @konst_moroz via `tgc`; bot side = a script driving the Bot API
  (`curl .../sendMessage`, `sendMediaGroup`) with timed sends.
- @konst_moroz presses Start on the bot once; thereafter the harness is fully
  scriptable (bot → user messages, user-side `await` catches them).
- Cases: `send --await-reply` round-trip; debounce coalescing a burst; media-group
  coalescing; timeout; mark-read (verify read state); `--from` in a group.

**CI:** unit suite green in `go test ./...`; network paths verified via the live
harness (the client layer has `no covering tests` today — consistent with the
project's status quo, now improved by the scriptable bot harness).

Pure functions to isolate from gotgproto:

- buffer + debounce + filter → independent of `*client.Conn`, takes event structs
  → fully unit-testable.
- connect / dispatcher / mark-read → thin wrappers, verified live.
