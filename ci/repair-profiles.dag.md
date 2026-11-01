---
name: repair-profiles
description: S549.R4a repair test profiles — one command runs a core component's (bashy, outpost) full repair profile natively on the host it runs on (Linux, macOS, Windows, like the release gate); landmine suites (uutils) only in the Linux oci lane; writes the bashy-repair-profile-v1 evidence record the repair gate consumes. Use when asked to produce repair-profile evidence or to run a component's full suite set on a test host.
type: dag
default: profile
vars:
  COMPONENT ?= bashy
  OUT ?= results/repair-profile.json
  ROOT ?= .
  OUTPOST_ROOT ?= ../outpost
---

# repair-profiles — S549.R4a

For each core component, ONE command runs its full repair profile natively on
the host it runs on — Linux, macOS or Windows, like the release gate — and
writes the evidence record (`bashy-repair-profile-v1`, schema in
`docs/repair-profiles.md`) the repair gate (S549.R3a `src pr`) consumes.

Rules of record, enforced by the runner (`tools/repairprofile`), not by trust:

- **Offline passes from the cache.** With `BASHY_OFFLINE=1` the run never
  reaches for the network; the GPL bash-5.3 fixtures come from the SHA-pinned
  user cache (`<user cache>/bashy/conformance/bash-5.3`, verified by the
  `.bashy-source-sha256` marker). An empty cache under `BASHY_OFFLINE=1`
  fails clearly — never a fetch, never a silent skip.
- **A killed, partial or truncated run is a failure, never a pass.** Every
  suite runs under a wall-time bound; a timeout or signal kill records
  `complete:false, pass:false` with the reason. The evidence file is written
  once, atomically, after the last suite — a runner killed mid-profile leaves
  no new record, which the gate reads as failure.
- **GPL suites are fetched by SHA and cached; never vendored.**
- **VSC/PCTS is excluded** (licensed; a different lane, never auto-fetched).
- **Only landmine suites (uutils) run in the Linux oci lane**, and only when
  that lane is available; otherwise the evidence records
  `skipped (lane unavailable)` — never "pass".

The per-OS targets below are the lanes a host can run individually;
`profile` is the one-command entry that runs everything valid on THIS host.

## Tasks

### profile
The ONE entry: run ${COMPONENT}'s full repair profile on THIS host (the
runner picks the native suites for this OS plus the oci landmine lane when it
is available) and write the evidence record to ${OUT}. Exit 0 only when the
record says complete and pass.

    bashy dag ci/repair-profiles.dag.md profile COMPONENT=bashy OUT=results/repair-evidence.json
    bashy dag ci/repair-profiles.dag.md profile COMPONENT=outpost

Effects: read, write, exec, net

```bash
set -e
: "${COMPONENT:?set COMPONENT=bashy|outpost}"
BASHY_EXE="${BASHY:-bashy}"
case "${COMPONENT}" in
  outpost) R="${OUTPOST_ROOT}" ;;
  *)       R="${ROOT}" ;;
esac
"$BASHY_EXE" go run ./tools/repairprofile -component "${COMPONENT}" -root "${R}" -out "${OUT}"
```

### bashy-linux
The bashy profile on a Linux host: make test + make test-bash (86/86), plus
the uutils landmine lane when the oci sandbox is available. Same runner as
`profile`, an explicit lane for per-host dispatch.
When: test "$(uname -s)" = Linux
Effects: read, write, exec, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go run ./tools/repairprofile -component bashy -root "${ROOT}" \
  -out "results/repair-profile-bashy-linux.json"
```

### bashy-darwin
The bashy profile on a macOS host: make test + make test-bash (86/86), both
native; the uutils lane reports skipped (Linux-oci-only), never pass.
When: test "$(uname -s)" = Darwin
Effects: read, write, exec, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go run ./tools/repairprofile -component bashy -root "${ROOT}" \
  -out "results/repair-profile-bashy-darwin.json"
```

### bashy-windows
The bashy profile on a Windows host: make test + make test-bash (86/86), both
native (`make` resolves through bashy's multicall when the host has no make);
the uutils lane reports skipped (Linux-oci-only), never pass.
When: test "$(uname -s)" = Windows_NT
Effects: read, write, exec, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go run ./tools/repairprofile -component bashy -root "${ROOT}" \
  -out "results/repair-profile-bashy-windows.json"
```

### bashy-uutils-linux-oci
The landmine lane alone: the uutils suite, only in the Linux oci sandbox
(docker or bashy podman present). When the lane is structurally unavailable
the dag skips this target cleanly AND a direct runner invocation records
`skipped (lane unavailable)` in the evidence — never "pass".
When: test "$(uname -s)" = Linux
Effects: read, write, exec, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go run ./tools/repairprofile -component bashy -root "${ROOT}" \
  -only test-uutils -out "results/repair-profile-bashy-uutils.json"
```

### outpost-linux
The outpost profile on a Linux host: go test -short -tags fb_archives over
every package except internal/agent/shell (needs a real TTY) — the Makefile
`test-headless` lane, cross-platform and make-free.
When: test "$(uname -s)" = Linux
Effects: read, write, exec, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go run ./tools/repairprofile -component outpost -root "${OUTPOST_ROOT}" \
  -out "results/repair-profile-outpost-linux.json"
```

### outpost-darwin
The outpost profile on a macOS host: the headless short-test lane.
When: test "$(uname -s)" = Darwin
Effects: read, write, exec, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go run ./tools/repairprofile -component outpost -root "${OUTPOST_ROOT}" \
  -out "results/repair-profile-outpost-darwin.json"
```

### outpost-windows
The outpost profile on a Windows host: the headless short-test lane.
When: test "$(uname -s)" = Windows_NT
Effects: read, write, exec, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go run ./tools/repairprofile -component outpost -root "${OUTPOST_ROOT}" \
  -out "results/repair-profile-outpost-windows.json"
```
