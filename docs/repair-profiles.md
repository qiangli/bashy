# Repair test profiles (S549.R4a, R4b)

Sprint #549, Stories #3070 (R4a) and #3074 (R4b). For each core component,
ONE command runs its full repair profile natively on the host it runs on —
Linux, macOS or Windows, like the release gate — and writes the evidence
record the repair gate (S549.R3a `src pr`) consumes. Orchestration is the
[`ci/repair-profiles.dag.md`](../ci/repair-profiles.dag.md) recipe (a `bashy
dag` task file, not ad-hoc shell); the runner is `tools/repairprofile` over
`internal/agentos/repairprofile.go`.

## Every suite runs inside the bashy shell (R4b)

The operator rule this implements: *"bashy is all you need"* — repair should
always assume the Bash#/bashy runtime, and where a tool is missing, add a
simple Bash# wrapper over another bashy command (make over `bashy make`,
cc over `bashy zig cc`) instead of installing anything on the host.

- The runner executes each suite as `<bashy> -c '<suite command>'`, where
  `<bashy>` is the running bashy: the current executable when the runner is
  built into bashy, else `bashy` resolved on PATH, else the runner's
  `-bashy` flag (the dag recipe pins the bashy it runs as). Inside that
  shell `make` is bashy's in-process coreutils make, and `go`, `cc`, `c++`
  (with their `gcc`/`clang`/`g++`/`clang++` spellings) resolve through the
  **Bash# wrapper units** in [`ci/bin/`](../ci/bin) — small markdown units
  the runner materializes onto each suite's private PATH as
  `#!<bashy> --bashsharp` executables (plus a `.cmd` twin on Windows, where
  cgo's CreateProcess cannot run a shebang script; cmd.exe ships with the
  OS). Host make/go/gcc are never exec'd, and a host that has none of them
  still runs the profile.
- `CC`/`CXX` default to the `cc`/`cxx` wrapper units (→ `bashy zig cc` /
  `bashy zig c++`) whenever the suite environment does not provide a
  compiler, so cgo works on a bare host.
- The same private PATH carries **bashy's own POSIX utilities** (S549.R4b
  rework): every name from the data behind `bashy commands --view posix`
  (the 116 POSIX-required utilities plus the GNU coreutils inventory) that
  the bashy binary dispatches on argv[0] — `sh`/`sleep`/`ls`/`head`/… — is
  linked into the suite bin dir pointing at the running bashy. An external
  shell a suite spawns (`/bin/sh -c "sleep 30"`, the live finding on a
  bashy-only host) resolves its utilities on PATH and finds bashy's own,
  never a host binary. Unix links are symlinks; Windows prefers same-volume
  hard links with the `.exe` suffix (argv[0] still names the utility) and
  falls back to a small `.cmd` shim naming the utility when a hard link
  cannot be made — never a copy of the ~250 MB binary. Wrapper-owned names
  (`go`/`cc`/`c++` and their spellings) are never shadowed, and names bashy
  cannot dispatch are never linked (a broken link would shadow a working
  host binary). The bin dir stays **first** on the suite PATH with the host
  PATH as a trailing fallback (the existing R4b semantics). The evidence
  records the provenance in `runtime.posix_links` (count, bin dir, link
  kind).
- If a suite needs a tool the bashy runtime cannot provide, the evidence
  records that suite `skipped (tool <x> unavailable in bashy runtime)` —
  never pass — and names the missing tool.
- The evidence records **which bashy ran** (`runtime.bashy`: path + version)
  and **which provider each tool resolved to** (`runtime.tools`).
- Commit identity for a source tree with no `.git` (a `git archive`):
  `BASHY_REPAIR_COMMIT=<sha>` or the runner's `-commit <sha>` flag (the dag
  recipe forwards `COMMIT=`).

## The entry command

