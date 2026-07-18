# Design: Local `./.tgc` config discovery (walk-up) for multi-agent profiles

**Date:** 2026-07-18
**Status:** Approved (brainstorming) — pending implementation plan
**Brainstorming bead:** tgc-5ot

## Problem

Multiple agents can run on one machine, each needing a different default
Telegram account. Today tgc resolves config to a single global directory
(`~/.config/tgc`), so "which account is default" is machine-global. Agents
working in different project directories cannot each have their own default
account without juggling `TGC_CONFIG_DIR`/`--profile` on every call.

## Goal

Let tgc prefer a **local `./.tgc` directory** (discovered by walking up from the
current working directory, like `git`, `terraform`, `direnv`, `playwright`),
falling back to the global config when none exists. An agent operating inside
its own project directory automatically gets its own account.

## Non-goals (YAGNI)

- Merging local + global config (a single config root is chosen; no overlay).
- Auto-creating `./.tgc` on arbitrary commands (creation is explicit via `tgc init`).
- Embedding default api_id/api_hash in the binary (ban-risk; see Decisions).
- Auto-migrating existing global profiles into a local dir (do it manually:
  `tgc init` then `tgc auth login`).

## Key decisions

1. **Priority:** `TGC_CONFIG_DIR` (env) > walk-up `./.tgc` > `$XDG_CONFIG_HOME/tgc` > `~/.config/tgc`.
   Env stays highest so existing scripts/CI are unaffected.
2. **Walk-up boundary:** ascend from CWD up to and including `$HOME` (or FS root,
   whichever is reached first — CWD may live outside `$HOME`, e.g. `/srv`, docker,
   `/root/workspace`), then stop. NOT bounded by git-root: a single
   `workspace/.tgc` is intended to cover many sibling git subprojects, so the
   walk-up must be able to climb past a subproject's `.git`. The **nearest**
   `.tgc` wins, so a `projectA/.tgc` still overrides a `workspace/.tgc` above it.
3. **Walk-up is read-only.** It never creates `./.tgc`. Only `tgc init` does.
4. **Explicit `tgc init`** (matches git/terraform/npm/playwright industry consensus):
   idempotent, additive, never clobbers an existing `config.toml`.
5. **Session secret safety:** `tgc init` writes `./.tgc/.gitignore` containing `*`
   so account sessions (secrets) can't be accidentally committed.
6. **api_id/api_hash:** required (cannot be removed — `NewClient` + `initConnection`
   send them). NOT embedded as a hidden default (shared public ids get flagged →
   ban risk; ToS: an official-API session should stay on official API). Instead
   creds live alongside the session in the active config root. `tgc init` inherits
   them from the global config if present; sessions are never copied.

### Why api_id/hash can't simply be dropped when a session exists

gotd restores the persisted **auth_key** from the session, and that key already
authenticates every RPC (`AUTH_KEY_UNREGISTERED` means "no key", not "wrong
id/hash"). The server does not re-validate api_id/hash on reconnect (confirmed by
Telethon maintainer, issue #1569). BUT: `telegram.NewClient(apiID, apiHash, …)`
requires non-zero values and `initConnection` transmits them on every connect;
and swapping api_id for an existing session violates the ToS heuristic that an
official-API session stays on official API (ban risk). So we keep creds required
and pinned to the session, without a magic embedded default.

## Design

### 1. Config-directory resolution (`internal/config/config.go`)

The single change point is `config.Dir()`. New order:

```
1. TGC_CONFIG_DIR        (env override)
2. walk-up ./.tgc        (CWD → $HOME inclusive; first existing .tgc dir; READ ONLY)
3. $XDG_CONFIG_HOME/tgc
4. ~/.config/tgc
```

New helper `findLocalDir() string`:
- start at `os.Getwd()`
- ascend via `filepath.Dir` until reaching `$HOME` (inclusive) or FS root
- at each level, if `<dir>/.tgc` exists and is a directory, return it (nearest wins)
- NOT bounded by git-root — must climb past a subproject `.git` to find a shared
  `workspace/.tgc`
- otherwise return `""` (Dir() falls through to global)

Everything built on `Dir()` (`profiles/`, `session*`, `APICredentials`,
`configPath`) is unchanged — it just receives a different root.

**Resilience requirements (findLocalDir):**
- A `stat`/permission error at any level is skipped (`continue` upward), never fatal.
- If `$HOME` is unset (docker/CI), the boundary is FS root only.
- A `.tgc` that is a *file* (not a directory) is ignored (skip, keep climbing).
- No `filepath.EvalSymlinks` — use `os.Getwd()`'s already-cleaned path (YAGNI;
  symlink resolution invites edge bugs).

**Self-heal:** when a resolved local `./.tgc` is used and `./.tgc/.gitignore`
is missing, tgc silently (re)writes it as `*` — no stderr noise (keeps the JSONL
contract clean). Catches a `.tgc` created outside `tgc init`.

### 2. `tgc init` command (`internal/cli/init.go`)

```
tgc init [--profile <name>]
```

1. Create `./.tgc/` in CWD (0700) if missing.
2. Write `./.tgc/.gitignore` = `*`.
3. Create/augment `./.tgc/config.toml` (0600):
   - `default_profile` = `<name>` or `"default"`
   - `api_id`/`api_hash` inherited from, in order: env `TGC_API_ID`/`TGC_API_HASH`,
     then the **global** config (`$XDG_CONFIG_HOME/tgc` → `~/.config/tgc`). NOT
     from a local walk-up (would be self-referential) and NOT from
     `TGC_CONFIG_DIR` (a transient session override). Else empty.
