---
id: 1d1b71914ed5
kind: task
title: 'S159.2 bashy: collapse path duplicates onto owners; context.mode and doctor report decisions with the signal'
seq: 258
status: todo
priority: p1
created: 2026-09-12T19:37:40.925484Z
sprint: 159
---

bashy half of S159.2 (umbrella story 080662f5cfe4; design docs/bashy-inspect-design.md section 8, PRIVATE).

Path duplicates collapsed onto their owners: bashySkillsDir -> skills.DefaultStoreDir, execHistDir -> execlog.DefaultRoot, engineCacheDir -> binmgr.CacheDir. Each keeps its name as a thin call so no call site moves; the ladder now lives in exactly one package per store, which is what stops the space graph writer and the graph reader diverging again.

context.mode reports DECISIONS: agent_driven (weavecli.IsAgentDriven) and decided_by (the env var, or the detected tool + the marker variables that identified it) are new fields; advisor is now advisorEnabled() rather than the raw BASHY_ADVISOR value. doctor's agent-mode row says agent-driven and why when a harness drives without BASHY_AGENTIC. agentDrivenSignal is shared by inspect mode, inspect context and inspect doctor so the three cannot disagree.

Gate: TestContextModeReportsDecisions, TestStorePathsResolveThroughOwners; bashy graph space returns rows on this host (pre-fix binary said nothing had been learned; the edges store was 779 KB); make test green; e2e dispatch gate ok.
