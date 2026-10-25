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
"$BASHY" --bashsharp ./brainstorm.bsh serve
```

### test
Check privacy, teacher control, persistence and concurrent participants.
Effects: read, write, exec

```bash
"$BASHY" --bashsharp ./brainstorm.bsh test
```

### register
Project the one Bash# file into an ordinary command record and executable skill.
Effects: read, write, exec

```bash
"$BASHY" ./register.sh
```

### show
Read the registered executable body and skill face.
Effects: read

```bash
"$BASHY" commands show brainstorm
"$BASHY" skill show brainstorm
```
