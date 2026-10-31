---
name: tutor
description: Isolated Session rehearsal and command/skill parity
type: dag
default: dry-run
---

# Session

The rehearsal plans and demonstrates a small improvement using the default format. The verifier owns a fresh temporary home and git repo for each lane;
all state is removed on exit. No live session accounts or boards are accessed.

## Tasks

### dry-run
Run both lanes in order. Each lane executes plan → reviewed five-step agenda
→ apply → start → done/next → status --line and checks command/skill duality.

```bash
python3 ./verify-duality.py
```

### no-agent
Offline template plan; start reports that an explicit manager is required.
Continue with next/done and verify Step 2/5 while all eight steps stay in JSON.

```bash
python3 ./verify-duality.py --mode none
```

### stub-agent
Stub chat supplies a draft and stub sprint start acknowledges the request.
Verify the fallback 5400-second session clock; no real agent is launched.

```bash
python3 ./verify-duality.py --mode stub
```

### test
Run the focused tutor suite, including isolated registration and both lanes.

```bash
python3 -m unittest discover -s tests -t . -v
```
