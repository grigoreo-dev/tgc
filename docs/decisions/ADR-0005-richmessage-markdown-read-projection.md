# ADR-0005: Render incoming RichMessage to Markdown in the `text` field

**Date:** 2026-07-19
**Status:** Accepted
**Related:** beads tgc-fvz (brainstorm), tgc-7dn (stress-test); spec
`docs/superpowers/specs/2026-07-19-richmessage-markdown-read-design.md`;
mirror of tgc-9xa (send-side rich drop)

## Context

Telegram Bot API 10.1 (June 2026) introduced **Rich Messages**: a message whose
body lives in `tg.Message.RichMessage` — an Instant-View / Telegraph block tree
(`PageBlock*` blocks over a `RichText*` inline tree, plus attached
`Photos`/`Documents`) — while the plain `tg.Message.Message` field is **empty**.

tgc's message projection `messageToMap` read only `m.Message`, so rich messages
(sent by bots/channels, e.g. @richtextdemobot, or by our own bot via `send
--rich`) were **completely invisible to the user**: `read`/`context`/`await`
emitted `{"text":"","media":null}` for a message that actually carried headings,
tables, formatted text, and media. This is silent data loss on the read path —
the mirror of the already-fixed send-side bug (tgc-9xa).

No `RichMessage → text` renderer exists in gotd (it has only send-side
MD/HTML→rich builders) nor any open upstream PR. Official clients (tdlib, tweb
PR #669) render rich blocks with their Instant-View block renderer. This is a
current, industry-wide problem (tweb, openclaw were fixing the same thing in the
same weeks).

## Decision

Render `RichMessage` to **Markdown** and surface it in the existing `text` field,
with `rich:true` signalling that `text` contains a rich render.

- **Vendor-first:** a pure renderer in `internal/markup`
  (`RenderRichMessage(tg.RichMessage, optional map[int64]string) string`),
  core-complete over Instant-View semantics with graceful, never-silent fallback.
  Traversal is separated from serialization so an HTML serializer can be added
  later; v1 ships Markdown only (agent-oriented: `**bold**` beats `<b>bold</b>`
  for an LLM and for tokens). Upstreaming to gotd is an optional follow-up.
- **`text` field, not a separate field:** rich-only → `text` = rendered Markdown,
  `rich:true`. Both `Message` and `RichMessage` non-empty (rare) → concatenate
  (`Message + "\n\n" + render`), `rich:true`. Non-rich → untouched (zero
  regression).
- **`Part=true` auto-fetch** via `messages.getRichMessage{Peer,ID}` only in
  non-live paths (`Read`/`Context`/`SearchMessages`/`await`-drain), with a
  per-invocation budget (~10) and a stop on first FLOOD_WAIT. The **await live
  handler is excluded** — it runs in the non-blocking gotgproto dispatcher
  callback and must not do a synchronous RPC.
- **Security:** Markdown metacharacters in untrusted plain text are escaped, so
  formatting is defined only by the tree structure, never by content (prevents a
  bot forging formatting/links).

## Rationale

- The agent consumes `text`; a single field it already reads means zero client
  changes and no "which field?" ambiguity. `rich:true` is honest — set only when
  `text` actually contains a rich render.
- Markdown over HTML/JSON: the user explicitly wanted MD ("rich lays into MD
  well; JSON would be too noisy for an agent"). Official clients confirm the
  Instant-View block mapping to follow.
- Vendor-first over upstream-first: no gotd renderer exists and none is in
  flight; waiting on an upstream PR would leave rich messages invisible
  indefinitely. Keeping the renderer pure (input `tg.RichMessage`, output
  `string`, optional resolve map — never `conn`/network) keeps it liftable to
  gotd later.
- The dispatcher-block and RPC-storm risks were caught in stress-test from the
  code; the budget + live-exclusion protect the await concurrency contract and
  avoid flood bans.

## Consequences

- `messageToMap` gains rich handling; `collectMessages` covers read/context/search
  automatically. Part auto-fetch is a post-pass in callers holding `conn`+`ip`.
- New output keys: `rich:true` (text is a rich render) and `rich_truncated:true`
  (render was capped or the full-version fetch was skipped/failed).
- Safety caps are code constants: recursion depth 64, output 64 KB; overflow →
  truncate + `rich_truncated:true`.
- Rich media renders as inline MD reference lines in `text`; the `media` field
  stays `null` for rich messages, and downloading rich media is out of scope.
- Explicit non-goals become follow-up beads: HTML serializer, `reply_markup`
  (button) projection, message `entities` projection, button-press
  (`getBotCallbackAnswer`), await edited-message handling, and async live rich
  auto-fetch.
- Golden fixture is a real TL-encoded `.bin` from @richtextdemobot plus small
  handcrafted struct fixtures; a decode failure emits a recapture hint.
