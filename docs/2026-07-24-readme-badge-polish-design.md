# README Badge Polish — Design

**Date:** 2026-07-24
**Bead:** tgc-y0qu (brainstorming session)
**Status:** Approved

## Problem

The README badge area had three cosmetic issues:
1. A static `golangci-lint` badge sitting next to the live CI badge (both in
   the same `ci.yml` workflow, so CI already covers lint) — a dead plaque
   beside a live status.
2. Three near-but-different cyans in one row (Go `00ADD8`, lint `00ADD8`,
   Agent-first `22d3ee`) — visual noise.
3. A mix of `for-the-badge` and `flat-square` styles across two rows — abrupt
   transition.
4. Bright neon `39ff14` used as a fill behind white text (Release/JSONL/
   agent-first) — unreadable contrast.

## Decision (visual companion, direction A + contrast A1)

**Single unified `flat-square` style, one centered row.** Drop the static
golangci-lint badge. Palette:

- **Muted GitHub-green `238636`** for status/identity greens (release, CI,
  output=JSONL, agent-first) — white text reads well on it; it's the "healthy"
  status green. Replaces neon `39ff14` as a text background.
- **Cyan** kept only for ecosystem badges: Go `00ADD8`, MTProto `229ED9`.
- **Grey `8b949e`** for license (meta).
- Dark `labelColor=0d1117` retained everywhere.

Neon `39ff14` is no longer used as a fill behind text (the contrast failure).

## Final badge row (both README.md and README.ru.md)

```html
<p align="center">
  <a href="https://github.com/grigoreo-dev/tgc/releases"><img alt="Release" src="https://img.shields.io/github/v/release/grigoreo-dev/tgc?style=flat-square&logo=github&label=release&labelColor=0d1117&color=238636"></a>
  <a href="https://github.com/grigoreo-dev/tgc/actions"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/grigoreo-dev/tgc/ci.yml?style=flat-square&logo=githubactions&logoColor=white&label=CI&labelColor=0d1117&color=238636"></a>
  <a href="https://go.dev"><img alt="Go" src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white&labelColor=0d1117"></a>
  <a href="https://core.telegram.org/mtproto"><img alt="MTProto" src="https://img.shields.io/badge/MTProto-229ED9?style=flat-square&logo=telegram&logoColor=white&labelColor=0d1117"></a>
  <img alt="Output" src="https://img.shields.io/badge/output-JSONL-2ea043?style=flat-square&logo=json&logoColor=white&labelColor=0d1117">
  <img alt="Agent-first" src="https://img.shields.io/badge/agent--first-%E2%9C%93-2ea043?style=flat-square&labelColor=0d1117">
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/grigoreo-dev/tgc?style=flat-square&label=license&labelColor=0d1117&color=8b949e"></a>
</p>
```

Replaces the previous two `<p>` badge blocks (the `for-the-badge` row and the
`flat-square` row) with this single `<p align="center">`.

## Scope

- Edit the badge blocks in `README.md` and `README.ru.md` only. No other
  content, links, or the banner image change.
- The existing `[Русская версия] · [Install] · ...` nav line stays.

## Verification

- Both files render the single row on GitHub (no broken image URLs, valid
  HTML, `align="center"` preserved).
- No leftover `for-the-badge` in the badge area; no `golangci-lint` badge; no
  `39ff14` used as a text fill.

## Out of scope

- Banner image, logos, section content.
- Any CI/workflow change (lint is already a CI job).
