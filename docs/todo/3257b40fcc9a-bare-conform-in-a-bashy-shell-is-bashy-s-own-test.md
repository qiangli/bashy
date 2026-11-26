---
id: 3257b40fcc9a
kind: bug
title: bare conform in a bashy shell is bashy's own test batteries and shadows the dhnt/conform judge binary on PATH
seq: 3323
status: todo
priority: p2
labels:
    - conform
created: 2026-11-26T09:41:44.269666Z
---

Found in the Sprint 723 arm probe (2026-11-26). Shapeware spec section 19 says the conform suite ships with bashy and agents use it while building; a task that names conform run CONTRACT.md reaches bashy's compat/compliance/benchmark verb instead when the agent's shell is bashy (unknown command for run/version). The judge is reachable only as bashy conform. Decide one name: route the judge's subcommands (run, cert, mutate, spec, version) through the bare verb, or rename the batteries verb. Workaround used in the drill: a plain system shell in every arm.
