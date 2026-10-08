# Sprint 379 / Story 412: Windows inbox notifications

Story-ID: `39b5d2028677`. Worker base: `7c377034`; reported candidate:
`ffc629b1`. Work and gates use `GOWORK=off`.

## Plan

Reproduce the reported waits on noviwin1.local from copied standalone trees,
trace registration and event delivery, add a regression at the failing seam,
and fix that seam without lengthening waits or skipping tests. Run module
build/vet and focused tests, then commit on the worker branch for integration.

## Diagnosis and change

`notify` v0.9.3 shares its native watch tree process-wide. Bashy registered a
nonrecursive nearest-existing ancestor for a missing store. On Windows, a later
recursive child with the same event mask can be absorbed by that ancestor:
`notify.Watch` returns nil without upgrading `ReadDirectoryChangesW`. Child
shutdown also affects later registrations below that ancestor. The notification
generation stays unchanged, so the bounded inbox never resnapshots before its
one-second deadline (the backstop rescan is 30 seconds).

The original pair passed in isolation on both the reported candidate and the
worker base; 20 repetitions and a base full package run also passed. A private
missing-store ancestor reproducer is therefore essential: it made the exact
original board and Meet assertions fail, rather than relying on incidental
process-wide watcher state. The controlled setup also exposed the latent issue
on macOS. A minimal Windows probe confirmed that both registrations return nil
while the child receives no events. Merely making the ancestor recursive was
insufficient across successive child watchers.

The fix initializes each configured store directory and watches that root
directly. No unrelated ancestor is subscribed. Incomplete registration returns
an unavailable fingerprint so the gate reads fully and retries registration on
later polls. Normal idle sampling remains an atomic generation read. The
observable filesystem change is creation of absent inbox store directories.
No timeout, dependency, or test-skip changes are involved.

## Reproduction

On noviwin1.local, before the production fix:

```text
go test -count=1 -v -run TestInboxWaitBelowMissingStoreAncestor ./internal/agentos
=== RUN   TestInboxWaitBelowMissingStoreAncestor/board
    inbox_test.go:187: bounded watch did not deliver new input: ""
=== RUN   TestInboxWaitBelowMissingStoreAncestor/meet
    inbox_test.go:262: bounded wait stdout="", want only B reply
FAIL (exit 1)
```

`TestInboxChangeNotifierSeesWriteBelowAnotherNotifiersAncestor` independently
failed because the generation never advanced (exit 1). The two original test
functions are reused unchanged in the controlled regression. Another regression,
`TestInboxChangeNotifierRetriesUnavailableStore`, covers failure and recovery
of store registration.

## Validation

All commands below use the final production change. Windows runs use Go 1.27.1,
the toolchain selected by go.mod (the host launcher reports Go 1.27.0 outside
the module). Dragon uses Go 1.27.1.

- Dragon/macOS: `go build ./...`, `go vet ./...`, and
  `go test -count=1 -run 'TestInbox|TestWireMessageBoardGatesRelayOnHostNotifier' ./internal/agentos`:
  exit 0 (focused tests 4.223 seconds).
- noviwin1.local: `go test -count=10 -v -run
  'TestInboxChangeNotifierSeesWriteBelowAnotherNotifiersAncestor|TestInboxWaitBelowMissingStoreAncestor|TestInboxWatchDeliversANewBoardPostWithoutHumanRelay|TestInboxBoundedWaitDoesNotFinishOnOwnMeetPost'
  ./internal/agentos`: exit 0, 14.117 seconds. `go build ./...` and
  `go vet ./...`: exit 0, captured directly with `%ERRORLEVEL%`.
- noviwin1.local: `go test -count=1 -v ./internal/agentos`: PASS, exit 0,
  135.027 seconds, including all three new regressions.
- novidesign.local, Linux arm64 Podman machine `bashy`:
  `s379-agentos-linux.test -test.v -test.count=1 -test.run=TestInbox`: PASS,
  exit 0. The binary was cross-built on Dragon with `GOWORK=off CGO_ENABLED=0
  GOOS=linux GOARCH=arm64 go test -c`, and executed only inside the Linux VM.
  Its first launch lacked the fleet test ring. The rerun supplied the unchanged
  pinned yoke `pkg/fleet/testdata/ring` fixture, with that package's compiler
  `-trimpath` mapping the source location to the private remote fixture directory.
  Raw log: novidesign.local `~/s379-issue8/linux-inbox.log`.

Windows raw logs are under `%USERPROFILE%\s379-issue8`: `ancestor-red.log`,
`green-focused.log`, `build.log`, `vet.log`, and `full-green.log`. The original
operator log remains `%USERPROFILE%\s379-claude\win-b1.log`.

No push or CI run was requested under the final worker contract; the conductor
integrates the committed worker branch.
