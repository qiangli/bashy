# Evidence: bounded inbox wait reads once at the deadline (story 080b089a3573)

Cause: a bounded `inbox --wait` only did a full read when the change
notifier's fingerprint moved (full rescan is 30s). A lost native event let it
report `EMPTY (timeout)` with a peer reply already stored.

Fix: `runUnifiedInboxWithPoll` forces one read at the bound.

- RED (before fix): `TestInboxWaitBelowMissingStoreAncestor` failed 2 of 6
  (`inbox_test.go:267 bounded wait stdout="", want only B reply`).
- GREEN (after fix): 30 of 30; and `-count=20` of
  `TestInboxWaitBelowMissingStoreAncestor` plus the deterministic test passed.
- Deterministic seam: `TestInboxBoundedWaitReadsQueuedReplyAtDeadlineWhenNotifierUnchanged`
  (fingerprint constant, reply stored without a notification). Without the fix:
  `bounded wait stdout="" stderr="EMPTY (timeout)\n", want queued reply delivered at the bound`.
  With the fix: pass, exactly two reads (initial + deadline).

Command: `GOWORK=off go test ./internal/agentos -run '<test>' -count=N`.
The pre-correction commit is preserved on local branch `bashy-orig-bbbc4869`.
