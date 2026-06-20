# Scoping: remaining POSIX-mode job-control (#23–27, #49) and fc (#54–57) behaviors

Scoped 2026-06-20 against the bash 5.3 oracle (`ycode podman run bash:5.3`) and
the sh implementation (`interp/builtin.go` job code, `interp/history.go` fc).
Classifies each remaining behavior as **TRACTABLE** (implement + probe now),
**VERIFY** (sh likely already conformant — confirm + assert), or **CEILING**
(blocked by the pure-Go goroutine job model; document, don't chase).

## Key findings shaping the verdicts

1. **sh already has the job-formatting machinery.** `formatJob`
   (`interp/builtin.go:4992`) prints `[N]<marker>  <State><cmd><suffix>` with
   `currentJob`/`previousJob` markers and `Running`/`Stopped`/`Done` states.
   What it lacks is the **posix format variant**: bash posix prints
   `Done(<status>)` for nonzero exits and `Stopped(<signame>)`; sh prints bare
   `Done`/`Stopped`. So #23/#24 are a **posix-gated format change in one
   function**, not new infrastructure.
2. **The PTY harness CAN drive `jobs` deterministically.**
   `printf 'sleep 0.3 &\njobs\n…' | bashy --posix -i` reliably shows
   `[1]+  Running   sleep 0.3 &`. So job-state **format** is probeable. What is
   NOT reliably probeable is **asynchronous inter-prompt notification timing**
   (#25/#26) — those messages print when a job changes state *between* prompts,
   which depends on the async notifier + real timing.
3. **sh already removes finished jobs after wait/jobs** (`removeFinishedJobs`,
   `interp/builtin.go:5027`, "mirrors bash mark_dead_jobs_as_notified +
   cleanup_dead_jobs") — #27 is likely already conformant.
4. **fc fallback editor is `vi`** (`history.go:2366`), with no posix `ed` gate —
   #55 is a one-line posix-gated change.
5. **`wait` consumes the async Done message** in both bash and bashy, so any
   probe of #23/#24 must observe the state via `jobs`, not after `wait`.

## fc cluster (#54–57) — mostly TRACTABLE

| # | Behavior | Verdict | Notes |
|---|---|---|---|
| 54 | fc -l does not indicate whether an entry was modified | **VERIFY** | sh `fc -l` prints `N\t<cmd>` with no modified flag — likely already conformant; add an assertion. |
| 55 | default fc editor is `ed` (posix) | **TRACTABLE** | sh falls back to `vi` (`history.go:2366`); add a posix gate → `ed`. One-line change + unit test. Non-interactive-testable via `FCEDIT`/`EDITOR` unset + a stub editor probe. |
| 56 | fc treats extra args as an error (vs ignoring) | **TRACTABLE** | sh currently ignores args beyond first/last; add arg-count validation in `fcBuiltin`. Unit-testable. |
| 57 | fc -s with too many args → error + failure | **TRACTABLE** | same `fcBuiltin` arg-validation path as #56; do them together (shared scope). |

fc cluster scope is **`interp/history.go` only** — one disjoint work unit.
Caveat: fc behaviors only manifest with history populated; the in-sandbox gate
is sh unit tests (drive `fcBuiltin` directly), not `-c` probes.

## job-control cluster (#23–27, #49)

| # | Behavior | Verdict | Notes |
|---|---|---|---|
| 23 | job-exit message format is `Done(status)` | **TRACTABLE** | posix-gate `formatJob` (`builtin.go:5012`): nonzero-exit Done → `Done(N)`. Format change; unit-testable + harness-probeable via `jobs`. |
| 24 | stopped-job message is `Stopped(signame)` | **TRACTABLE (unix)** | posix-gate `formatJob` Stopped → `Stopped(SIG…)`. Reachable only where a job can actually stop (unix Wait4 WUNTRACED — the `*_unix.go` path); format unit-testable. |
| 25 | interactive: no job notify between `;`/newline list commands; non-interactive prints status after a fg job | **CEILING-ish** | Async inter-prompt notification timing on the goroutine model; not deterministically probeable. Document; revisit only if the async notifier is reworked. |
| 26 | interactive: defer bg status to next prompt | **CEILING-ish** | Same async-timing class as #25. |
| 27 | permanently remove jobs from the table after notifying via wait/jobs | **VERIFY** | sh `removeFinishedJobs` already does this — confirm against oracle and assert; likely already conformant. |
| 49 | bg builtin format omits current/previous indication | **TRACTABLE** | `bg` output uses the same `formatJob` path; posix-gate the marker so bg omits the `+`/`-` current/previous indicator. Harness-probeable. |

job-control cluster scope is **`interp/builtin.go` job functions** (+
`*_unix.go` for #24) — one disjoint work unit, **separate from fc**. NOTE: this
is the exact code outpost consumes (`bgjobs.go` via `WithBgPidCallback`); any
merge here must additionally gate on `outpost go build ./... + go test
./internal/agent/shell -run 'Job|Bg|Reg'`, not just bashy `make test-bash`.

## Recommended next round (fleet, two disjoint issues)

1. **fc posix conformance (#55/#56/#57, verify #54)** — scope `interp/history.go`.
   Oracle targets: default editor `ed` under posix; extra-args / `fc -s`
   too-many → error+failure. Gate: sh `go test` + new unit tests.
   Capability match: **codex** (surgical arg-validation + one posix gate).
2. **job-state posix format (#23/#24/#49, verify #27)** — scope
   `interp/builtin.go` (+`*_unix.go`). Oracle targets: `Done(N)`,
   `Stopped(signame)`, bg marker omission. Gate: sh `go test` + new unit tests
   + **outpost build/job tests**. Capability match: **claude** (deeper, the
   shared/ outpost-consumed path).

Both deliver via sh unit tests asserting the format strings (no podman needed
in-sandbox); the orchestrator verifies end-to-end via the PTY harness +
`jobs`/`bg` at convergence.

**Defer #25/#26** as documented async-timing ceilings of the pure-Go goroutine
job model — they are the only two of the ten that lack a deterministic probe.
That leaves **8 of 10** remaining behaviors tractable in one two-issue fleet
round.
