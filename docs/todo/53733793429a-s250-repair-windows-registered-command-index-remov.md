---
id: 53733793429a
kind: bug
title: S250 repair Windows registered-command index removal detection
seq: 336
status: done
priority: p0
labels:
    - windows
    - release
created: 2026-09-24T00:54:57.420677Z
assignee: codex-s250
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-24T02:13:00.352056Z
closed_by: codex-s250
---

Exact-head Bashy 5546226 CI run 35940128642 Windows job 107445984392 fails internal/agentos TestRegisteredIndexSeesAddAndRemoveInTheSameShell at registered_test.go:232: after removing shout.yaml the next registeredLookup still resolves. Ubuntu/macOS gates and strict-sh release are held pending diagnosis. The index fingerprints ring directory mtimes; test sleeps 20 ms, which may not reliably advance native Windows directory mtime. Determine whether cache invalidation contract or fixture timing is wrong using native matched controls. Preserve immediate add/remove visibility and existing shell behavior; no timeout or fixture bypass. Acceptance: focused Windows test passes repeatedly on clean exact head, full four-job Bashy CI passes, release source/pins and VSC approval are refreshed as needed.

Acceptance: native Windows focused pair passed 20/20 on the reviewed fix. Exact Bashy 7416f0b workflow 35941029862 passed Ubuntu, macOS, Windows, and meet-spa-fresh; Bashy 7416f0b is the tested source of both v0.28.0 tags. The fix changed only registered-index cache fingerprint handling and did not alter shell fixtures or limits.
