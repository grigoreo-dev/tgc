# Rich message spike (Task 11)

## Module inspection (gotd/td v0.153.0, Layer 227)

`gotd/td` at the pinned version already ships the rich-message primitives, so
tgc does **not** need a custom PageBlock AST/translator in v1:

- `tg.InputRichMessageMarkdown{Markdown string, ...}` — TL
  `inputRichMessageMarkdown#4b572c`.
- `tg.InputRichMessageHTML{HTML string, ...}` — TL `inputRichMessageHTML#dacb836a`.
- `tg.InputRichMessage` (PageBlock form) — present, but intentionally unused in v1.
- Both markdown/html constructors implement `tg.InputRichMessageClass`.
- `(*tg.MessagesSendMessageRequest).SetRichMessage(v tg.InputRichMessageClass)` —
  the `rich_message` conditional field on `messages.sendMessage` (and the
  matching flag on `messages.editMessage`).

Layer constant: `const Layer = 227`
(`$(go env GOMODCACHE)/github.com/gotd/td@v0.153.0/tg/tl_registry_gen.go`).

## Decision

Use the **server-side Markdown constructor** (`InputRichMessageMarkdown`) via
`req.SetRichMessage(...)` for rich sends, with a **transparent fallback to the
Task 6 entities path** if the user-layer rejects rich. No custom PageBlock
translator in v1.

### SendText paths

1. **`--rich <json>`** (expert): decode `{"type":"markdown|html", ...}` via
   `markup.ParseRichJSON`, `SetRichMessage`, send. An RPC failure here is
   surfaced to the caller — an explicit `--rich` request does **not** silently
   fall back.
2. **default (non-plain)**: attempt `SetRichMessage(TryRichMarkdown(body))`;
   if the RPC fails (user-layer may reject rich), **retry once** with the plain
   entities path (`SetEntities`) — transparent, no error surfaced.
3. **`--plain`**: today's entities-free/entities path, no rich.

## Open item (Task 13)

The user-account rich behavior can only be confirmed against a live server.
Task 13 runs the first live probe and updates this note with the observed
outcome (does the user layer accept `rich_message`, or is the entities fallback
always taken?).
