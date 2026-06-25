# Drop-in fidelity ceiling assessment — the last 5 gaps + interactive job control

Status: **2026-06-25.** Written after driving drop-in fidelity to **1100/1105 = 99.5%** (`scripts/bash-fidelity.sh`, de-noised) with **make test-bash 86/86** held on every push. This assesses what blocks 100% and whether it's worth lifting.

## Where we are

| Metric | Value |
|---|---|
| Drop-in fidelity vs bash 5.3 | **1100/1105 (99.5%)** |
| bash 5.3 own fixture suite | **86/86** |
| POSIX-XCU / Oils differential | **0 deviations** (cert pre-flight GO) |

The campaign closed ~37 diffs (1059→1100) with 5 reverts (the dual-gate never shipped a regression). The remaining **5 gaps are all ceiling-class** — none is a simple interp bug.

## The 5 remaining gaps, by root cause

### A. redirect fd-7 (2 cases: redirect__019, __027) — ARCHITECTURAL ceiling
A script opens/closes fds and asserts fd 7 is **closed**; bash (real `fork()` + per-process fd table) has it closed, **ours leaks it open**. The basic case works (`>&7` → "Bad file descriptor", matches bash) — the leak only shows in nested redirect/subshell sequences. **Root: the engine simulates subshells as goroutines, not `fork()`** — there is no real per-process fd table to close-on-fork. (See `sh` `interp/` CLAUDE.md, commit `12f5191d`.) A naive fix hung `comsub` (#246).

### B. interactive job control — NARROWER than "goroutine-not-fork" (corrected 2026-06-25)
**Background jobs are NOT goroutines** — `foo &` runs as a *real OS process* via the ExecHandler, and sh exposes its real PID through `WithBgPidCallback` (`interp/api.go:1568`). **outpost already ships real-process job control on this** (`internal/agent/shell/bgjobs.go` + `cmd/outpost/jobs.go`): a persisted PID registry with `jobs` (list), `bg` (SIGCONT), `kill` (signal), and `fg` (**wait-for-exit**). So `jobs`/`kill %n`/`$!`/`wait`/`bg` all work on real PIDs — Gate C scriptable JC is **11/12**, and the registry extends it across invocations.
The **only** genuine limit is **stdio/controlling-terminal re-attach**: native `fg` (bring a job to the foreground and hand it the terminal) and Ctrl-Z `SIGTSTP` suspend. The in-process runner doesn't own/reattach a controlling terminal to a process group — outpost's comment names it directly: *"stdio cannot be re-attached once a job has detached."* This is the **in-process-runner / detached-process trade-off**, NOT the subshell-goroutine model — and it does NOT share fd-7's root. **bashy can adopt outpost's `bgjobs.go` + `WithBgPidCallback` model** to provide native `jobs/fg(=wait)/bg/kill` (the T3 migration item "outpost job-control → sh/bash"); the irreducible residue is just terminal-reattach `fg` + Ctrl-Z.

### C. recover-vs-abort (2 cases: array__072, assign-extended__009) — PARSE-CORRECTNESS risk
bash parse-errors the whole script and aborts with **zero output**; **ours is more permissive** — it accepts the construct, recovers, and runs it. Matching bash means making the parser *reject* what it accepts — the exact change that **regressed 14 valid-construct fixtures** in the dbracket excursion. High regression risk for 2 cases.

### D. output-ordering (1 case: assign-extended__010) — BUFFERING
Identical content; bash batches stderr "not found" errors before the stdout `[declare]`, ours interleaves differently. **stdout/stderr flush ordering** — buffering-dependent, very hard to match byte-exactly without reworking the write path.

(A residual arith-status edge may still move in/out of the 5 as rounds run; it's the only one that's plausibly a clean single-round close.)

## The lifts (A and B do NOT share a root — corrected)

- **B (job control) is mostly already lifted.** Real-process bg jobs + `jobs/bg/kill/wait/fg-as-wait` work today via `WithBgPidCallback` (outpost ships it; bashy can adopt the `bgjobs.go` model). The only residue is **terminal-reattach `fg` + Ctrl-Z** — which needs the in-process runner to own and hand off a controlling terminal/process group. That's a focused PTY/pgrp feature, **not** a full fork rewrite.
- **A (fd-7) is the subshell-goroutine fd-table** — `( … )` subshells are goroutines, so an fd the script closes inside one isn't closed in a real per-process table. This is the `12f5191d` area; lifting it cleanly is the deep part (a naive fix hung comsub). It is **separate from B** — job control doesn't fix it and vice-versa.

The `sh/plan-dual-mode-job-control.md` real-subprocess path is the heavyweight option that would address both at once, but **B doesn't need it** (the PID-callback model already covers everything except terminal-reattach), so the only thing that truly wants the fork rewrite is A's 2 fd-7 cases — a poor cost/benefit.

## Recommendation

**Ship 99.5% drop-in + 86/86 + 0-dev POSIX as the honest claim.** The last 5 are documented limitations, not bugs:
- **1 architectural** (fd-7 subshell fd-table leak, 2 probe cases) — the only thing wanting the fork rewrite; not justified for 2 cases pre-cert.
- **Job control is largely solved** — adopt outpost's `bgjobs.go`+`WithBgPidCallback` model into bashy for native `jobs/fg(=wait)/bg/kill`; the residue (terminal-reattach `fg` + Ctrl-Z) is a focused PTY/pgrp feature, build **only if VSC-PCTS shows interactive JC is load-bearing in batch mode**.
- **2 parse-permissiveness** (recover-vs-abort) — regression-risky (14-fixture precedent); revisit with a probe-gated surgical pass, not a fleet round.
- **1 output-ordering** — buffering rework, low value.

**100% drop-in is reachable but gated on a fork-model rewrite + risky parser-correctness work** — neither is worth blocking the cert or the launch. The credible public claim is unchanged and strong: *"a pure-Go bash that passes 100% of bash's own 5.3 suite (86/86), 0 deviations from POSIX, and 99.5% drop-in fidelity on a 1,105-case differential."*
