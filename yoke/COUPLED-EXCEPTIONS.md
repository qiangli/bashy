# Coupled Yoke Verbs Conversion Status and Exceptions (Sprint 677, Story #606)

Tracking the 10 coupled verbs: `todo`, `dag`, `mb`, `inbox`, `kb`, `chat`, `meet`, `foreman`, `sprint`, `weave`.

All 10 are converted. The four originally exception-listed verbs (inbox, kb,
meet, sprint) were converted under the operator's duplication rule
(2026-11-21): no non-internal command depends on another, even if it means
duplicate code.

| Verb | Status | Adapter Entry | Notes / Exception Reason |
|---|---|---|---|
| `todo` | Converted | `yoke/todo` | Landed in 62ee3c74; links `_ "github.com/qiangli/yoke/pkg/weave"` so sprint/story-close reclamation hooks fire in the standalone adapter; standalone deps and help verified. |
| `dag` | Converted | `yoke/dag` | Carries its own copy of `jq.go` (`yoke/cmds/jq/jq.go`) so DAG target bodies resolve `jq` in-process via `coreutils/shell.Handler()` without importing `yoke/cmds/jq`; `AssertStandaloneDeps` clean; routed via `runCommandAdapter`. |
| `mb` | Converted | `yoke/mb` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `inbox` | Converted | `yoke/inbox` | Sprint 677 story #612 (conductor review 2026-11-21): the session-relay, turn-delivery, watcher-claim and poll glue is DUPLICATED into `yoke/inbox` (package main, no import of another command package); routed via `runCommandAdapter`. Copied inbox tests run there; the agentos originals stay for chat turn delivery and the unread hint. |
| `kb` | Converted | `yoke/kb` | Conductor review 2026-11-21 (operator rule: no non-internal command depends on another, even at the cost of duplicate code). `yoke/kb` carries its own recall reader wiring (kb's copy of the code-ring reader, the indexed-file lane, and the recall/context/ask/memory mounts) instead of importing `yoke/cmds/graph`; builds alone `GOWORK=off`; `AssertStandaloneDeps` clean with no allowed extra command packages; routed via `runCommandAdapter`. |
| `chat` | Converted | `yoke/chat` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `meet` | Converted | `yoke/meet` | Conductor review 2026-11-21 (same rule). `yoke/meet` carries its own copy of the `wireMeet()` seams: the permanent-role and room-secretary activation hooks, steward agent selection and budget preview, and the message-board seam, instead of calling into `internal/agentos/steward.go`; builds alone `GOWORK=off`; `AssertStandaloneDeps` clean; routed via `runCommandAdapter`. |
| `foreman` | Converted | `yoke/foreman` | Standalone deps and help verified; routed via `runCommandAdapter`; monolith cmds/foreman import removed. |
| `weave` | Converted | `yoke/weave` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `sprint` | Converted | `yoke/sprint` | Sprint 677 story #612 (conductor review 2026-11-21): the managed sprint-owner loop, monitor, attachSprintWatch with its inbox stream, inbox-ack, wait and await are DUPLICATED into `yoke/sprint` (including its own copy of the inbox glue; no import of `yoke/inbox`); routed via `runCommandAdapter`. Copied sprint tests run there. |

## Duplicated glue (kept in step by hand)

The operator rule trades duplication for independence. The originals stay in
`internal/agentos` because other front-door paths still use them
(`wireWebConsole`, the cobra schema projection, inbox turn delivery); when
one side changes, change the other.

| Adapter | Duplicated file(s) | Original |
|---|---|---|
| `yoke/dag` | `capacity.go`, `capacity_monitor.go` | Sprint capacity services and read-only monitor snapshot closure from `internal/agentos/sprint_monitor*.go`; old dispatch/services retained only in legacy test helpers. No sprint command import. |
| `yoke/dag` | `jq.go` | `yoke/cmds/jq/jq.go` |
| `yoke/kb` | `codering.go`, `index_reader.go`, `recall.go` | `yoke/cmds/graph/codering.go` (+ graph cache helpers), `internal/agentos/index_reader.go`, `kbrecall.go`, `kbmemory.go` |
| `yoke/meet` | `steward_meet.go`, `steward_select.go`, `mb_seam.go`, `main.go` (`wireMeet`) | `internal/agentos/steward_meet.go`, `steward_select.go`, `budget_admission.go`, `messageboard.go`, `agentos.go` (`wireMeet`) |

## Duplication rule (operator, 2026-11-21)

No non-internal command depends on another, even if it means duplicate code.
`yoke/inbox` and `yoke/sprint` each carry their own copy of the agentos glue
they need, extracted as the transitive closure of the entry points
(`newUnifiedInboxCmd`; `newSprintCmd` + `wireSprintOwner`), plus copies of the
existing agentos inbox/sprint unit tests whose closure stays standalone.
Declarations only those tests need live in `testdeps_*_test.go`. A fix to
this glue in `internal/agentos` must be applied to each copy as well.

Not copied (they stay in `internal/agentos`, which still runs them): the
front-door unread-mail hint and hook tests (`inbox_hint_test.go`,
`inbox_reader_hint_test.go`), the agent-roster, Meet turn-copy and
sprint-inbox-ack tests of `inbox_turn_test.go`/`inbox_owner_test.go`, and the
`weave` and `models` cases of the sprint test files.
