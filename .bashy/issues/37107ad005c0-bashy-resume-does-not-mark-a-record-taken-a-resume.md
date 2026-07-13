---
id: 37107ad005c0
kind: bug
title: 'bashy resume does not mark a record taken: a resumed handoff still shows in --list and can be grabbed twice'
status: closed
priority: p1
refs:
    - ../coreutils
reporter: qiangli
created: 2026-07-12T23:32:07.100452Z
closed: 2026-07-12T23:41:05.568412Z
resolution: declined
closed_by: qiangli
---

## Observed

    $ bashy resume --list
    20260712T231259Z-1ff574ad  ...  from=claude  SECURITY/COMPLIANCE UPLIFT ...

A record that has been RESUMED still appears in the list, so a second agent can resume
the SAME handoff and two agents duplicate one piece of work -- the collision this whole
system exists to prevent.

## Root cause: the write side of an existing design is missing

The design is already HALF there, which is the tell:
  - `pkg/handoff/record.go:117` has `ResumedAt *time.Time` / `ResumedBy *principal.Ref`,
    with a comment "stamped when the record is claimed".
  - `pkg/handoff/store.go:110` `Pending()` already "returns the UN-RESUMED handoffs".

But `resume` (cli.go:189) never STAMPS ResumedAt/ResumedBy, and/or `resume --list` does
not call `Pending()`. So the filter exists, the fields exist, and nothing ever sets
them. This is the exact twin of the issue->run latch bug (a63cd7f0): a link/state that
is drawn but whose transition is never written.

It is also today's recurring theme: a state reached by the ABSENCE of an update. A
handoff is "available" until proven taken, and nothing ever proves it.

## Fix (supersede, don't delete)

1. `resume <id>` stamps `ResumedAt` = now and `ResumedBy` = the resuming principal,
   atomically, BEFORE handing over — so a race between two resumers is decided by who
   wrote first, and the loser is told it was already taken (like the weave
   double-queue guard).
2. `resume --list` shows `Pending()` (un-resumed) BY DEFAULT.
3. `resume --list --all` shows resumed records too, with `taken by <who> at <when>` --
   history is kept and marked, never deleted (matches the kb supersede-not-delete rule
   and the audit trail).
4. Resuming an already-taken record without --force is refused, naming who holds it.

## Verify

    go test ./pkg/handoff/
    # a resumed record disappears from `resume --list` and appears under `--list --all`
    # marked taken; a second `resume` of it is refused.

## Resolution

NOT A BUG (verified). The record in --list had resumed_at:(none) = pending, correctly shown. resume stamps ResumedAt in-place (Save overwrites by ID); Pending() filters it. I diagnosed from a hunch without testing. Real adjacent gap, if wanted: no `--list --all` to see WHO took a record.
