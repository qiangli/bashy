---
name: brainstorm
description: Native LAN brainstorming for a classroom
type: dag
default: serve
---

# Classroom brainstorming

Run from this directory with `bashy dag serve`, or use `bashy awd PATH -- bashy dag serve`.

## Tasks

### serve
Build and start the native foreground board. Ctrl-C stops it; saved data remains.
Effects: read, write, exec, net

```bash
"$BASHY" ./brainstorm.sh serve
```

### test
Check privacy, teacher control, persistence and concurrent participants.
Effects: read, write, exec

```bash
"$BASHY" ./brainstorm.sh test
```
