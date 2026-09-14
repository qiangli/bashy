---
id: f350696274fd
kind: task
title: 'S168.3 bashy define <ref>: resolve every kind, text + --json; unknown ref exit 1 with vocabulary; define graduates from curatedHiddenVerbs on this gate'
seq: 275
status: todo
priority: p1
created: 2026-09-13T22:56:21.736541Z
sprint: 168
---

S168.3 (L-B in coreutils pkg/lexicon, then B1 here). Plan: dhnt docs/sprint-168-master-execution-plan.md, D4/D5, traps 4+6.
L-B (coreutils pkg/lexicon): define detects a ref via ref.Parse and consults lexicon.RefResolvers (kind -> ref.Resolver registry filled by the embedding shell; lexicon imports NO store). Text: kind id title status where open; --json. Exit 1 for an unresolved REF with the vocabulary + the store listing verb; exit 1 with a DISTINCT message for a vocabulary kind with no resolver wired; bare word and non-vocabulary x:y keep exit 0 unknown here. TestDefineCmd_HasNoSubcommands stays green.
B1 (bashy internal/agentos): wireLexicon() registers all 15 kinds; a test asserts the registry covers ref.Kinds(); define leaves curatedHiddenVerbs AND TestCommandsCatalogSources expected set in ONE commit with the gate output in the body. Announce the push on card #165 first.
