# ADR-0004: Update-check cache lives in GlobalDir, not config.Dir()

**Date:** 2026-07-18
**Status:** Accepted
**Related:** beads tgc-1zm (regression of ADR-0003 / local `./.tgc` in v0.1.1)

## Context

Since v0.1.1, `config.Dir()` can resolve to a local `./.tgc` via walk-up from
CWD (ADR-0003). The self-update startup path stored its 24h dedup cache at
`config.Dir()/update-check.json`. That made the cache **per config root**:

- each project with its own `.tgc` → separate cache file
- each new root is a cold miss → `StartupNotify` spawns a fresh detached
  `tgc self check` → another `api.github.com` call
- unauthenticated GitHub limit is 60/hr; fragmented cache burns it far faster
  than the user's real manual `self check/update` volume

Symptom in the wild: macOS user hit `rate_limited` without ~60 manual calls.

The cache is machine-global state (one binary, one "have we checked today?"
answer), not per-project Telegram credentials.

## Decision

`internal/selfupdate` cache path uses `config.GlobalDir()`
(`$XDG_CONFIG_HOME/tgc` or `~/.config/tgc`), **not** `config.Dir()`.

- Ignores local `./.tgc` walk-up.
- Ignores `TGC_CONFIG_DIR` (same as `GlobalCredentials`).
- `GlobalDir()` is the shared helper; `GlobalCredentials` was refactored to
  call it instead of inlining the path.

## Rationale

- Update freshness is a property of the installed binary on the machine, not of
  which project/agent is currently using which Telegram session.
- One cache file → one check per 24h per machine, matching the original
  self-update design (ADR-0002).
- Mirrors the existing "global-only" pattern already used for credential
  inheritance (`GlobalCredentials` deliberately bypasses `Dir()`).

## Consequences

- `cachePath()` / `WriteCache` always write under GlobalDir.
- Tests that exercised the cache must set `XDG_CONFIG_HOME` (or HOME), not
  `TGC_CONFIG_DIR`.
- Leftover `update-check.json` files under old local `.tgc` dirs are orphaned
  and harmless; no migration required.
- Other future machine-global state (e.g. install metadata) should prefer
  `GlobalDir()` over `Dir()` for the same reason.
