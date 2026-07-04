# `bashy verify` — formal test batteries as subcommands (design + decisions)

**Status:** implemented (first cut). **Date:** 2026-07-03.
**Code:** `internal/agentos/verify.go` (+ dispatch in `agentos.go`).

## What & why

Codify bashy's formal test batteries — previously scattered across Makefile
targets and `scripts/` — as one discoverable, consistent verb surface, with the
setup automated. `codify` in both senses: **formalize** the batteries **and** turn
them into **code** (subcommands).

## The naming decision (the "right word")

The four batteries are a **precision ladder, not synonyms**. Collapsing them under
one word would overstate a claim — the exact discipline this project already keeps
(`86/86 own-suite` ≠ "100% POSIX"). So: a **neutral umbrella verb** + **precise
subcommand words**.

| subcommand | precise term | the claim it earns | strength |
|---|---|---|---|
| `verify compat` | **compatibility** | behaves like GNU Bash 5.3 (reference impl) | matches an implementation |
| `verify conformance` | **conformance** | passes 96% of the yash POSIX suite — *measured* | meets a standard, self-measured |
| `verify compliance` | **certification** | *certified* POSIX (Open Group) | authority-granted, licensed |
| `verify benchmark` | **benchmark** | faster / fewer tool-calls for agents | not a correctness claim |

Umbrella = **`verify`** (neutral; doesn't itself assert "certified"/"standards"/
"like bash"). Rejected: one flat word (overclaims), `certify`/`comply` (only the
4th earns those), `test` (collides with the `test` builtin + `go test`), `check`
(taken by script preflight). Alternatives left open: `bashy conformance {…}` +
`bashy benchmark` (split), or `bashy qualify`. Renaming the verb is a trivial
find-replace — the names live in one registry.

## The licensing decision (embedded runtime-download URL?)

**Yes for public suites; no for licensed ones.** Codified as data in
`suiteRegistry` (`suiteLicense`):

- **`licensePublicFetch`** — GPL/public suites (Bash 5.3 tests, yash). Fetched at
  runtime via an **embedded URL** into a **gitignored cache**, never vendored.
  Fetching for local use is **not redistribution**, so no copyleft propagates to
  bashy; the *harness* stays permissive. This is already the repo's posture (the
  yash script `git clone`s; the bash-5.3 fixtures are a gitignored symlink).
  - `verify compat` → `ensureBash53Fixtures` streams `ftp.gnu.org/gnu/bash/bash-5.3.tar.gz`
    and extracts **only `bash-5.3/tests/`** into `<UserCacheDir>/bashy/conformance/`,
    symlinked in. Idempotent; a no-op when present.
- **`licenseUserSupplied`** — the official POSIX suite (**Open Group VSC-PCTS**) is
  **licensed, not OSS**; its agreement restricts who may download and forbids
  redistribution. So it is **never auto-fetched**. `verify compliance` is a **stub**
  that documents how to obtain a license and accepts a **user-supplied local path**
  (`--suite PATH`, BYO). The runner lands once a licensed suite exists to test.
- **Rule (codified):** *harness code = permissive (shipped); test suites =
  fetched-at-runtime if public, user-supplied if licensed; never vendored.*

## Surface

```
bashy verify --list                 # the four suites + kind + status + license posture
bashy verify compat        [args→make test-bash-parallel]     # 86/86 gate; auto-fetch fixtures
bashy verify conformance   [args→scripts/yash-posix-suite.sh] # yash -p panel; auto-clones yash  (alias: yash)
bashy verify compliance    [--suite PATH]                     # Open Group VSC-PCTS — STUB       (alias: posix)
bashy verify benchmark     [args→eval/agent-shell harness]    # agentic bashy-vs-bash × agents
```

Runs from a **bashy source checkout** (needs Makefile + `scripts/`) — the natural
home for conformance tooling; errors helpfully otherwise. Subcommands
orchestrate the existing, proven harnesses rather than reimplementing them, and
pass flags straight through.

## Status / phasing

- **Done:** the `verify` surface + registry + `--list`; `compat` (auto-fetch +
  gate); `conformance` (yash panel); `compliance` stub (obtain guide + BYO);
  `benchmark` orchestration. Tests pin the precision-ladder + license posture +
  the fetch no-op.
- **Next:** `compliance` runner once a VSC-PCTS license is obtained (per
  `project_posix_cert_vsc_pcts_application`); a `--json` result envelope per suite;
  optionally make `compat`/`conformance` self-contained (embed the runner) so a
  released binary can self-verify without the source tree.
