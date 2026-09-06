---
id: 5a6747e44263
kind: task
title: 'Bug: sprint instruct races managed-session control socket readiness'
seq: 235
status: assigned
priority: p0
created: 2026-09-06T03:17:42.788144Z
assignee: codex-gpt5.6-sol
sprint: 127
---

Repeated 3/3 in bash script/e2e-sprint-modes.sh at M2b: immediately after sprint start --managed, sprint instruct fails connecting to the manager control socket with connection refused. The detached manager process still exists and later reports reachable, so startup returns before its control socket is ready. Fix the supported managed-session path without adding a new feature; add a deterministic regression test covering immediate instruct after start. Acceptance: M2b passes repeatedly, focused pkg/weave/pkg/foreman tests pass, and scripts/crossvet.sh passes.

RESOLUTION 2026-09-05: `waitForSprintOwnerControl` now requires a successful
Unix-socket connection instead of treating the socket pathname as proof of a
listener. A deterministic regression test holds a stale pathname, proves the
wait does not return, then installs a listener and proves readiness completes.
Focused `internal/agentos` tests and `go test -short ./...` pass. A freshly
built binary passes `script/e2e-sprint-modes.sh` 22/22, including immediate
`sprint instruct`; coreutils `scripts/crossvet.sh` passes all targets.
