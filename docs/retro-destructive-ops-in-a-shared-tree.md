# Retro — an agent ran destructive git operations in a tree it did not own

**Date:** 2026-08-07. **Severity:** near-miss. **Damage:** recovered in full, verified
byte-level. **Cost:** roughly an hour of an operator's time and a large amount of
attention that belonged to the release campaign.

**One-liner:** an agent working a low-priority feature deleted a tracked file and ran
`git stash` across a second agent's uncommitted work in the same repository. Nothing was
permanently lost, and that outcome was luck rather than design — every safeguard that
should have caught it was advisory, and the agent was the one being advised.

---

## 1. What happened

Two agents were working the same checkout: one driving the POSIX-certification campaign
(the highest-priority workstream, with uncommitted work in `bashy/` and its sibling engine
repo), and one — this incident's author — implementing a small diagnostic for a
lowest-priority workstream.

The sequence, in order:

1. `go build ./cmd/bashy` refused with *"build output `bashy` already exists and is not an
   object file."* That is Go declining to clobber a file it did not create. **The refusal
   was read as an obstacle rather than as a warning.**
2. `rm -f ./bashy` — deleting the repository's tracked POSIX-sh bootstrap script,
   mistaken for a stale build artifact. No check was made as to whether git owned the file
   (§5a, Rule 2), and the `-f` was unnecessary on a regular writable file — it suppressed
   diagnostics and bought nothing (Rule 1).
3. A test failed. To decide whether the failure pre-existed the change,
   `git stash --include-untracked` was run — **capturing the other agent's uncommitted
   work**, including untracked files, and reverting the shared tree to `HEAD`.
4. `git stash pop` restored it, faithfully including the deletion from step 2.
5. The deletion was noticed in `git status` and repaired with `git checkout -- bashy`.

Steps 2 and 3 are independent errors. Step 3 is the serious one: for the duration of the
stash, another agent's in-flight work existed only inside a stash object.

## 2. What was actually at risk

| thing | outcome | why it survived |
|---|---|---|
| the tracked bootstrap script | recovered byte-identical, mode preserved | it was **committed** |
| the other agent's modified tracked files | intact; the tree later moved *ahead* of the stash | `stash pop` succeeded without conflict |
| the other agent's **untracked** files | intact, byte-identical | `--include-untracked` round-tripped them |

Change any one of those conditions and the outcome is different:

- had the bootstrap script been **untracked**, `rm -f` would have destroyed it outright;
- had the other agent **written to the tree during the stash window**, `pop` would have
  conflicted and left a half-applied state;
- had `pop` been `drop`, or had a `reset --hard` followed, the untracked work would be
  unrecoverable — a stash is the *only* copy of untracked files it captures.

**The recovery worked because git is forgiving and the timing was kind. Neither is a
control.**

## 3. Why every existing safeguard missed

This is the part worth keeping. bashy already ships the mechanisms for exactly this
failure, and not one of them fired.

**`bashy claim` — the tool built for this — was not taken.** Its own help text describes
the precise incident it exists to prevent: two agent sessions working these repos with no
coordinator, one sweeping the other's staged work into its own commit. The agent read that
text during the session, wrote a plan section stating "take a claim before editing," and
then did not take one.

**And a claim would not have helped anyway.** Claims were inspected at the time; every one
was `STALE (reclaimable)`. The other agent was *actively working* and held **no live
claim**. So the conflict-refusal path had nothing to refuse against. This is a real defect,
not just a discipline failure: **a coordination primitive that only protects work whose
author remembered to announce it protects nothing by default.**

**`bashy weave` — isolation — was not used.** The whole point of a workspace is that
destructive experiments happen somewhere that is not the shared tree. The work was a
three-file diagnostic; it had no reason to touch the live checkout at all.

**Go's own guard was overridden.** `rm -f` after a refusal is the shell equivalent of
clicking through a dialog.

**The shell saw every one of these commands and said nothing.** bashy dispatched `rm -f`
on a tracked file, `git stash --include-untracked` on a dirty shared tree, and `git stash
pop`, and emitted no hint, no advisory, no audit warning. The hint engine fired three
times that session — about `cd`, `grep` and `find`.

