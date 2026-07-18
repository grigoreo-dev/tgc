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

**Ordering matters — dispatcher first, then drain.** If we drained history first
and *then* started the dispatcher, a message arriving in that gap would be lost
(not in the drain, not yet live). So we start buffering live updates *before*
reading history, and the two windows deliberately overlap; a `map[int]bool` on
message id **deduplicates** the merge (a message straddling the seam may appear
in both the drain and the live buffer).

1. Resolve peer, build InputPeer.
2. `ConnectWatch` — dispatcher is up and already buffering live inbound into the
   channel.
3. **Startup drain** (correctness, not optimization) — **user profiles only;
   skipped entirely for bot profiles** (bots have no dialogs/read-pointer, so they
   are live-only): read the accumulated unread deterministically:
   a. `messages.getPeerDialogs([peer])` → `read_inbox_max_id`, `unread_count`,
      `top_message`.
   b. If `unread_count == 0` → buffer empty, go straight to live (no history call).
   c. Else `messages.getHistory(peer, min_id = read_inbox_max_id, limit = min(unread_count, 100))`
      → the unread inbound messages (cap 100 as a sanity ceiling).
   d. Filter `id > read_inbox_max_id && out == false` (getHistory `min_id` is not
      always strictly exclusive).

   We do NOT rely on the dispatcher's getDifference to redeliver already-unread
   messages, because getDifference is `pts`-relative, not read-pointer-relative —
   a session whose `pts` is already current would report an empty gap and never
   replay the standing unread. The read-pointer (`read_inbox_max_id`) is the
   authoritative "what has the agent already seen" marker. Merge the drained
   messages into the buffer, deduplicating by message id against anything the
   live dispatcher already captured.
4. Live messages continue arriving via `handlers.NewMessage`. A message is
   accepted into the buffer only if **all** hold (the same filter chain applies to
   both the live path and the startup drain):
   - `peer == target` (matches our chat_id),
   - `out == false` (inbound, not our own echo — matters for `send --await-reply`),
   - it is a `*tg.Message`, not a `*tg.MessageService` (join/title/photo service
     events are dropped — no text for the agent),
   - if `--from` is set: `sender_id == resolve(--from)`.
5. **Debounce timer**: each new accepted message resets a `--debounce` timer.
6. `select` over: debounce-fire (buffer non-empty) / timeout deadline.
7. debounce-fire → stop client, return buffer.
8. **timeout deadline reached:**
   - buffer **non-empty** → flush it now (return the batch), do NOT emit a
     timeout marker. The deadline is a hard ceiling: debounce must never let the
     deadline swallow already-received messages (race: message arrives at t=299
     with a 2s debounce and a 300s deadline).
   - buffer **empty** → return `(nil, true, nil)` (timeout marker).

**mark-read** lives in `ops.MarkRead` (`messages.readHistory`, `max_id`); the CLI
calls it **after** `output.Emit`. Two invariants (correctness, not detail):

- `max_id` = the id of the **last message in the emitted batch**, never a
  whole-chat read. A message that arrives after the buffer was sliced stays
  unread and surfaces on the next `await` — no message is marked read without
  being shown to the agent.
- The watch client is **stopped before** `MarkRead`, so no new messages drip into
  a dead buffer while readHistory runs.
- **Bot profiles skip MarkRead** (no read state) — live-only, forward-only.

**CLI** (`internal/cli`): new `await.go` command; `send.go` extended with
`--await-reply` (after send, call the same `Await` + `MarkRead`).

Boundaries:

- `client.ConnectWatch` — connection with updates enabled.
- `ops.Await` — buffer, debounce, filter, startup drain (pure receive logic).
- `ops.MarkRead` — read receipt.
- `cli` — flags, output ordering, mark-read after print.

**Concurrency (single-owner goroutine, no mutex):** the dispatcher callback runs
in its own goroutine but does the minimum — it only sends the message to a
buffered `chan *tg.Message`. All state (buffer append, debounce timer reset,
deadline check) lives in one `select` loop owned by the main goroutine:

```go
for {
    select {
    case m := <-msgCh:   buf = append(buf, m); timer.Reset(debounce)
    case <-timer.C:      if len(buf) > 0 { return buf, false, nil } // debounce fire
    case <-deadline.C:   if len(buf) > 0 { return buf, false, nil } // flush, not timeout
                         return nil, true, nil                       // empty → timeout
    }
}
```

The buffer is touched by exactly one goroutine, so there is no mutex and no data
race by construction. The event channel is also the clean seam for unit tests
(feed `*tg.Message` structs, assert batching/debounce/timeout).

## 4. Errors & edge cases

