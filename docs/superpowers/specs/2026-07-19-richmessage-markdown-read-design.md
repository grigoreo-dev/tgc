# RichMessage → Markdown read projection — Design

**Date:** 2026-07-19
**Status:** Approved (brainstorming)
**Bead (brainstorm session):** tgc-fvz
**Related bug:** rich messages arrive invisible to the user (`text:""`)

## Problem

Telegram Bot API 10.1 (June 2026) introduced **Rich Messages**: a message
whose body lives in `tg.Message.RichMessage` (an Instant-View / Telegraph block
tree — `PageBlock*` blocks whose inline text is a `RichText*` tree, plus
attached `Photos`/`Documents`), while the plain `tg.Message.Message` field is
**empty**.

tgc's message projection `messageToMap` (`internal/ops/messages.go:169`) reads
only `m.Message`. It ignores `m.RichMessage` entirely. As a result **rich
messages are completely invisible to a tgc user**: `read`/`context`/`await` emit
`{"text":"","media":null}` for a message that actually carries headings, tables,
formatted text, media, etc.

Verified live: @richtextdemobot and our own @tgcdevbot (`send --rich`) both
deliver messages the tgc user reads as empty `text`. Raw MTProto
(`messages.getHistory`) confirms `Message=""`, `Entities=0`, `Media=nil`, and a
large populated `RichMessage` tree.

This is silent data loss on the read path — the mirror of the send-side bug
(tgc-9xa) already fixed, where a bot's rich send dropped text.

## Goals

- Make rich messages legible to an agent by rendering `RichMessage` to Markdown
  and surfacing it in the existing `text` field.
- Never silently drop content: unknown blocks/text degrade to a readable
  placeholder plus any extractable inner text.
- Zero regression for ordinary (non-rich) messages.

## Non-goals (separate beads / separate e2e)

- **reply_markup / inline buttons** projection (buttons are currently invisible too).
- **entities** projection (bold/italic/link URL on ordinary messages).
- **Pressing inline buttons** (`messages.getBotCallbackAnswer`) — needed to reach
  @richtextdemobot's rich samples; out of scope.
- **Edited-message handling in `--await`** — the demo bot edits its menu message
  on navigation; await only catches new messages today. Out of scope.
- **HTML serializer** for upstream — deferred (see Strategy).
- **Media download** of rich media — out of scope; media rendered as inline MD
  reference lines only.

## Strategy

