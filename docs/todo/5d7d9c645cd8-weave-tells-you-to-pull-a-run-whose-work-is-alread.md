---
id: 5d7d9c645cd8
kind: task
title: weave tells you to pull a run whose work is already in — and the pull would REGRESS main
seq: 223
status: todo
priority: p1
created: 2026-09-06T00:17:45.257281Z
sprint: 126
---

FOUND while acting on the first `sprint tick` worksheet, which reported three runs
"waiting on you to review and merge". All three were already integrated; merging any
of them would have DELETED work.

WHAT HAPPENED. Runs coreutils#3, coreutils#4 and bashy#19 sat in `submitted` while
their work had been integrated BY HAND (fda5c854, e261241f, 5dc3369) — the same
patch, a different SHA. `weave status` therefore said, confidently:

    action:  pull or steward with `weave pull 3`
    merged:  no — `weave pull` to merge

That advice is WRONG AND DESTRUCTIVE. Each branch is based on a clone-time main; a
full tree diff against the real main shows the merge REMOVING pkg/weave/
weave_story_tick.go (603 lines) and its tests (358 lines), among others. bashy#19 is
worse in kind: its docs/todo/ snapshot DELETES story 221 outright and reverts story
211 from done back to todo — merging it would silently reopen finished work.

ROOT CAUSE. weaveReconcileMerged flips a submitted item to done when its work is
merged, but weaveItemMerged tests SHA ANCESTRY. Hand-integrated work (cherry-picked,
re-committed, or re-implemented identically) is patch-equivalent and never an
ancestor, so the reconcile never fires and the run is stuck in `submitted` forever
while the board reports it as outstanding.

THE TRAP THAT COST THE MOST TIME, AND IT GENERALIZES. `git cherry -v main HEAD` run
INSIDE the workspace reported `+` (not integrated) for all three. It was wrong: a
weave workspace has its own `main` ref frozen at clone time, so the comparison used a
baseline that predates the integration commits. Re-run from the real checkout against
the real main, the same command reported `-` for #3 and #4. A cherry check is only as
good as its baseline, and the workspace baseline is stale BY CONSTRUCTION.

FIX (smallest honest one, no new machinery):
  1. weaveItemMerged should fall back to PATCH EQUIVALENCE (git cherry / patch-id)
     against the real base when SHA ancestry fails, so hand-integrated runs reconcile
     to done on their own.
  2. `weave status` must not print "pull or steward with `weave pull N`" for a run
     whose branch is behind base far enough that the merge would delete files. It
     already computes the diff; it should say so, and name the deletions.

BOTH ARE STATED BY THE UMBRELLA CLEANUP RULE ALREADY: remove a branch when it is an
ancestor of main OR when `git cherry` shows its patches equivalent to integrated
patches. weave implements only the first half.

DISPOSITION OF THE THREE RUNS (done): reviewed, verdict recorded on each run thread,
abandoned with --force so each tip is preserved at refs/salvage/abandoned-{3,4,19}.
Nothing was lost and main was not touched.
