# Scoping: remaining POSIX-mode job-control (#23–27, #49) and fc (#54–57) behaviors

Scoped 2026-06-20 against the bash 5.3 oracle (`bashy podman run bash:5.3`) and
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
| 25 | interactive: no job notify between `;`/newline list commands; non-interactive prints status after a fg job | **TRACTABLE** (corrected — was wrongly "ceiling") | The interactive async-timing half isn't deterministically probeable, BUT the POSIX-relevant clause is the **non-interactive** one, and it IS `-c`-testable. Source `jobs.c:4613` + `:4625`: in DEFAULT `-c` mode a foreground *external* command killed by a fatal signal prints to stderr `bash: line N: <pid> <signal desc> [(core dumped)] <cmd>`; POSIX `-c` mode takes the skip-notify branch (defers to wait/jobs). bashy prints **nothing** in default → gap in default mode. Implement in sh via `*exec.ExitError`/`syscall.WaitStatus.Signaled()`+`.Signal()`. |
| 26 | interactive: defer bg status to next prompt | **TRACTABLE** (corrected) | Same non-interactive signal-notification path as #25 — one fix covers both. |
| 27 | permanently remove jobs from the table after notifying via wait/jobs | **VERIFY** | sh `removeFinishedJobs` already does this — confirm against oracle and assert; likely already conformant. |
| 49 | bg builtin format omits current/previous indication | **TRACTABLE** | `bg` output uses the same `formatJob` path; posix-gate the marker so bg omits the `+`/`-` current/previous indicator. Harness-probeable. |

job-control cluster scope is **`interp/builtin.go` job functions** (+
`*_unix.go` for #24) — one disjoint work unit, **separate from fc**. NOTE: this
is shared `sh` code consumed via the `WithBgPidCallback` hook by other
consumers too; any merge here must additionally gate on their build/job tests,
not just bashy `make test-bash`.

## Recommended next round (fleet, two disjoint issues)

1. **fc posix conformance (#55/#56/#57, verify #54)** — scope `interp/history.go`.
   Oracle targets: default editor `ed` under posix; extra-args / `fc -s`
   too-many → error+failure. Gate: sh `go test` + new unit tests.
   Capability match: **codex** (surgical arg-validation + one posix gate).
2. **job-state posix format (#23/#24/#49, verify #27)** — scope
   `interp/builtin.go` (+`*_unix.go`). Oracle targets: `Done(N)`,
   `Stopped(signame)`, bg marker omission. Gate: sh `go test` + new unit tests
   + **downstream consumers' build/job tests**. Capability match: **claude**
   (deeper, the shared `sh` job-control path).

Both deliver via sh unit tests asserting the format strings (no podman needed
in-sandbox); the orchestrator verifies end-to-end via the PTY harness +
`jobs`/`bg` at convergence.

**Defer #25/#26** as documented async-timing ceilings of the pure-Go goroutine
job model — they are the only two of the ten that lack a deterministic probe.
That leaves **8 of 10** remaining behaviors tractable in one two-issue fleet
round.

## CORRECTION (2026-06-20): "ceiling" was premature — checked the bash source

The earlier draft labelled #25/#26 (and treated #28) as pure-Go ceilings. That
was wrong: inspecting the GNU bash 5.3 reference implementation shows all three
have concrete, implementable mechanics. (Recurring lesson: never call something
a ceiling without reading the reference first.)

### #25/#26 — non-interactive signal-death notification (source: jobs.c notify_of_job_status)
Oracle matrix, foreground external command killed by signal, `bash -c` (default):

| signal | default `-c` stderr | posix `-c` |
|---|---|---|
| SEGV | `bash: line N: <pid> Segmentation fault (core dumped) <cmd>` | (none) |
| ABRT | `bash: line N: <pid> Aborted (core dumped) <cmd>` | (none) |
| FPE  | `bash: line N: <pid> Arithmetic exception (core dumped) <cmd>` | (none) |
| ILL  | `bash: line N: <pid> Illegal instruction (core dumped) <cmd>` | (none) |
| BUS  | `bash: line N: <pid> Bus error (core dumped) <cmd>` | (none) |
| TERM | `Terminated <cmd>` (bare — IS_FOREGROUND branch, no prefix/coredump) | (none) |
| INT / PIPE / QUIT | (none — suppressed) | (none) |

So: DEFAULT mode prints the signal description (most fatal signals with
`(core dumped)`; SIGTERM bare; SIGINT/SIGPIPE suppressed); POSIX `-c` suppresses
all (defers to wait/jobs). **bashy prints nothing in either mode → real gap in
DEFAULT mode.** `-c`-testable, implementable in sh.

### #28 — vi `v` (edit-and-execute) command
NOT a fundamental ceiling. `github.com/ergochat/readline@v0.1.3` already ships a
vi mode (`vim.go`: `opVim`, movement/insert/delete/change/replace) — it simply
lacks the `v` edit-and-execute command. Closing #28 is a **library-level**
change: add the `v` handler in a vendored copy (`sh/libs/readline/` per sh
CLAUDE.md's local-vendoring rule, with CREDITS) or wire it at the bashy
interactive layer. POSIX detail: in posix the `v` command invokes `vi` directly;
non-posix checks `$VISUAL`/`$EDITOR` first.

### Revised standing
**0 true ceilings.** All 3 remaining behaviors are implementable: #25/#26 as one
sh change (default-mode signal notification), #28 as a readline library/wiring
change. The "73/76, 3 ceilings" framing is retired — it's 73/76 with 3 tractable
items (one sh fix + one library feature).
