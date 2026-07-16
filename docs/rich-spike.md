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

## Live result (Task 13)

The open question from Task 11 — does the user layer accept `rich_message`, or
is the entities fallback always taken? — was answered by the Task 13 live probe
against a real server. The observed outcome:

- **Default block-markdown send** (non-plain, no `--rich`): the server rejects
  `rich_message` for a normal user account, so tgc's transparent entities
  fallback fires. A message combining a heading, a list, and a quote was
  delivered as entity-formatted text, with the Markdown rendered correctly and
  no error surfaced to the caller.
- **Explicit `--rich`** on a user account: fails with `RICH_MESSAGE_UNSUPPORTED`.
  tgc maps this to a structured error,
  `{"error":"rich_unsupported","message":"rich messages are not supported for this account; omit --rich or send default Markdown"}`,
  and exits 1. An explicit `--rich` request does not silently fall back — that
  is by design, so an expert who asked for the rich path learns it was refused
  rather than getting a quietly different result.

On user accounts the entities path is therefore effectively always taken. The
`InputRichMessageMarkdown` path is still retained for two reasons: bots and
other account classes may accept it, and the default send degrades to entities
transparently when it does not. No custom PageBlock AST is needed in v1.
