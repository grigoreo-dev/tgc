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

**`rich:true` means strictly "the `text` field contains a rich render"** — it is
never set unless rendered rich content is present in `text`. Cases:

- `m.Message == ""` **and** `m.RichMessage` non-empty (`!rm.Zero()` / has Blocks):
  `out["text"] = markup.RenderRichMessage(rm)`, `out["rich"] = true`.
- Both non-empty (rare edge — Telegram normally puts the body in one or the
  other): **concatenate** — `out["text"] = m.Message + "\n\n" + render(rm)`,
  `out["rich"] = true`. Never silently drop the rich layer; the flag stays
  honest because `text` really does contain the rich render.
- Both empty: unchanged (`text:""`, no `rich` key).
- Non-rich messages (non-empty `m.Message`, empty `RichMessage`): completely
  untouched — no `rich` key, `text` as before. **Zero regression.**

`has_buttons` flag was considered and **rejected**: Telegram does not allow an
empty-body message, so a "buttons-only, no text" message cannot exist — every
button message already carries plain or rich text, which this feature renders.
Button *projection* (labels/callback_data) remains a separate follow-up bead for
its own reasons, not for emptiness.

### 3. `Part=true` auto-fetch (ops layer, over messageToMap)

`RichMessage.Part == true` means the inline copy is truncated; the full version
is fetched via `messages.getRichMessage{Peer, ID}` (present in gotd v0.153.0,
returns `MessagesMessagesClass`).

Both `messageToMap` and `collectMessages` (`messages.go:131`) are pure,
network-free functions. All message-list paths — `Read` (incl. `--search`/`--from`
via `messages.search`), `Context`, and `SearchMessages` — flow through
`collectMessages`, so the rich→Markdown **render** covers every one automatically.
The `Part` auto-fetch is a **post-processing pass** in the callers that hold a
`*client.Conn` + `InputPeer`:

- After `collectMessages`, for each map whose source message has
  `RichMessage.Part == true`, call
  `conn.Ctx.Raw.MessagesGetRichMessage(ctx, {Peer: ip, ID: m.ID})`, take the full
  message's `RichMessage`, re-render.
- **Per-call auto-fetch budget:** at most `richFetchBudget` (const ≈ 10)
  `getRichMessage` RPCs per command invocation. Beyond the budget, and after the
  **first FLOOD_WAIT**, stop fetching: remaining `Part` messages render truncated
  + `rich_truncated:true`. This prevents a single `read --limit 100` from
  becoming an RPC storm / earning a flood ban.
- On any RPC failure: **do not fail the read.** Keep the truncated inline render,
  set `out["rich_truncated"] = true`, emit a stderr warning (same pattern as the
  existing `mark_read_failed` await warning).

**`await` live handler is excluded from auto-fetch.** The live handler
(`await.go:115`) runs inside the gotgproto dispatcher callback and is deliberately
non-blocking (no network, `select/default`); a synchronous `getRichMessage` there
would block the update loop and break the await concurrency contract. So: live
handler renders the truncated inline version + `rich_truncated:true`, no network.
The await **drain** path (`await.go:199`, already synchronous `getHistory`) may
auto-fetch within the same budget. Async live auto-fetch is a separate follow-up
bead.

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

