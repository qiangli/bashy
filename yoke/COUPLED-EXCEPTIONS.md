# Coupled Yoke Verbs Conversion Status and Exceptions (Sprint 677, Story #606)

Tracking the 10 coupled verbs: `todo`, `dag`, `mb`, `inbox`, `kb`, `chat`, `meet`, `foreman`, `sprint`, `weave`.

| Verb | Status | Adapter Entry | Notes / Exception Reason |
|---|---|---|---|
| `todo` | Converted | `yoke/todo` | Landed in 62ee3c74; standalone deps and help verified. |
| `dag` | Converted | `yoke/dag` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `mb` | Converted | `yoke/mb` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `inbox` | Exception | (kept in internal/agentos) | No standalone pkg/inbox in yoke; implemented directly in internal/agentos coupled to in-process session relays and turn delivery. |
| `kb` | Converted | `yoke/kb` | Conductor review 2026-11-21 (operator rule: no non-internal command depends on another, even at the cost of duplicate code). `yoke/kb` carries its own recall reader wiring (kb's copy of the code-ring reader, the indexed-file lane, and the recall/context/ask/memory mounts) instead of importing `yoke/cmds/graph`; builds alone `GOWORK=off`; `AssertStandaloneDeps` clean with no allowed extra command packages; routed via `runCommandAdapter`. |
| `chat` | Converted | `yoke/chat` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `meet` | Converted | `yoke/meet` | Conductor review 2026-11-21 (same rule). `yoke/meet` carries its own copy of the `wireMeet()` seams: the permanent-role and room-secretary activation hooks, steward agent selection and budget preview, and the message-board seam, instead of calling into `internal/agentos/steward.go`; builds alone `GOWORK=off`; `AssertStandaloneDeps` clean; routed via `runCommandAdapter`. |
| `foreman` | Converted | `yoke/foreman` | Standalone deps and help verified; routed via `runCommandAdapter`; monolith cmds/foreman import removed. |
| `weave` | Converted | `yoke/weave` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `sprint` | Exception | (kept in internal/agentos) | In-process active conductor loop composed with agentos sprint monitor, attachSprintWatch (inbox stream), inbox-ack, and await. |

## Duplicated glue (kept in step by hand)

The operator rule trades duplication for independence. The originals stay in `internal/agentos` because other front-door paths still use them (`wireWebConsole`, the cobra schema projection, inbox turn delivery); when one side changes, change the other.

| Adapter | Duplicated file(s) | Original |
|---|---|---|
| `yoke/kb` | `codering.go`, `index_reader.go`, `recall.go` | `yoke/cmds/graph/codering.go` (+ graph cache helpers), `internal/agentos/index_reader.go`, `kbrecall.go`, `kbmemory.go` |
| `yoke/meet` | `steward_meet.go`, `steward_select.go`, `mb_seam.go`, `main.go` (`wireMeet`) | `internal/agentos/steward_meet.go`, `steward_select.go`, `budget_admission.go`, `messageboard.go`, `agentos.go` (`wireMeet`) |
