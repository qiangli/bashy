---
id: aa251db7587b
kind: bug
title: bashy ollama on Windows leaks runner processes (one ~13 GB ycode-runner per interrupted request)
seq: 3183
status: todo
priority: p1
labels:
    - ollama
created: 2026-11-08T01:12:14.171363Z
---

Seen 2026-11-07 on the Windows test host with bashy ollama serve (engines build) as a bashy service unit: requests whose client disconnected (remote exec timeout) left ycode-runner.exe processes alive under exited parents; 7 runners accumulated within 10 minutes (13.3 + 12.2 + 7.6 GiB working sets), after which every load failed 'model requires more system memory (13.2 GiB) than is available (6.5 GiB)'. After a clean restart, two runners were again present after one completed request. Expected: one runner per loaded model, reaped when its server/request ends (Windows job object or explicit kill on parent exit).
