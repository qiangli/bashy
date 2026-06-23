# Handoff — conformance next step (2026-06-22)

One-line: **bash-5.3 is done (86/86, 100%, default mode). The remaining
conformance work is the POSIX / VSC-PCTS cert pre-flight — pick up the
`vsc-pcts-readiness.md` checklist, starting with the POSIX-mode breadth
sweep.** This note is the next instruction for the agent picking up cold.

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
  clean PATH (`PATH=/bin:/usr/bin:$(dirname $(which go))`; the ycode shell
  wrapper shadows `sh` and false-fails) and with the `external/bash-5.3`
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
