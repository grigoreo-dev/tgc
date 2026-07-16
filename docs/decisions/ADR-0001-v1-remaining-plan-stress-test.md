# ADR-0001: tgc v1 remaining plan (Tasks 8–13) stress-test decisions

## Context

Implementation plan `docs/superpowers/plans/2026-07-14-tgc-v1-implementation.md` covers
Tasks 1–13. Tasks 1–7 are on `main`. Before executing Tasks 8–13, an adversarial
stress-test interrogated remaining design branches (RichMessage, gotd pin,
read pagination, bot limits, media defaults, download paths, live integration).

## Decision

1. **RichMessage:** Use gotd `InputRichMessageMarkdown` (and HTML / expert
   `--rich` JSON). No custom PageBlock AST in v1. Fall back to entities on
   server reject.
2. **gotd version:** Keep current pin (`v0.153.0`, Layer 227). Optional
   gotd-only bump before Task 8 gated by `go build` + `go test`; no forced
   gotgproto upgrade.
3. **read pagination:** `--after` → `MinID`, `--before` → `MaxID` (+
   `OffsetID` when used as cursor). One RPC = one page. No `AddOffset` hack.
4. **Session security:** Export/import keep writing `0600`; never log session
   strings; README warns sessions equal account credentials; no
   encryption-at-rest in v1.
5. **bot_unsupported:** Preflight deny-list for bot profiles (`chats`, `search`
   without `--messages`, phone resolve, fuzzy-name resolve) plus reactive
   `BOT_METHOD_INVALID` safety net (not broad `*_INVALID`).
6. **Task order:** Serial 8 → 9 → 10 → 11 → 12 → 13.
7. **Task 13 live:** Required to close v1; credentials from `.env` +
   interactive user-bot login; never commit or log secrets.
8. **Media send:** Images default to photo; `--as-document` forces document.
9. **Download path:** `TGC_DOWNLOAD_DIR` or
   `~/.tgc/downloads/<file_id>/<original_filename>`; `-o` override;
   `uniquePath` on conflict; `--stdout` raw bytes.

## Rationale

- gotd already exposes RichMessage constructors at Layer 227; a local
  PageBlock tree would reimplement server Markdown parsing for no gain.
- Native `MinID`/`MaxID` match MTProto offsets docs and avoid fragile
  `AddOffset` edge gaps.
- Bot method availability varies by peer; a narrow preflight list prevents
  pointless dialog/contact RPCs while still allowing bots to message chats
  they belong to.
- Agent workflows need headless session portability more than
  encryption-at-rest in a trusted operator environment.
- Photo-default matches user/agent expectation for images; document path is
  the explicit exception.
- Download under `~/.tgc/` keeps agent CWD clean and groups files by Telegram
  file id for cache-friendly paths.

## Consequences

- Plan Global Constraints and Tasks 8–11, 13 checklists updated accordingly.
- Task 11 scope shrinks (helpers + send integration, not full AST).
- Live RichMessage acceptance for user accounts remains a Task 13 probe risk;
  entities fallback keeps send usable either way.
- CLI flag surface: `--as-document` (not `--as-photo`); download help text
  points at `~/.tgc/downloads/...`.
