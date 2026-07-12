---
id: a63cd7f0c9f1
kind: bug
title: 'the issue->run back-ref is a one-way latch: an abandoned/failed run orphans its register entry forever'
status: open
priority: p0
refs:
    - ../coreutils
reporter: qiangli
created: 2026-07-12T23:12:52.884624Z
---

## Observed, live

    $ bashy weave abandon 10        # the run died; agy never worked
    $ bashy weave add --from-issue 3e91
    weave add: issue 3e91c872 is already in flight as weave #10

The register entry can NEVER be re-queued. The P0 it describes (CI is red) is
unfixable through the normal path.

## Root cause

`Issue.Weave` is set by `runWeaveAddFromIssue` and cleared in exactly ONE place --
`weaveCloseRegisterOnMerge`, on a successful merge. Every OTHER terminal state
(abandoned, killed, failed) leaves the latch set forever.

The guard that refuses a double-queue is correct and must stay -- two agents on one
issue is the collision this whole system exists to prevent. The bug is that the latch
has no release.

## Fix

Clear `Issue.Weave` whenever its run reaches a terminal state that is NOT a merge:
abandoned / killed / failed. The issue returns to `triaged` (accepted, not started) and
becomes re-queueable.

Also give the operator an explicit escape hatch -- `bashy issue unlink <id>` (or
`weave add --from-issue <id> --force`) -- because the automatic path will always have a
case it missed, and a register you cannot repair by hand is a register that eventually
lies.

## Wider lesson (worth a line in the design doc)

Every LINK between the durable record and the ephemeral execution needs a release path
for each way the execution can die -- not just the way it succeeds. The happy path was
implemented; the four sad paths were not.

## Verify

    # abandon a run, then re-queue its issue -- must succeed
    bashy weave add --from-issue <id> && bashy weave abandon <run> && bashy weave add --from-issue <id>
    go test ./pkg/weave/ ./pkg/issue/
