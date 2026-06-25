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

### B. interactive job control — the SAME architectural ceiling
`fg`/`bg`/Ctrl-Z/monitor-mode notifications are **non-functional** (no real process groups / SIGTSTP delivery under the goroutine model). *Scriptable* JC (`wait`/`$!`/`kill %n`/`jobs`) is **~conformant (Gate C: 11/12)**. Shares fd-7's root: **goroutine-not-fork**.

### C. recover-vs-abort (2 cases: array__072, assign-extended__009) — PARSE-CORRECTNESS risk
bash parse-errors the whole script and aborts with **zero output**; **ours is more permissive** — it accepts the construct, recovers, and runs it. Matching bash means making the parser *reject* what it accepts — the exact change that **regressed 14 valid-construct fixtures** in the dbracket excursion. High regression risk for 2 cases.

### D. output-ordering (1 case: assign-extended__010) — BUFFERING
Identical content; bash batches stderr "not found" errors before the stdout `[declare]`, ours interleaves differently. **stdout/stderr flush ordering** — buffering-dependent, very hard to match byte-exactly without reworking the write path.

(A residual arith-status edge may still move in/out of the 5 as rounds run; it's the only one that's plausibly a clean single-round close.)

## The unifying lift: real-subprocess path

**A (fd-7) and B (interactive JC) share one root and one fix:** an opt-in **real-process re-exec** execution mode (subshells/pipelines as actual OS processes with real PIDs, fd tables, and process groups) — the plan of record is `sh/plan-dual-mode-job-control.md`. This is a **significant rewrite** of the runner's execution model (the engine's defining design choice is goroutine-not-fork for portability — pure-Go, `GOOS=plan9`/`wasm` builds). It would lift fd-7 + interactive JC together; it would **not** help C (parser) or D (buffering).

## Recommendation

**Ship 99.5% drop-in + 86/86 + 0-dev POSIX as the honest claim.** The last 5 are documented limitations, not bugs:
- **2 architectural** (fd-7 leak, interactive JC) — lift only via the real-subprocess path, and **only if VSC-PCTS data shows interactive JC is load-bearing in batch mode** (per `vsc-pcts-readiness.md` §Known limitations). Building a fork-model rewrite for 2 probe cases is not justified pre-cert.
- **2 parse-permissiveness** (recover-vs-abort) — regression-risky (14-fixture precedent); revisit with a probe-gated surgical pass, not a fleet round.
- **1 output-ordering** — buffering rework, low value.

**100% drop-in is reachable but gated on a fork-model rewrite + risky parser-correctness work** — neither is worth blocking the cert or the launch. The credible public claim is unchanged and strong: *"a pure-Go bash that passes 100% of bash's own 5.3 suite (86/86), 0 deviations from POSIX, and 99.5% drop-in fidelity on a 1,105-case differential."*
