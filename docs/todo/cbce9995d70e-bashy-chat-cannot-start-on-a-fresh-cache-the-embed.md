---
id: cbce9995d70e
kind: bug
title: 'bashy chat cannot start on a fresh cache: the embedded yoke/chat adapter is extracted without its Go source'
seq: 3321
status: todo
priority: p1
labels:
    - chat
created: 2026-11-26T00:24:32.370429Z
---

Found 2026-11-25 (Sprint 722 Phase B smoke, agent-bench isolated arm HOME).
Symptom: `bashy chat --agent X` under a fresh XDG_CACHE_HOME exits at once with 'go: go.mod file not found in current directory or any parent directory'. Every agent-bench arm (fresh HOME) fails at launch.
Cause (traced with a logging fake go on PATH): chat runs the embedded adapter launcher <cache>/bashy/embedded-adapters/<hash>/yoke/chat/chat.bsh, which runs `go build .` next to itself when <cache>/bashy/adapters/yoke/chat is missing. The extracted directory holds only chat.bsh (no main.go, no go.mod), so the build cannot succeed. Hosts with an old prebuilt adapter in ~/.cache/bashy/adapters never notice. Installed binary: a dev build, 98a460a-dirty. Check whether release builds embed the adapter source.
Classification: product defect (packaging of the embedded adapters), exposed by a realistic environment (a fresh cache).
Workaround used in Sprint 722: BASHY_ADAPTER_CACHE=<a frozen copy of a prebuilt adapters dir>.
Fix: embed the adapter's main.go and go.mod (or ship prebuilt adapters, or run the adapter in-process), plus a fresh-cache regression test that runs `bashy chat --help` with an empty XDG_CACHE_HOME.
