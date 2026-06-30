# Handoff — conformance next step (2026-06-22, updated 2026-06-23)

One-line: **bash-5.3 is done (86/86, 100%, default mode). Drop-in fidelity is
now a COMMITTED, repeatable metric (`scripts/bash-fidelity.sh`, baseline
941/1105 = 85%); the weave fleet is actively clearing its backlog. Pick up
**round 2** (below) — re-assign param-expansion, finish `[[ ]]`, run
array/declare/redirect.**

## UPDATE 2026-06-23 — breadth metric shipped + weave round 1 landed

The three "next step" items below are now ADDRESSED:
- **Breadth sweep → committed metric.** `scripts/bash-fidelity.sh` is a 2-way
  (our `bash` vs real bash:5.3), per-case-isolated drop-in fidelity probe —
  the high-signal complement to the 5-shell consensus (which masks bash-only
  gaps as "ambiguous"). Baseline **164 diffs / 1105 = 85%**. This replaces the
  ad-hoc "~86%" with a reproducible number + a triaged backlog (~25 scopes:
  arith/array/dbracket/declare/var-op/redirect/…).
- **`<<${a}` DECIDED: do not fix.** It's a deliberate upstream mvdan/sh parser
  strictness (6 `langErr` tests assert it; even Oils' own OSH rejects it; bash
  itself *warns*). Diverging the streaming parser for a bash-discouraged edge
  case isn't worth it. Goes in the declared-limitations list.
- **Oils mining → low-signal beyond the curated set** (Oils-harness-var
  artifacts: `$TMP`/`$SH`/argv.py). `bash-fidelity.sh` is the better frontier.

**Weave round 1 (3 workers, disjoint sh files, gated 86/0): fidelity 164 → 159
(+5).** codex→`expand/arith.go` (arith gaps; salvaged — uncommitted at submit);
claude→`interp/test.go` (2 `[[ ]]` tilde cases + diagnosed the other 11 with
verified patches → **`docs/dbracket-fidelity-round2.md`**); aider→`expand/param.go`
STALLED (0 edits/55m — wrong tool for the hard `${x@Q/@P/@a}` cluster).
Landed: sh `1608a824`, bashy pin, umbrella `72ede1d`.

## ROUND 2 (next — drive these via the weave fleet, gate each 86/0)

1. **`[[ ]]` — finish the 11 cases** in `docs/dbracket-fidelity-round2.md`. Most
   have VERIFIED patches; several are blocked only on a coordinated
   `interp/interp_test.go` assertion update (e.g. `=~` regex wording, case 005).
   The orchestrator can apply the ready ones directly. Files: `interp/test.go`,
   `syntax/parser.go`, `pattern/`, `interp/runner.go` — NOT disjoint, sequence them.
2. **param-expansion (re-assign)** — `expand/param.go`, the `${x@Q/@P/@a}`
   transform operators + `${x:-y}` family. aider failed; give it to **codex or
   claude** (deep). 17 cases.
3. **The rest of the backlog** — array (`expand/param.go`+`interp/vars.go`),
   declare/`assign-extended` (`interp/builtin.go`), redirect (`interp/runner.go`),
   word-split, getopts, brace-expansion. Triage from a fresh
   `bash-fidelity.sh` run; group by sh source FILE (disjoint), one agent per file.

Reminder: fixes live in **`sh`**; run weave on `sh`; the authoritative gate is
**bashy `make test-bash` 86/0 + `bash-fidelity.sh` re-measure**, run by the
orchestrator at merge (workers self-verify with `go test ./interp -run
TestRunnerRun$ ./expand ./syntax` + `gosh`). NEVER touch `docs/` from a worker.

---

(Original 2026-06-22 note follows — the VSC-PCTS cert pre-flight is still the
strategic frame; the breadth-sweep item is now the bash-fidelity campaign above.)

## Where we are (verified)

- **bash-5.3 fixture suite: 86 / 86, 0 failing, 0 skipped — 100%** (default
  mode, per `docs/TODO.md`, last flips 2026-06-18). The shell-language core is
  in good shape; do **not** re-litigate it.
