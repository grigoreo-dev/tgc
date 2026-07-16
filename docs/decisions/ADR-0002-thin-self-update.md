# ADR-0002: Own thin self-update code instead of a third-party library

**Date:** 2026-07-16
**Status:** Accepted
**Related:** `.internal/specs/2026-07-16-install-self-update-design.md`, beads tgc-izb / tgc-yhu

## Context

tgc needs a `tgc self update` command plus a background startup version-check.
The obvious options for the update mechanics are (a) adopt a ready-made Go
self-update library (`minio/selfupdate`, `creativeprojects/go-selfupdate`), or
(b) write a thin implementation over the standard library.

Two constraints shape the choice:

1. **Strict output contract.** tgc guarantees stdout = results only (compact
   JSONL / `--pretty`) and stderr = structured `{"error":...}` JSON. Third-party
   updaters print progress/logs on their own terms (often to stdout, with ANSI or
   `log`), which would have to be wrapped and silenced to preserve the contract.
2. **Targets are darwin/linux, POSIX-only.** The genuinely hard problem those
   libraries solve — safely, atomically replacing a *running* binary on Windows
   (where you cannot overwrite a running exe) with rollback — is not needed here.
   On darwin/linux, `rename` over a running binary works natively.

The remaining mechanics are small: `GET releases/latest` → select asset by
os/arch → download tar.gz + checksums.txt → sha256 verify → atomic rename. That
is ~150–200 lines of `net/http` + `crypto/sha256` + `archive/tar`, all stdlib;
the only added dependency is `golang.org/x/mod/semver` for version comparison.

## Decision

Implement self-update as an in-repo `internal/selfupdate` package built on the
standard library, rather than adopting a third-party self-update library.

## Rationale

- The library's highest-value feature (Windows running-exe replacement) is out of
  scope, so most of what we'd import is unused.
- A thin implementation keeps full control over the JSONL/stderr output contract
  with no wrapping/silencing gymnastics.
- Fewer transitive dependencies in a security-sensitive CLI (it authenticates into
  a user's Telegram account), reducing the audit surface.
- The remaining code is small, well-bounded, and directly testable with
  `httptest.Server`.

This was interrogated in a stress-test (tgc-yhu); the trade-off was judged
favorable for this project's constraints, not as a blanket preference.

## Consequences

- We own and must maintain the download + verify + atomic-replace logic, including
  edge cases surfaced in the stress-test: `EvalSymlinks` on `os.Executable`, temp
  file created in the *target* directory to avoid `EXDEV`, detached-process stdio
  isolation + `Setsid`, and GitHub rate-limit/404 handling.
- Integrity is verified (sha256) but not authenticity (publisher signature) in
  v0.1.0. Real supply-chain protection (cosign/sigstore keyless signing) is a
  documented, deferred follow-up — not covered by this thin-code decision.
- If Windows support is ever added, this decision must be revisited: the
  running-exe replacement problem would then be in scope and a library may win.