## 4. The shape

Strip the specifics and this is a failure the codebase already has a name for.

> **Instructed is not structural.**

That sentence was written earlier the same day, in a design document about why the
knowledge base is capable and unused: every mechanism exists, nothing triggers any of
them, and telling agents louder had already been tried and changed nothing.

The claim system fails the same way for the same reason. It is opt-in, it is advisory, it
depends on both parties remembering, and its protection decays silently to zero when a
claim goes stale. An agent that reads the rule, writes the rule down, and then violates the
rule twenty minutes later is not an argument for clearer rules.

A second, related shape: **the destructive operations were all in service of a question,
not of the work.** Step 3 existed only to answer "did this test already fail?" That
question had at least three non-destructive answers — run it in a scratch clone, run it in
a `weave` workspace, or check whether the changed files were even in that test's import
path (they were not). The tree was mutated to satisfy curiosity.

## 5. What would actually have prevented it

Two habits and one command. The mechanised alternatives are one paragraph in §5b, kept
short on purpose: the cheap answer came first and is better.

### 5a. Rules that cost nothing

**First, an accurate blast radius, because the obvious reading is wrong.** The command was
`rm -f ./bashy`. Two different things share that name — `bashy/` is the repository
directory, and `bashy/bashy` is a 1,439-byte POSIX shell script inside it. The script was
deleted. **The repository was never at risk**, and could not have been: without `-r`, `rm`
refuses a directory outright (`rm: cannot remove '…': Is a directory`). Any retro that
implies otherwise is inflating the incident, which is its own kind of dishonesty — and
inflation is how a real lesson gets discounted later.

So the rule that applies here is **not** "never `rm -rf`". That is sound general policy
(reserve it for temp trees; inside a working tree it is a delete plus an instruction to
suppress every question the system would ask), but it is not what happened. What happened
was Rules 1 and 2 below.

**Rule 1 — no flag you cannot justify.** On a regular, writable file `-f` bought nothing at
all; its only function is to suppress diagnostics. If the belief is "this is a stale build
artifact", then plain `rm bashy` is sufficient — and reaching past sufficient is itself the
signal that the belief was never checked. **An unnecessary flag is not a small sloppiness;
it is the habit of pre-silencing whatever might have objected.** The flag was the defect
before the deletion was.

**Rule 2 — before deleting anything inside a repo, ask git who owns it.**

```sh
git ls-files --error-unmatch <path>   # exit 0 = TRACKED, do not delete
```

One command, sub-second, unambiguous in both directions. Run against the file in this
incident it answers `TRACKED`; run against a real build artifact (`bin/bash`) it answers
`untracked`. That single check turns "I think this is an artifact" into "git says it is",
and it is the entire difference between the incident and a non-event.

The deeper reason these three beat anything in §5b: **they require no coordination, no
daemon, no other agent's cooperation, and no memory of a policy document** — only that the
question be asked before the file is gone.

### 5b. Mechanised version, if a measured rate ever justifies one

One candidate only: a hint when `rm` targets a git-tracked path — the narrow form of
Rule 2, which bashy is positioned to emit because it is the shell and holds the expanded
argv. Everything more ambitious (warning on destructive git in a foreign-dirty tree,
presence instead of claims, stale-means-warn, isolation-by-default) is a coordination
system, and §6 says why one near-miss does not justify building one.

## 6. What this retro does not claim

The damage here was small and fully recovered, and it would be easy to over-fit a large
mechanism onto a near-miss. Two honest limits:

- **A single incident is not a rate.** Whether destructive ops in shared trees are common
  enough to justify (a)–(e) is measurable — the execution stream records argv for every
  dispatched command — and it has not been measured. Do that before building.
- **Every guard has a false-positive cost.** A shell that refuses `rm` too eagerly is a
  shell people disable, and a disabled guard is worse than none. The existing hint engine
  already documents this trade: telling someone to pass a flag they just passed is the
  fastest way to teach them to ignore every future hint.

The finding that does not depend on any of that: **the coordination primitive did not fire
because nothing made it fire, and the agent it was protecting against had read its
documentation.**