| Case | Behavior |
|---|---|
| not_authenticated | structured error, exit 1 (as everywhere) |
| timeout, empty | `{"status":"timeout"}`, **exit 0** (normal outcome, not error) |
| bot profile | **live-only**: dispatcher works for bots, so `await` / `--await-reply` catch *future* incoming, but the startup drain (needs `getPeerDialogs`/`read_inbox_max_id`, which bots lack) and mark-read (bots have no read state) are **skipped**. A bot waits forward-only; messages sent before the wait started are not backfilled. No error. |
| `--from` unresolvable | `not_found` before waiting (fail fast) |
| chat empty then new msg | wait, catch live, emit |
| mark-read fails | messages already emitted/received; best-effort, log to stderr, exit on print success (next await may re-show — acceptable) |
| SIGINT | clean stop; already-printed output stays |
| FLOOD_WAIT | existing middleware (≤30s auto-retry) covers read/getHistory |
| media-group (album) | arrives as N `UpdateNewMessage` near-simultaneously → debounce coalesces into one batch (the motivating case) |
| own message echo | dispatcher may deliver outgoing; filtered out (`out == false`) |
| `send --await-reply` send fails | if the send itself errors (flood, PEER_ID_INVALID, …), propagate that error and exit 1 — do **not** enter the wait (nothing was sent to reply to) |
| `--debounce` > `--timeout` | the deadline is a hard ceiling, so debounce is effectively capped by timeout: at the deadline a non-empty buffer flushes (branch: debounce↔timeout) |

Security:

- `mark-read` is a visible server-side side effect (read receipts to the peer).
  Intentional and required (the agent genuinely read them). Documented explicitly.
- `--from` is a buffer filter, not a leak.
- No new secrets/tokens. AccessHash never emitted (`messageToMap` omits it).
- Relation to future read-only mode (tgc-ayp): `await` reads but mark-read
  mutates; when read-only lands, `await` stays allowed, possibly with
  `--no-mark-read`. Not built now, only noted.
- **Invariant:** the watch client is **always** closed via `defer conn.Close()`
  in the CLI wrapper — including on panic in a dispatcher callback or on timeout —
  so a long-lived connection/session is never left dangling. `ConnectWatch` lives
  at most `--timeout` (≤300s default); it is not a daemon.

## Scale / concurrency limit (documented)

Two concurrent `await`/`watch` on the **same profile** is **not supported** in v1.
A profile is a single MTProto session (SQLite `session.db`); two `NoUpdates:false`
consumers on one auth key compete for the same updates state (`pts`) — Telegram
delivers each update once, so the two would steal updates from each other, and the
session DB would see concurrent writers.

This is fine for the actual use case: an agent loop calls `await` **sequentially**
(one conversational turn at a time). Genuinely parallel agents should use
**separate profiles** (local `./.tgc` already encourages this). No lockfile in v1
(YAGNI); if a hard guarantee is later needed, file a separate bead for a
profile-level session lock (second `await` waits or fails `busy`).

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

## Stress Test Results: await-messages design

### Resolved Decisions
- **Startup drain**: explicit `getPeerDialogs` (read_inbox_max_id, unread_count) → `getHistory(min_id, cap 100)`, not a vague "getHistory by read-pointer".
- **Debounce↔timeout**: deadline is a hard ceiling — non-empty buffer flushes at the deadline; timeout marker only when the buffer is empty. No lost message at t=299 with a 2s debounce.
- **mark-read**: strictly `max_id = last emitted message`, never whole-chat; client stopped before MarkRead. Messages after the slice stay unread → next await.
- **Concurrency**: single-owner goroutine + buffered channel (callback only sends); no mutex, no data race by construction; channel is the unit-test seam.
- **Filter chain**: `peer==target && out==false && *tg.Message (drop MessageService) && (--from if set)`, applied to both live and startup drain.
- **Lifecycle ordering**: dispatcher-first, then drain; windows overlap; dedup by message id on merge — closes the seam-gap.
- **Security**: no new vuln (read-receipts documented, AccessHash not emitted); invariant added — watch client always closed via defer (panic/timeout safe); not a daemon (≤ timeout).
- **Scale**: two concurrent watch on one profile NOT supported in v1 (documented); parallel agents use separate profiles; session-lock deferred to a future bead.
- **Bot profile**: live-only — dispatcher works, but startup drain + mark-read skipped (bots lack dialogs/read-pointer/read-state); forward-only, no error.

### Changes Made
- Startup-drain algorithm made explicit (2-step, cap 100, filter).
- Deadline-as-hard-ceiling flush rule for non-empty buffer.
- mark-read max_id + stop-before-read invariants.
- Single-owner-goroutine concurrency model (removed mutex).
- Filter chain incl. MessageService drop, in both paths.
- Dispatcher-first ordering + id dedup.
- defer-close security invariant.
- Concurrency limit section (no lockfile v1).
- Bot live-only behavior (replaces hard bad_args).
- Reflexion: `send --await-reply` send-failure → propagate, don't wait; `--debounce`>`--timeout` capped by deadline.

### Deferred / Parking Lot
- Global inbox / `--follow` / edit-delete events (out of v1 scope).
- Profile-level session lock for concurrent watch (file bead if needed).
- Bot startup-drain/backfill (needs bot-specific mechanism).
- `--no-mark-read` when read-only mode (tgc-ayp) lands.

### Confidence Assessment
- Overall: High.
- Areas of concern: gotgproto dispatcher exact API for per-peer NewMessage filtering + graceful stop mid-wait — verify against the library during implementation (thin wrapper, live-tested via the bot harness).
