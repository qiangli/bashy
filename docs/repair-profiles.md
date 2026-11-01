# Repair test profiles (S549.R4a)

Sprint #549, Story #3070 (S549.R4a). For each core component, ONE command
runs its full repair profile natively on the host it runs on — Linux, macOS or
Windows, like the release gate — and writes the evidence record the repair
gate (S549.R3a `src pr`) consumes. Orchestration is the
[`ci/repair-profiles.dag.md`](../ci/repair-profiles.dag.md) recipe (a `bashy
dag` task file, not ad-hoc shell); the runner is `tools/repairprofile` over
`internal/agentos/repairprofile.go`.

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
  exited on its own, not killed by the per-suite wall bound (`timeout:
  killed after …`) or by a signal (`killed by signal …`), and actually
  started (`start: … not found` fails closed; through the bashy multicall
  fallback, exit 127 is a start failure, not a suite verdict).
- **`pass`** is true only if complete and every eligible suite exited 0.
  A killed or partial run is NEVER a pass.
- **Skipped** suites (structurally unavailable lane) are recorded with
  `skipped: true` and the spec wording `skipped (lane unavailable: …)` —
  never counted as pass, never failing the native lanes that did run.
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

Run from the bashy checkout (a peer needs only bashy + the checkout; the
recipe runs the runner through `bashy go`):

```sh
# Linux test host (native + the oci landmine lane when available)
bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence-linux.json

# macOS test host (native; uutils recorded skipped — Linux-oci-only)
bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence-macos.json

# Windows test host (native; make resolves through bashy's multicall when the
# host has no make on PATH)
bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence-windows.json

# Any OS — the outpost profile (OUTPOST_ROOT names the outpost checkout)
bashy dag ci/repair-profiles.dag.md profile COMPONENT=outpost OUTPOST_ROOT=../outpost

# Offline proof (works from a warm cache; fails clearly when the cache is empty)
BASHY_OFFLINE=1 bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence-offline.json
```

The manager runs the real full suites on the test hosts; on a dev box use the
short lanes only (`go test ./internal/agentos -run TestRepair`, the runner's
`-list`).
