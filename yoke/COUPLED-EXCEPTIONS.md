# Coupled Yoke Verbs Conversion Status and Exceptions (Sprint 677, Story #606)

Tracking the 10 coupled verbs: `todo`, `dag`, `mb`, `inbox`, `kb`, `chat`, `meet`, `foreman`, `sprint`, `weave`.

| Verb | Status | Adapter Entry | Notes / Exception Reason |
|---|---|---|---|
| `todo` | Converted | `yoke/todo` | Landed in 62ee3c74; standalone deps and help verified. |
| `dag` | Converted | `yoke/dag` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `mb` | Converted | `yoke/mb` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `inbox` | Exception | (kept in internal/agentos) | No standalone pkg/inbox in yoke; implemented directly in internal/agentos coupled to in-process session relays and turn delivery. |