```sh
# in the bashy checkout (fresh: ./bashy dag …; installed: bashy dag …)
bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence.json
bashy dag ci/repair-profiles.dag.md profile COMPONENT=outpost OUTPOST_ROOT=../outpost
```

`dag` has no `run <target> -- --args` subcommand: the idiomatic argument
mechanism is make-style `KEY=VALUE` var overrides after the target (see
`docs/dag.md`), so `--component`/`--out` become `COMPONENT=`/`OUT=` vars the
`profile` body forwards to the runner. Exit 0 only when the evidence record
says complete **and** pass. The per-OS lane targets (`bashy-linux`,
`bashy-darwin`, `bashy-windows`, `outpost-*`, `bashy-uutils-linux-oci`) are
individually runnable lanes for a specific host; list them with
`bashy dag ci/repair-profiles.dag.md --list`, plan without running with
`--dry-run`, and ask the runner itself for this host's plan with
`go run ./tools/repairprofile -list -component bashy`.

## Profiles (MVP: two components)

| Component | Suite | Lane | Command |
|---|---|---|---|
| bashy | `make-test` | native (every OS) | `make test` |
| bashy | `test-bash` | native (every OS) | `make test-bash` (86/86; SHA-pinned GPL fixture cache) |
| bashy | `test-uutils` | **oci-linux only** | `make test-uutils` (the contained runner; never native) |
| outpost | `go-test-short-headless` | native (every OS) | `go test -short -tags fb_archives` over `go list ./...` minus `internal/agent/shell` (needs a real TTY) — the Makefile `test-headless` lane, cross-platform and make-free |

VSC/PCTS is excluded (licensed; a different lane, never auto-fetched).

## Evidence schema (`bashy-repair-profile-v1`)

Written once, atomically (temp file + rename), after the last suite finishes —
a runner killed mid-profile leaves no new record, which the gate reads as
failure.

```json
{
  "schema": "bashy-repair-profile-v1",
  "component": "bashy",
  "commit": "ea1baa7",
  "os": "darwin",
  "arch": "arm64",
  "runtime": {
    "bashy": {
      "path": "/Users/qa/.local/bin/bashy",
      "version": "bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-dev (b75e1bf)"
    },
    "tools": [
      { "tool": "make", "provider": "bashy make (in-process coreutils make)", "via": "bashy shell ExecHandler, ahead of PATH" },
      { "tool": "go", "provider": "bashy go (pinned provider go1.27.2)", "via": "wrapper unit ci/bin/go.md on the suite PATH" },
      { "tool": "cc", "provider": "bashy zig cc (pinned zig provider; zig cc is clang)", "via": "wrapper unit ci/bin/cc.md on the suite PATH (CC default)" },
      { "tool": "c++", "provider": "bashy zig c++ (pinned zig provider)", "via": "wrapper unit ci/bin/cxx.md on the suite PATH (CXX default)" }
    ],
    "posix_links": { "count": 150, "bin_dir": "/tmp/bashy-repair-bin-123", "kind": "symlink" }
  },
  "start": "2026-10-31T22:19:07Z",
  "end": "2026-10-31T22:31:44Z",
  "suites": [
    {
      "name": "make-test",
      "lane": "native",
      "command": "make test",
      "exit_code": 0,
      "duration_ms": 416203,
      "counts": { "pass": 412, "fail": 0, "skip": 2 },
      "output_tail": "…last 16 KiB of the combined output…",
      "complete": true,
      "pass": true
    },
    {
      "name": "test-bash",
      "lane": "native",
      "command": "make test-bash",
      "exit_code": 0,
      "duration_ms": 512001,
      "counts": { "pass": 86 },
      "output_tail": "Results: 86 passed, 0 failed, 0 skipped, 0 timed out\n",
      "complete": true,
      "pass": true
    },
    {
      "name": "test-uutils",
      "lane": "oci-linux",
      "command": "make test-uutils",
      "skipped": true,
      "complete": false,
      "pass": false,
      "reason": "skipped (lane unavailable: landmine suites run only in the Linux oci lane (host is darwin))"
    }
  ],
  "complete": true,
  "pass": true
}
```

