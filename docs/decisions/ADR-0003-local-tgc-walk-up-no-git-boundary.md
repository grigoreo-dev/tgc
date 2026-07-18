# ADR-0003: Local `./.tgc` discovery walks up to `$HOME`, not bounded by git-root

**Date:** 2026-07-18
**Status:** Accepted
**Related:** `docs/superpowers/specs/2026-07-18-local-tgc-config-design.md`, beads tgc-5ot / tgc-1ax

## Context

tgc is adding a local `./.tgc` config directory, discovered by walking up from
the current working directory (like `git`, `direnv`, `playwright`), so multiple
agents on one machine can each have their own default Telegram account. A design
question surfaced during stress-test: **where should the upward walk stop?**

The intuitive answer, borrowed from many project-scoped tools, is "stop at the
git repository root" — treat the repo boundary as the project boundary. This was
initially recommended.

But the real target layout is a shared workspace holding many independent git
subprojects, with one `.tgc` meant to serve all of them:

```
/root/workspace/           ← .tgc here (shared default account)
   ├─ projectA/  ← .git
   ├─ projectB/  ← .git
   └─ projectC/  ← .git
```

A git-root boundary breaks this: from `projectA/`, the walk would stop at
`projectA/.git` and never see `workspace/.tgc` above it.

## Decision

Walk-up ascends from CWD to `$HOME` (inclusive) — or FS root when CWD is outside
`$HOME` or `$HOME` is unset — and is **NOT** bounded by git-root. It must be able
to climb past a subproject's `.git` to find a shared parent `workspace/.tgc`. The
**nearest** `.tgc` wins, so a `projectA/.tgc` still overrides a `workspace/.tgc`
above it.

## Rationale

- The shared-`workspace/.tgc` covering many sibling git subprojects is the
  primary real-world use case; a git boundary would defeat it.
- "Nearest wins" preserves per-subproject override when genuinely wanted.
- `$HOME`/FS-root is a predictable, well-understood ceiling; it never climbs past
  the user's home.
- Consistency with the mental model of "one shared config for a tree of projects"
  is more valuable here than the git-scoped model.

## Consequences

- Walk-up can cross git repository boundaries — intentional, and must be covered
  by a test (climb-past-`.git` to `workspace/.tgc`).
- Because a shared `workspace/.tgc` may be written by concurrent agents,
  `config.Save` is made atomic (write-temp + `rename`) to avoid torn files.
- No cross-root session fallback: if the selected `.tgc` profile is not logged
  in, tgc returns `not_authenticated` (naming the local context) rather than
  silently using a global session — predictability over convenience.
- If a future need arises for a strict per-repo boundary, this decision must be
  revisited (e.g. an opt-in `stop_at_git` config).
