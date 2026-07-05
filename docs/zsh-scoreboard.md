# zsh-own-suite scoreboard (Tier 0 of the zsh-compatibility ladder)

**What:** score bashy on **zsh's own regression suite** — `Test/*.ztst` at the
`zsh-5.9` tag (the version macOS ships as `/bin/zsh`, and the version the
engine's experimental `LangZsh` parser variant tracks).

**Why:** the strategic context and the full tiered effort estimate live in the
umbrella doc `bashy-zsh-compatibility-estimate.md`. Tier 0 = measure, don't
implement: bashy already runs a slice of zsh incidentally through the shared
POSIX+bash core; this scoreboard turns "some degree of compatibility" into a
number nobody else publishes, with the corpus named. Prior art high-water mark
is Oils, which merely *parse-ignores* one zsh construct — no shell
reimplementation has ever scored itself on zsh's own suite.

## Run it

```sh
make test-zsh            # scoreboard; output under /tmp/zsh-scoreboard
make test-zsh-list       # print the current failure list
bashy dag suites.md zsh  # as part of the conformance matrix (INFO row)
```

Requires a real `zsh` on the host (macOS: `/bin/zsh` 5.9) and network on the
first run (clones zsh into the gitignored `.zsh-tests/` cache, never vendors —
same posture as `.yash-tests/`; zsh's license is permissive but the suite is
used strictly as a runtime-fetched oracle corpus).

## Mechanism

zsh's native driver (`Test/ztst.zsh`) is itself a zsh script that evals each
test chunk inside its own process — it can only ever test the shell it runs
under, so it cannot score bashy. `tools/ztst` (a dev-only Go runner, not part
of `make build`) reimplements the `.ztst` file format:

- parses fixtures (sections `%prep`/`%test`/`%clean`; indented code chunks;
  status line `<code><flags>:<message>`; `<` stdin, `>` stdout, `?` stderr
  blocks; `*>`/`*?` pattern lines; `F:` notes);
- generates one driver script per fixture that evals every chunk in **one
  persistent shell process** (state carries across tests) inside a
  `ZTST_execchunk` function (so `typeset` creates locals, like ztst.zsh),
  with per-test stdin/stdout/stderr capture;
- mirrors the build-tree layout fixtures assume: cwd `<work>/Test`,
  `../Src/zsh` → the shell under test (many fixtures spawn fresh instances),
  `$ZTST_exe`, a `config.h` shim (`PATH_DEV_FD`/`HAVE_FIFOS` for D03),
  `fpath` pointed at the checkout's `Functions/`+`Completion/` trees;
- compares host-side: exact status/stdout/stderr, honoring flags `f` (xfail),
  `d`/`D` (skip stream), `q` (expected text expanded in-driver), `-` (any
  status); `*>` pattern lines are matched by delegating to the real zsh
  binary so both arms are judged by one pattern engine.

### Both arms, one runner — validity via the reference arm

`scripts/zsh-scoreboard.sh` runs the suite twice through the same runner:
once under real `zsh -f`, once under `bin/bash` (bashy). The runner's
deliberate Tier-0 approximations (option save/restore ≈ `set +e +u +f`
instead of the `$options` round-trip; `q`-flag `${(e)…}` ≈ unquoted heredoc)
apply to both arms identically, and **real zsh defines validity**: a case
real zsh fails under this runner is runner/environment noise and is excluded
from the denominator. Reference-arm fidelity at time of writing: ~98.8%
(~18 noise cases of ~1,500).

Headline metric: **bashy passes N of the M cases real zsh passes**, reported
per class and total; failures land in `<out>/failures.txt`, per-case verdicts
in `<out>/{zsh,bashy}.tsv`.

## Scope and claim discipline

- Classes **A B C D E W Z** (grammar, builtins, constructs, expansion,
  options, jobs/history, contrib utils). Excluded: **P** (needs root),
  **V** (zmodload modules), **X** (zle), **Y** (completion) — the
  interactive/module surface, out of Tier-0 scope by design (see the tier
  table in the umbrella estimate doc).
- **INFO metric, never a gate.** Per `cross-shell-conformance-baseline.md`,
  never quote a bare "N% zsh compatible": the claim is always
  "N% of zsh's own suite (zsh-5.9 tag), non-interactive classes, valid-case
  denominator".
- The number is a **baseline for the Tier-1 decision**, not a target to
  chase piecemeal: genuine semantic work belongs in `../sh` behind a
  dialect seam (see the umbrella doc §4 Tier 1), not as one-off hacks to
  flip scoreboard rows.

## Baseline (2026-07-05, first run)

Host: macOS, reference `/bin/zsh` 5.9, bashy `bin/bash` at HEAD.

```
A: 29/363 (7%)   B: 27/239 (11%)  C: 13/261 (4%)   D: 4/481 (0%)
E: 1/121 (0%)    W: 0/20 (0%)     Z: 0/21 (0%)
bashy: 74 pass / 1506 valid = 4%   (runner-noise excluded: 21)
```

Failure anatomy (bashy arm, 1,527 scoped cases): **PREPFAIL 450** (the
fixture's own `%prep` is zsh code bashy can't execute, forfeiting the whole
file — e.g. B02typeset, A04redirect, D02glob), **FAIL 740** (runs but
diverges — the zsh `print` builtin alone is everywhere: 513 occurrences in
D04parameter), **NOSTATUS 258** (a chunk killed the driver shell mid-file),
OK 74, SKIP 3, UNIMPL 2.

Reading: bashy-as-is is **not** meaningfully zsh-compatible on zsh's own
corpus — the incidental compatibility runs in the other direction (POSIX/bash
scripts under zsh). This grounds the umbrella doc's Tier-1 sizing: the gap is
the semantics layer, and even the "easy" clusters (a `print` builtin, `setopt`
tolerance, 1-based subscripts) belong behind the Tier-1 dialect seam, not as
scoreboard-chasing patches.