Verdict rules (unit-tested with fake suites: `true`, `false`, a timeout-kill,
a signal-kill, truncated output):

- **`complete`** is true only if every ELIGIBLE suite ran to completion —
  exited on its own (the bashy shell carrying the suite included), not
  killed by the per-suite wall bound (`timeout: killed after …`) or by a
  signal (`killed by signal …`), and actually started. Inside the bashy
  shell a command that resolves nowhere exits 127 — a completed, FAILED
  suite, never a pass.
- **`pass`** is true only if complete and every eligible suite exited 0.
  A killed or partial run is NEVER a pass.
- **Skipped** suites (structurally unavailable lane, or a tool the bashy
  runtime cannot provide) are recorded with `skipped: true` and the spec
  wording `skipped (lane unavailable: …)` / `skipped (tool <x> unavailable
  in bashy runtime)` — never counted as pass, never failing the native
  lanes that did run.
- **A profile where nothing ran fails closed** (`"no suite ran on this
  host"`): measuring nothing is not passing.
- `output_tail` is bounded (default 16 KiB); `output_truncated: true` marks
  that the full output exceeded the tail. `counts` appear only when parseable
  (go-test `--- PASS/FAIL/SKIP` lines; the bash53suite `Results:` line, where
  timed-out fixtures count as failures).

## Offline cache (fetch by SHA, never vendor)

The bash-5.3 GPL fixtures reuse the exact cache `make test-bash` owns:
fetched by SHA-256 from the pinned tarball into
`<user cache>/bashy/conformance/bash-5.3` with a
`.bashy-source-sha256` marker, gitignored, never vendored
(`EnsureBash53Fixtures`). The profile runner is offline-aware:

- **`BASHY_OFFLINE=1`** — no network attempt, ever. A valid cache (marker
  matches the pin) satisfies the run; an **empty cache fails clearly**:
  `bash-5.3 fixture cache is empty at … and BASHY_OFFLINE=1: populate it
  once online (make test-bash-fixtures) — refusing to fetch`, the profile
  fails closed before any suite runs.
- Online (default) the cache is filled/verified by SHA on first use, then
  every later run is a cache hit.

The uutils landmine lane additionally needs the sibling clone at
`../coreutils/reference/uutils-coreutils` and an OCI runtime (`docker` or
`bashy podman`); when either is missing the lane is structurally unavailable
→ skipped entry, never pass.

## Exact commands per host

Run from the bashy checkout — the host needs **only bashy** (R4b): the
recipe drives the runner through the bashy it runs as (`bashy go run`),
and every suite then runs inside that bashy's shell where make/go/cc are
bashy's own. No make, go or gcc on the host is required.

```sh
# Linux test host (native + the oci landmine lane when available)
bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence-linux.json

# macOS test host (native; uutils recorded skipped — Linux-oci-only)
bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence-macos.json

# Windows test host (native; make/go/cc resolve inside the bashy shell —
# the host needs no make/go/gcc of its own)
bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence-windows.json

# Any OS — the outpost profile (OUTPOST_ROOT names the outpost checkout)
bashy dag ci/repair-profiles.dag.md profile COMPONENT=outpost OUTPOST_ROOT=../outpost

# A source tree with no .git (git archive): record the commit identity
BASHY_REPAIR_COMMIT=<sha> bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy
#   (equivalently: … profile COMPONENT=bashy COMMIT=<sha>)

# Offline proof (works from a warm cache; fails clearly when the cache is empty)
BASHY_OFFLINE=1 bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence-offline.json
```

The manager runs the real full suites on the test hosts; on a dev box use the
short lanes only (`go test ./internal/agentos -run TestRepair`, the runner's
`-list`).