- **Origin baseline, verified 2026-06-22:** pristine upstream `mvdan/sh`
  `gosh` **v3.13.1** scores **3 / 86** on this same suite (passes `extglob3`,
  `invert`, `precedence`; one fixture hangs/times out — not a latent pass).
  Method: point `THIS_SH=<upstream gosh>` through the `Makefile` `test-bash`
  loop (same fixtures + transforms). This is the provenance for the published
  **"3/86 → 86/86"** story; keep it reproducible (see "Suggested" below).
- **POSIX differential:** 0 deviations on the clean-room XCU corpus
  (`scripts/posix-diff.sh`, `scripts/oils-diff.sh`); Gate-D mining found +
  fixed 12 real bugs over ~half the Oils suites. Strong, **not yet
  proven-sufficient** against the cert (it's adversarial + broader).

## The next step (in order — from `vsc-pcts-readiness.md` § Pre-flight)

The gating work is **breadth + harness/license**, not the language. The two
agent-doable items, do these first:

1. **POSIX-mode breadth sweep.** Our 86/0 is *default* mode; the cert runs
   bashy as `sh` / `--posix`. Drive a posix-mode differential pass —
   `scripts/posix-parity.sh` is the seed — until it's green, so we aren't blind
   to posix-mode-only behavior. (Prior sweep sat ~86%; the 2-way bash `--posix`
   oracle is more sensitive than the 5-oracle consensus, so re-measure with it.)
   Fix what surfaces, each fix gated 86/0 + regression-tested.
2. **Finish Oils mining** (remaining ~116 of 222 suites) → drive the
   live-differential to **stable 0-deviations** across the whole corpus; fix
   what surfaces. Same gate discipline.

Then the one quick decision:

3. **Resolve `<<${a}`** — bashy parse-errors an expansion in a heredoc
   delimiter word; bash accepts. Decide whether bash-fidelity warrants relaxing
   the parser before the run, and record the decision.

Human / non-agent steps (flag to the owner, do **not** block on them):
- VSC-PCTS license application to The Open Group (legal/human).
- Stand up TET, wire bashy as the SUT in POSIX mode, scope the scenario to
  **shell + builtins** (the ~160 standalone utilities are out of scope).
- Dual-mode job-control decision — build the opt-in real-process path **only
  if** VSC-PCTS data shows interactive JC is load-bearing in batch mode.

## Guardrails (do not skip)

- **Every change is gated on the FULL `make test-bash` 86/0** — measure under a
  clean PATH (`PATH=/bin:/usr/bin:$(dirname $(which go))`; a wrapper shim in
  PATH can shadow `sh` and false-fail) and with the `external/bash-5.3`
  fixture symlink present (gitignored — a missing symlink false-passes).
- **Declared limitations stay declared, don't "fix" them blind:** interactive
  terminal job control (goroutine-not-fork) and the `((cmd)||(cmd))` spacing
  ambiguity are known/bounded — they go in the conformance statement, measured
  not assumed.
- **Published-number hygiene (important):** downstream materials now cite
  "bash-5.3 100% (86/86)" and the "3/86" origin. Keep both reproducible. Do
  **not** let any "100% POSIX" claim go out **bare** — anchor it to a named
  suite (today: the XCU clean-room corpus, 0 deviations; later: a VSC-PCTS
  result). "Clean on what we tested" ≠ "conformant"; say which.

## Suggested (small, high-value)

Make the origin baseline permanently reproducible from the repo: add a tiny
`make test-bash-baseline` target (or `scripts/baseline-upstream.sh`) that
`go install`s upstream `mvdan.cc/sh/v3/cmd/gosh@<pin>` and runs the existing
loop with `THIS_SH=$gosh`, printing the `3/86`. Cements the "3/86 → 86/86"
claim without a manual rerun.

## Go / no-go

**Go for the cert** when: posix-mode breadth sweep green, Oils mining at stable
0-deviations, `<<${a}` decided, declared-limitations list final. Pulling the
12-month license before that wastes the clock — the run should confirm a
known-good state and catch the long tail, not discover basics.

Read next: `docs/vsc-pcts-readiness.md` (the live checklist this summarizes),
`docs/plan-posix-conformance.md` (strategy/scope), `docs/TODO.md` (scoreboard).