- **Markdown-injection escaping (security).** Plain text from the rich tree
  (`TextPlain` and inner text) is UNTRUSTED — a sender bot could embed characters
  that are themselves Markdown (`**fake bold**`, `[fake](evil)`, `| table |`,
  `` ``` ``, leading `#`). All Markdown metacharacters in plain text are escaped
  (`\`, `*`, `_`, `` ` ``, `[`, `]`, `|`, leading `#`, etc.) so that **formatting
  is defined only by the rich tree structure, never by text content.** Node
  formatting (`TextBold` → `**`) is applied *around* already-escaped text. This
  prevents a bot from forging formatting/links the original did not contain.
- Unrecognized block/text → placeholder + inner text; never silently empty.
- `Part=true` auto-fetch failure → truncated render + `rich_truncated:true` +
  stderr warning; read never fails.
- Empty `RichMessage` (`Zero()`) → current behavior (`text:""`, no `rich` key).
- **Recursion depth cap = 64** on nested blocks (Details/Blockquote/Collage). On
  hit: truncate the branch, insert `[…]`, set `rich_truncated:true`. (64 leaves
  headroom for legitimately nested Instant-View pages while staying finite.)
- **Output size cap = 64 KB** of rendered Markdown per message. On overflow:
  truncate at a block boundary, append `[…]`, set `rich_truncated:true`
  (rich-bomb defense). Both caps are code constants, not flags (YAGNI).
- No network in the pure renderer; custom emoji use `Alt` (no document resolves).
- **Media:** rich media (`Photos`/`Documents` + Photo/Video/Audio/Map blocks)
  render as **inline MD reference lines** in `text`; the separate `media` field
  stays `null` for rich messages (rich media is not unpacked into `media` in v1).
  Type/caption/filename are read from the `RichMessage` structure itself (no
  network). **Downloading rich media is out of scope** (separate follow-up); the
  inline lines are informational only, with no download handle.
- **Renderer purity / mentions:** the core renderer stays pure —
  `RenderRichMessage(rm tg.RichMessage, resolve map[int64]string) string`, taking
  an OPTIONAL small user_id→display-name map (NOT the whole `conn`, no network).
  `conn`/network never enter the renderer. `TextMention`/`TextMentionName`:
  render via the map if provided, else fall back to the node's own text. This
  keeps the renderer trivially liftable to gotd upstream.

## Testing

1. **Golden fixture from real @richtextdemobot** (the "All Types Demo" message,
   which exercises headings, bold/italic/underline/strike/spoiler/marked/fixed,
   math, dates, custom emoji, blockquote, lists, ordered lists, preformatted,
   details, table, photo/video/audio/collage/map, footnotes, links/email).
   Stored as **TL-encoded `.bin`** (`internal/markup/testdata/richmessage_alltypes.bin`
   = bytes of `m.RichMessage.Encode()`), decoded in-test via
   `(&tg.RichMessage{}).Decode(buf)`. **Not** a Go literal (a ~4.5 KB tree becomes
   hundreds of unreadable, fragile lines). Golden output lives in
   `richmessage_alltypes.golden.md`, eyeballed once at creation, compared
   automatically thereafter. The one-off capture script is documented next to
   `testdata/` (e.g. a `README`/test comment) but **not committed as a build
   target**; if the `.bin` ever fails to decode (e.g. after a gotd upgrade), the
   test emits a clear message ("fixture decode failed — gotd version may have
   changed; re-capture with <script>") rather than a bare panic. TL is
   backward-compatible by constructor-id, so this is a safeguard, not an expected
   failure. gotd upgrades are a separate task.
2. **Table-driven unit tests** with **small handcrafted struct fixtures** (built
   in Go, readable, no external-bot dependency) per mapping: bold, spoiler, table,
   math, list, nested blockquote, unsupported fallback, depth cap (64), size cap
   (64 KB), and **MD-injection escaping** (plain text containing `*`, `[`, `` ` ``,
   `|` must be escaped, not interpreted).
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
- **Async live rich auto-fetch** for `await` (fetch full `Part` version off the
  dispatcher goroutine, then emit).

## Stress Test Results: RichMessage → Markdown read projection

Stress-test bead: tgc-7dn. 11 branches interrogated (9 mapped + 2 added by
reflexion), all resolved.

### Resolved Decisions

- **Part auto-fetch scope (modified):** excluded from the `await` live handler
  (would block the non-blocking gotgproto dispatcher). Auto-fetch only in
  `Read`/`Context`/`SearchMessages`/`await`-drain. Async live fetch → follow-up bead.
- **Auto-fetch budget (agreed):** ≤ ~10 `getRichMessage` per invocation + stop on
  first FLOOD_WAIT; excess renders truncated + `rich_truncated:true`. Prevents
  RPC storm / flood ban on `read --limit N`.
- **MD-injection escaping (agreed, security):** plain text is untrusted; escape
  all MD metacharacters so formatting comes only from tree structure, not content.
- **Media (agreed):** inline MD lines in `text`; `media` stays `null` for rich;
  rich-media download out of scope. Documented.
- **Caps (agreed):** recursion depth 64, output 64 KB; overflow →
  truncate + `rich_truncated:true`; constants not flags.
- **Fixture (agreed):** one real `.bin` + `.golden.md` from @richtextdemobot,
  plus small handcrafted struct fixtures for table-driven branches; decode-error
  gives a recapture hint; capture script documented, not committed.
- **Renderer purity (agreed):** core takes `tg.RichMessage` + optional
  `map[int64]string` (no `conn`, no network); mentions fall back to node text.
- **All list paths via collectMessages (agreed):** render covers
  read/context/search automatically; Part auto-fetch is a post-pass in the
  callers holding `conn`+`ip`.
- **Both-non-empty edge (modified twice):** `rich:true` means strictly "`text`
  contains a rich render." When both `Message` and `RichMessage` are non-empty,
  **concatenate** (`Message + "\n\n" + render`) — never mislabel plain as rich,
  never silently drop the rich layer.
- **`has_buttons` flag (rejected via data):** Telegram forbids an empty-body
  message, so "buttons-only, no text" cannot occur; every button message already
  carries text this feature renders. Button projection stays a separate bead for
  its own reasons.
- **Fixture-vs-gotd-version (agreed):** TL is constructor-id backward-compatible;
  clear decode-error + documented recapture script is sufficient safeguard.

### Changes Made

- Rewrote §3 (Part auto-fetch): live-handler exclusion, budget/FLOOD_WAIT stop,
  post-pass over `collectMessages`, covers read/context/search.
- Rewrote §2 (messageToMap): honest `rich:true` semantics + both-non-empty
  concatenation; documented `has_buttons` rejection.
- Error handling/safety: added MD-injection escaping; concrete caps (64 / 64 KB);
  media/`media:null` boundary; renderer-purity signature with optional resolve map.
- Testing: fixture format (`.bin` + golden), handcrafted struct fixtures,
  escaping test, decode-error safeguard.
- Added async-live-auto-fetch follow-up bead.

### Deferred / Parking Lot

- Async live rich auto-fetch (await) — follow-up bead.
- HTML serializer, button projection, entities projection, button-press, await
  edits — pre-existing non-goals.

### Confidence Assessment

- Overall: **High.** Every branch resolved; two code-verified findings
  (dispatcher-block, all-paths-via-collectMessages) hardened the design.
- Areas of concern: none blocking. The rarest edge (both fields non-empty) is
  handled defensively though it may never occur in practice.
