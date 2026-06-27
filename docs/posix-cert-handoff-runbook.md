# POSIX cert handoff runbook — the licensed VSC-PCTS run

Status: **handoff (2026-06-27).** This is the turnkey procedure for the parts of
POSIX certification an agent **cannot** do: the license application to The Open
Group and the TET/VSXgen run. It assumes the agent-drivable pre-flight is GREEN
(`scripts/posix-certdryrun.sh` clean — see `conformance-statement.md`). Read
`vsc-pcts-readiness.md` (go/no-go) and `plan-posix-conformance.md` (strategy)
first.

> **Do not start here until the dry-run is clean.** Pulling the 12-month license
> before the free suites are at 0 deviations wastes the clock — the licensed run
> should confirm a known-good state and catch the long tail, not discover basics.

## Pre-flight gate (confirm before applying)

```sh
cd bashy
make test-bash                      # 86/86, no regression
scripts/posix-certdryrun.sh         # VERDICT: CLEAN (or clean-on-built + reasoned pending)
```

If any suite shows deviations, triage them first (see "Triage loop" below) —
each deviation is either a real bug (fix in `../sh`, gated 86/86), a declared
limitation (already in `conformance-statement.md`), or a scope-excluded
standalone-utility test.

## Step 1 — License application (human/legal)

- VSC-PCTS = The Open Group's POSIX Verification Suite for Shell & Utilities,
  the official conformance suite behind POSIX/UNIX certification. Catalog:
  <https://posix.opengroup.org/testsuites.html>.
- An OSS / no-cost 12-month license arrangement has historically existed.
  **Verify current terms, deliverables, and the exact test-suite version at
  application time** — do not assume the historical terms still hold.
- Decide certification vs. self-test: a formal *certification mark* has
  additional process (registration, fees, audited submission). Running VSC-PCTS
  under license for our own verification is the lighter path and is enough to
  back the conformance statement. Pick the target before applying.
- Deliverable from this step: the licensed VSC-PCTS tarball(s) + any TET/VSXgen
  components bundled with them.

## Step 2 — Build the harness (TET3 + VSXgen)

- TET (Test Environment Toolkit) is the execution framework; VSXgen is the test
  scenario generator/driver the shell suite plugs into.
- Build them in a **Linux container** (the suite assumes a POSIX/Linux build
  host; this also keeps the dev machine clean). Mirror the existing pattern:
  the differential harnesses already build container images via
  `docker` / `ycode podman` — reuse that runtime.
- Pin the build in a script (suggested `scripts/vsc-tet-build.sh`, not yet
  written — it depends on the licensed tarball layout) so the run is
  reproducible. Keep the licensed tarball **out of git** (gitignored cache,
  same posture as the yash/bash-5.3 fixtures).

## Step 3 — Wire bashy as the SUT in POSIX mode

- The system-under-test is bashy's **`bash` drop-in** invoked as **`sh`** (or
  with `--posix`), so the suite exercises POSIX mode. Build it for the container
  target: `GOOS=linux GOARCH=<arch> go build -o /usr/bin/sh ./cmd/bash` (the
  differential harnesses already cross-build `cmd/bash` this way).
- **Scope the TET scenario to the shell + builtins.** VSC-PCTS tests *both* the
  shell language and ~160 standalone utilities; if you do not narrow the
  scenario, the run mostly tests the host's `ls`/`grep`/`sed`/… (out of bashy's
  scope — see `conformance-statement.md`). Configure the scenario so `sh`→bashy
  and the utility assertions are either excluded or pointed at the `coreutils`
  sibling track, not counted against bashy.

## Step 4 — Run

- Execute the shell + builtin subset under TET. Capture the journals.
- Expect the bulk to PASS (the dry-run already covers the mechanically-testable
  core); the value is the adversarial long tail the free corpora did not reach.

## Step 5 — Triage loop (per journal entry)

TET journals classify each assertion PASS / FAIL / UNSUPPORTED / UNTESTED / FIP
(further-information-provided). For every non-PASS, sort into exactly one bucket:

1. **Real bug** → fix in `../sh` (`interp`/`expand`/`syntax`), gated:
   `cd ../sh && go test ./...` green **and** `cd bashy && make test-bash` still
   **86/86** (no regression — the non-negotiable anchor), plus a new assertion in
   `posix-parity.sh` or the relevant corpus. Cherry-pick one fix at a time,
   re-measure the full suite, merge only if clean. Then bump `bashy/.sibling-pins`
   and the umbrella submodule pin.
2. **Declared limitation** → already in `conformance-statement.md` (interactive
   JC, `((` ambiguity, `<<${a}`, Go-runtime fds, stream buffering). If the
   journal shows interactive JC is *load-bearing in batch mode*, that is the
   trigger to build the opt-in `set -o realjobs` real-process path
   (`sh/plan-dual-mode-job-control.md`) — otherwise leave it declared.
3. **Scope-excluded** → a standalone-utility assertion. Not bashy's; route to the
   `coreutils` track or exclude from the scenario. Record it so the exclusion is
   auditable, not silent.

Iterate Steps 4–5 until the shell + builtin subset is clean (or every residual
is a documented declared-limitation / scope-exclusion). Then update
`conformance-statement.md` with the official-suite result and the final journal
summary.

## What stays agent-drivable (do these before Step 1)

- Keep `scripts/posix-certdryrun.sh` at VERDICT: CLEAN — finish the PENDING
  free-suite harnesses (dash / modernish / austin) and triage each to 0.
- Keep `make test-bash` at 86/86.
- Keep `conformance-statement.md`'s declared-limitations list final.

The licensed run is the last mile; everything that can be verified for free
should already be verified when it starts.
