---
id: d7d061f42273
kind: bug
title: 'dragon: bashy supervisord was SIGTERMed and nothing restarted it; orphaned llm held its port'
seq: 3211
status: todo
priority: p1
labels:
    - service
    - supervisord
created: 2026-11-10T15:15:31.76008Z
assignee: claude-sonnet5.5
---

2026-11-10 13:15Z on the dev box: supervisord.log 'killed by signal (status 143)'; all supervised units down (apps /sprint /mb /loom /meet, llm, meet, otel-stack, outpost-lern-si). Restored by hand (nohup bashy supervisord). The llm unit then failed 'port 24556 held by a foreign process': an orphan 'bashy llm serve' (ppid 1, 2 days old) from an earlier supervisor. Many leaked 'outpost-e2e-bin-*/outpost start' test daemons were running too (outpost todo filed). Tasks: (1) find what sent SIGTERM at 13:15Z (weave run logs under ~/.bashy/weave/*/logs around that time, test suites, pkill-style cleanup); (2) make supervisord boot-managed and restarted on failure on macOS through bashy's own service registration (not a hand-written plist), document it; (3) on start, supervisord adopts or replaces its own orphaned children that hold a unit's port (identify by argv/BASHY_HOME) instead of failing; red/green test with a fake orphan. Hosts by role in any public text.
