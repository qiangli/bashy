---
title: "Shaping a build, part 1: the pipeline"
date: 2026-11-10
series: shaping-notes
part: 1
summary: How a wish becomes a verified, reversible change on my copy only — with a diagram, an equation and a code block to show what the journal renders.
---

Every change to a tool I depend on goes through the same small pipeline, and
the pipeline is the subject of this series. Not the theory — the moving parts
I can point at on my own machine.

## The shape of the loop

A wish is just words. The interesting part is everything that has to be true
before the wish touches anything running:

```diagram
wish -> candidate : plain words in
candidate -> verify : declared checks
verify -> preview : verdict must pass
preview -> approve : only for me
approve -> undo : always a way back
```

Verify runs ordinary commands in an isolated clone. If the checks do not
pass, there is nothing to approve: the verdict, not a feeling, moves the
change forward.

## Why checks must be able to fail

A check that cannot fail is a decoration. The probability that a change is
good given a verifier that always says yes is just the prior:

$$P(\mathrm{good} \mid \mathrm{pass}) = P(\mathrm{good})$$

which is a long way of saying the check added no information. A real check
has a failure mode — remove the built index and it goes red. The base rate of
caught mistakes, call it $r$, then gates what reaches approval:

$$P(\mathrm{good} \mid \mathrm{pass}) = \frac{p \cdot (1 - r)}{p \cdot (1 - r) + (1 - p) \cdot r'}$$

where $p$ is the chance the change was good to begin with and $r'$ the false
pass rate. The noise term $r'$ shrinks like $1/\sqrt{n}$ over $n$ independent
checks, so the honest fix is more checks that can fail — design the check so
$(r')^{2}$ is small, and prove it by breaking the app on purpose.

## A gate you can read

The verifier for this site is a shell script anyone can read:

```bash
# checks run in an isolated clone of the candidate
set -euo pipefail
test -s public/index.html          # the build produced a home page
grep -q '<video' public/p/*/index.html  # media posts kept their players
```

Three lines, one claim: the candidate still builds a site. Everything else —
scoped activation, undo, history — is machinery around that claim.

Next in the series: recording the terminal scene that shows the loop running.