4. Sessions are NOT copied; the agent runs `tgc auth login` into the local profile.
5. Output (JSONL contract): `{"path":"…/.tgc","inherited_creds":true|false}`.
   When `inherited_creds=false`, also emit a `"next"` hint: set
   `TGC_API_ID`/`TGC_API_HASH` or edit `.tgc/config.toml`, then `tgc auth login`.
   `tgc init` never fails for missing creds — it builds structure; real use is
   still guarded by `APICredentials`.

**Idempotent = additive-only** (like `npm init`): existing non-empty keys in the
local `config.toml` are never touched; only empty/absent keys are filled, and the
directory/`.gitignore` are created if missing. A second `tgc init` never rolls
back manual edits. Writes directly to `./.tgc` in CWD — it does NOT use walk-up
(like `git init`).

**Atomic writes:** `config.Save` is changed from `O_TRUNC` to write-temp +
`rename` (atomic) so a shared `workspace/.tgc/config.toml` can't be torn by two
agents writing concurrently. This also hardens the existing (non-local) code
path. Full file locks are YAGNI (init is rare; sessions use SQLite's own
locking).

### 3. Observability (`internal/cli/config.go`)

New `tgc config path`:

```json
{"config_dir":"/proj/.tgc","source":"local","profile":"default"}
```

`source` ∈ `env` | `local` | `global`. No hidden merge: exactly one config
root is selected (first by priority); the local `./.tgc` is a self-contained
root (its own `profiles/`, its own `config.toml`), not an overlay.

**Shadowed-local hint:** when `source=env` but a local `./.tgc` exists in the
walk-up range (env is shadowing it), `config path` adds
`"shadowed_local":"/proj/.tgc"`. This removes the main source of confusion
("why doesn't my `tgc init` take effect?") when an agent runner exports
`TGC_CONFIG_DIR` globally.

**No auto-fallback across roots.** If the selected local `./.tgc` profile has no
session, tgc returns `not_authenticated` (it does NOT silently fall back to a
global session — that would make "which account?" unpredictable). The error
message names the local context: "profile X in local ./.tgc has no session; run
`tgc auth login`". (Enriches the existing `client.go` not_authenticated error.)

## Backwards compatibility

- No `./.tgc` anywhere in CWD..$HOME → behavior identical to today (global).
- `TGC_CONFIG_DIR` remains highest priority → existing scripts/CI unaffected.
- No change to config.toml format, profiles, or session storage.

## Testing

`internal/config/config_test.go` (and a new `internal/cli` test):

1. `findLocalDir`: finds `.tgc` in CWD; finds in a parent (nearest wins); climbs
   PAST a subproject `.git` to a `workspace/.tgc`; stops at `$HOME`; `$HOME` unset
   → FS root; a `.tgc` *file* is ignored; a permission error mid-climb is skipped
   not fatal; returns "" when absent. Temp trees with `$HOME`/CWD substitution.
2. `Dir()` priority table: env > local > XDG > home.
3. `tgc init`: creates dir + `.gitignore`(`*`) + config.toml; additive-only
   idempotency (re-run does NOT overwrite existing non-empty keys); inherits creds
   from env→global; does not copy sessions; never fails on missing creds; emits
   `next` hint when `inherited_creds=false`.
4. `tgc config path`: correct `source` for env/local/global; `shadowed_local`
   present when env shadows an existing local `.tgc`.
5. `config.Save`: atomic (temp+rename) — a concurrent writer never observes a
   torn file.

## Security

- `.tgc/.gitignore` = `*` prevents session (secret) leakage into git, plus
  self-heal re-creates it if a `.tgc` was made outside `tgc init`.
- `0700` on `.tgc/`, `0600` on files (consistent with current `Save`/`MkdirAll`).
- No embedded api creds (ban risk).

## Stress Test Results: local ./.tgc config design

Bead: tgc-1ax. 9 branches interrogated (8 modified, 1 agreed).

### Resolved Decisions
- **Walk-up boundary:** `$HOME` or FS-root, whichever first. **git-root REJECTED**
  as a boundary — a shared `workspace/.tgc` must cover many sibling git
  subprojects, so walk-up must climb past a subproject `.git`. Nearest `.tgc` wins.
- **Security:** `.gitignore`=`*` plus silent self-heal when a local `.tgc` lacks it.
- **init idempotency:** additive-only — never overwrite existing non-empty keys.
- **creds inheritance:** env `TGC_API_ID/HASH` → global config; not local, not
  `TGC_CONFIG_DIR`.
- **resolution priority:** env > local unchanged; `config path` surfaces
  `shadowed_local` when env shadows a real local `.tgc`.
- **FS-edge resilience:** stat/perm errors skip upward (non-fatal); `$HOME` unset
  → FS-root; `.tgc`-as-file ignored; no `EvalSymlinks`.
- **no-merge:** unauthenticated local profile → `not_authenticated` (no
  cross-root session fallback), error names the local context.
- **missing-creds:** `tgc init` never fails without creds; emits a `next` hint.
- **concurrency (reflexion-added):** `config.Save` becomes atomic (temp+rename);
  full file locks are YAGNI.

### Changes Made
- Walk-up: removed git-root boundary; documented nearest-wins + climb-past-`.git`.
- Added: self-heal `.gitignore`, additive-only init, creds source order,
  `shadowed_local` hint, no-merge error enrichment, `next` hint, atomic
  `config.Save`, findLocalDir resilience rules.
- Testing section expanded to cover all of the above.

### Deferred / Parking Lot
- Full file locks / advisory locking (YAGNI).
- Symlink resolution via `EvalSymlinks` (YAGNI).
- Auto-migration of global profiles into local (manual: `tgc init` + `auth login`).

### Confidence Assessment
- Overall: High.
- Areas of concern: none blocking. The shared-`workspace/.tgc` + concurrent-agent
  scenario is the least-exercised path; atomic writes + tests mitigate it.