**Vendor-first, core-complete, Instant-View semantics.** No rich→text renderer
exists in gotd (only send-side MD/HTML→rich builders) nor any open upstream PR;
official clients (tdlib, tweb PR #669) render rich blocks with their Instant-View
block renderer. We write our own renderer in `internal/markup`, following the same
block semantics, core-complete with graceful fallback. Upstreaming a renderer to
gotd is an **optional follow-up bead**, non-blocking.

**Traversal is separated from serialization.** The renderer walks the block/text
tree and drives a serializer interface. v1 ships only a **Markdown serializer**
(agent-oriented, compact — `**bold**` beats `<b>bold</b>` for an LLM and for
tokens). A future **HTML serializer** (lossless, browser-oriented) can be added
over the same traversal as the upstream contribution — that is why the split
exists now, but HTML is NOT implemented in v1.

## Architecture

Three pieces:

### 1. `internal/markup/renderrich.go` — pure renderer (no network)

```go
// RenderRichMessage renders a RichMessage block tree to Markdown.
func RenderRichMessage(rm tg.RichMessage) string
```

- Walks `rm.Blocks []tg.PageBlockClass`; each block renders to a block-level
  Markdown chunk, chunks joined by `\n\n`.
- Inline `tg.RichTextClass` trees render to inline Markdown.
- Traversal separated from serialization so an HTML serializer can be added later
  over the same walk. v1 = Markdown serializer only.
- No tgc-specific types in the core (input `tg.RichMessage`, output `string`) so
  it can be lifted to gotd upstream cleanly.
- No network, no extra resolves (custom emoji → its `Alt` string; media → inline
  reference lines only).

### 2. `messageToMap` extension (`internal/ops/messages.go`)

After the existing `text: m.Message`:

- If `m.Message == ""` **and** `m.RichMessage` is non-empty (`!rm.Zero()` / has
  Blocks): set `out["text"] = markup.RenderRichMessage(rm)` and `out["rich"] = true`.
- If both empty: unchanged (`text:""`, no `rich` key).
- Non-rich messages (non-empty `m.Message`): completely untouched — no `rich`
  key, `text` as before. **Zero regression.**

### 3. `Part=true` auto-fetch (ops layer, over messageToMap)

`RichMessage.Part == true` means the inline copy is truncated; the full version
is fetched via `messages.getRichMessage{Peer, ID}` (present in gotd v0.153.0,
returns `MessagesMessagesClass`).

Because `messageToMap` is a pure, network-free function, the auto-fetch lives in
the ops callers (`Read`, `Context`, `Await`) that already hold a `*client.Conn`:

- After building the map, if `m.RichMessage.Part == true`, call
  `conn.Ctx.Raw.MessagesGetRichMessage(ctx, {Peer: ip, ID: m.ID})`, take the full
  message's `RichMessage`, render that instead.
- On RPC failure (FLOOD_WAIT / rate-limit / network): **do not fail the read.**
  Keep the truncated inline render and set `out["rich_truncated"] = true`; emit a
  stderr warning (same pattern as the existing `mark_read_failed` await warning).

### Data flow

```
getHistory / update → *tg.Message
    → messageToMap  (inline RichMessage → Markdown, rich:true)
    → [if Part] getRichMessage → render full (or rich_truncated:true on failure)
    → map → JSONL
```

## Block / text mapping (Instant-View semantics)

### RichText (inline)

| RichText | Markdown |
|---|---|
| `TextPlain` | text |
| `TextBold` | `**…**` |
| `TextItalic` | `_…_` |
| `TextUnderline` | `<u>…</u>` |
| `TextStrike` | `~~…~~` |
| `TextSpoiler` | `\|\|…\|\|` |
| `TextFixed` | `` `…` `` |
| `TextMarked` | `==…==` |
| `TextURL` | `[text](url)` |
| `TextEmail` | `[text](mailto:…)` |
| `TextPhone` | `[text](tel:…)` |
| `TextConcat` | concatenation of children |
| `TextAnchor` | child text (anchor dropped inline) |
| `TextMath` | `$source$` |
| `TextCustomEmoji` | `Alt` string |
| `TextSubscript` / `TextSuperscript` | `<sub>…</sub>` / `<sup>…</sup>` |
| `TextDate` | child text |
| `TextMention*` / `TextHashtag` / `TextCashtag` / `TextBotCommand` / `TextBankCard` / `TextAutoURL/Email/Phone` | inner text as-is |
| `TextEmpty` | `""` |
| `TextImage` / `TextWithEntities` / unknown | fallback: inner text |

### PageBlock (block-level, joined by `\n\n`)

| PageBlock | Markdown |
|---|---|
| `Title` / `Heading1` | `# …` |
| `Subtitle` / `Heading2` | `## …` |
| `Header` / `Heading3` | `### …` |
| `Heading4/5/6` | `#### …` … `###### …` |
| `Subheader` / `Kicker` | `**…**` |
| `Paragraph` | text |
| `Preformatted` | ` ```lang\n…\n``` ` |
| `Blockquote` / `Pullquote` | `> …` (+ caption line) |
| `BlockquoteBlocks` | `>` per nested block |
| `List` | `- item` (checkbox → `- [x]` / `- [ ]`) |
| `OrderedList` | `1. item` (uses `Num`) |
| `Divider` | `---` |
| `Math` | `$$\n source \n$$` |
| `Table` | Markdown table (header row → `\|---\|`) |
| `Details` | `**Title**` + `> summary/blocks` (never drop title/summary) |
| `Photo` | `![photo](caption)` |
| `Video` | `[video: caption]` |
| `Audio` | `[audio: caption]` |
| `Collage` / `Slideshow` | media lines in sequence |
| `Map` | `[map: lat,long]` |
| `Cover` | render nested block |
| `Anchor` | skipped (invisible anchor) |
| `Footer` | `---\n<footer text>` |
| `AuthorDate` | `_author, date_` |
| `Embed` / `EmbedPost` / `Channel` / `RelatedArticles` / `Thinking` / `Box` / `Unsupported` / unknown | `[block: <TL-type>]` + extractable inner text |

### Fallback rule

Any unrecognized `PageBlockClass` / `RichTextClass` → extract inner text if
present, else `[unsupported block: <TL-type>]`. **Never a silent empty.**

## Error handling / safety

- Unrecognized block/text → placeholder + inner text; never silently empty.
- `Part=true` auto-fetch failure → truncated render + `rich_truncated:true` +
  stderr warning; read never fails.
- Empty `RichMessage` (`Zero()`) → current behavior (`text:""`, no `rich` key).
- **Recursion depth cap** (e.g. maxDepth 32) on nested blocks
  (Details/Blockquote/Collage) to defend against a maliciously deep tree.
- **Output size cap**: if rendered Markdown exceeds a sane limit, truncate and set
  `rich_truncated:true` (rich-bomb defense).
- No network in the pure renderer; custom emoji use `Alt` (no document resolves).

## Testing

1. **Golden fixture from real @richtextdemobot** (the "All Types Demo" message,
   which exercises headings, bold/italic/underline/strike/spoiler/marked/fixed,
   math, dates, custom emoji, blockquote, lists, ordered lists, preformatted,
   details, table, photo/video/audio/collage/map, footnotes, links/email).
   Capture the raw `tg.RichMessage` via a one-off dump script (not committed),
   serialize it into `internal/markup/testdata/` (TL-encoded `.bin` or Go literal).
   Test: `RenderRichMessage(fixture)` compared to a reviewed `.md` golden file.
2. **Table-driven unit tests** per mapping: bold, spoiler, table, math, list,
   nested blockquote, unsupported fallback, maxDepth cap, size cap.
3. **messageToMap regression:** non-rich message (non-empty `m.Message`) → no
   `rich` key, `text` unchanged; rich-only → `text` = MD, `rich:true`.
4. **Live e2e (separate e2e task, self-contained on our own bot):**
   `@tgcdevbot send <user> "body" --rich '{...markdown...}'` (payload structurally
   equivalent to @richtextdemobot output), then user `read @tgcdevbot` asserts
   non-empty `text` containing expected Markdown fragments and `rich:true`. No
   third-party button-pressing required.

## Follow-up beads (filed, out of scope here)

- Upstream a `RichMessage → Markdown`/`HTML` renderer to gotd.
- Project `reply_markup` (inline buttons: text + callback_data/url).
- Project message `entities` (bold/italic/code/link URL) on ordinary messages.
- Press inline buttons (`messages.getBotCallbackAnswer`) + e2e.
- Handle edited-message updates in `--await` + e2e.
