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
2. **Walk-up boundary:** ascend from CWD up to and including `$HOME`, then stop.
   Never climb past the user's home directory.
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
- at each level, if `<dir>/.tgc` exists and is a directory, return it
- otherwise return `""` (Dir() falls through to global)

Everything built on `Dir()` (`profiles/`, `session*`, `APICredentials`,
`configPath`) is unchanged — it just receives a different root.

### 2. `tgc init` command (`internal/cli/init.go`)

```
tgc init [--profile <name>]
```

1. Create `./.tgc/` in CWD (0700) if missing.
2. Write `./.tgc/.gitignore` = `*`.
3. Create/augment `./.tgc/config.toml` (0600):
   - `default_profile` = `<name>` or `"default"`
   - `api_id`/`api_hash` inherited from global `~/.config/tgc/config.toml` if set,
     else empty.
4. Sessions are NOT copied; the agent runs `tgc auth login` into the local profile.
5. Output (JSONL contract): `{"path":"…/.tgc","inherited_creds":true|false}`.

Idempotent: a second `tgc init` does not clobber an existing `config.toml`; it
only fills in what's missing (`.gitignore`, directory). Writes directly to
`./.tgc` in CWD — it does NOT use walk-up (like `git init`).

### 3. Observability (`internal/cli/config.go`)

New `tgc config path`:

```json
{"config_dir":"/proj/.tgc","source":"local","profile":"default"}
```

`source` ∈ `env` | `local` | `global`. No hidden merge: exactly one config
root is selected (first by priority); the local `./.tgc` is a self-contained
root (its own `profiles/`, its own `config.toml`), not an overlay — same model
as git picking the nearest `.git`.

## Backwards compatibility

- No `./.tgc` anywhere in CWD..$HOME → behavior identical to today (global).
- `TGC_CONFIG_DIR` remains highest priority → existing scripts/CI unaffected.
- No change to config.toml format, profiles, or session storage.

## Testing

`internal/config/config_test.go` (and a new `internal/cli` test):

1. `findLocalDir`: finds `.tgc` in CWD; finds in a parent; stops at `$HOME`;
   returns "" when absent. Uses temp trees with `$HOME`/CWD substitution.
2. `Dir()` priority table: env > local > XDG > home.
3. `tgc init`: creates dir + `.gitignore`(`*`) + config.toml; idempotent (no
   clobber on re-run); inherits creds from global; does not copy sessions.
4. `tgc config path`: correct `source` for env/local/global.

## Security

- `.tgc/.gitignore` = `*` prevents session (secret) leakage into git.
- `0700` on `.tgc/`, `0600` on files (consistent with current `Save`/`MkdirAll`).
- No embedded api creds (ban risk).
