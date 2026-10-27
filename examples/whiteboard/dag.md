---
name: whiteboard
description: Native LAN shared whiteboard for a group
type: dag
default: serve
---

# Shared whiteboard

Run from this directory with `bashy dag serve`, or use `bashy awd PATH -- bashy dag serve`.

## Tasks

### serve
Build and start the native foreground board. Ctrl-C stops it; saved boards remain.
Effects: read, write, exec, net

```bash
"$BASHY" --bashsharp ./whiteboard.bsh serve
```

### test
Check facilitator control, isolation, persistence and concurrent drawers.
Effects: read, write, exec

```bash
"$BASHY" --bashsharp ./whiteboard.bsh test
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
"$BASHY" commands show whiteboard
"$BASHY" skill show whiteboard
```
