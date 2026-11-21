# Coupled Yoke Verbs Conversion Status and Exceptions (Sprint 677, Story #606)

Tracking the 10 coupled verbs: `todo`, `dag`, `mb`, `inbox`, `kb`, `chat`, `meet`, `foreman`, `sprint`, `weave`.

| Verb | Status | Adapter Entry | Notes / Exception Reason |
|---|---|---|---|
| `todo` | Converted | `yoke/todo` | Landed in 62ee3c74; standalone deps and help verified. |
| `dag` | Converted | `yoke/dag` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `mb` | Converted | `yoke/mb` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `inbox` | Exception | (kept in internal/agentos) | No standalone pkg/inbox in yoke; implemented directly in internal/agentos coupled to in-process session relays and turn delivery. |
| `kb` | Exception | (kept in internal/agentos) | In-process recall.Reader injection in kb context/ask couples to cmds/graph (violates standalone single-command package rule). |
| `chat` | Converted | `yoke/chat` | Standalone deps and help verified; routed via `runCommandAdapter`. |
| `meet` | Exception | (kept in internal/agentos) | In-process wireMeet() callbacks couple to internal/agentos/steward.go for permanent role and secretary activation. |
| `foreman` | Converted | `yoke/foreman` | Standalone deps and help verified; routed via `runCommandAdapter`; monolith cmds/foreman import removed. |
| `weave` | Converted | `yoke/weave` | Standalone deps and help verified; routed via `runCommandAdapter`. |
