---
id: 5a0b3473bd86
kind: task
title: Pin and rebuild Bashy with delivered P2C sh head
seq: 198
status: done
priority: p0
created: 2026-09-03T16:25:38.561651Z
assignee: qiangli
sprint: 98
---

Pin only the pushed sh Sprint 98 head 3f763316; preserve unrelated coreutils and other sibling pins. Rebuild bin/bash and bin/bashy, run focused Bash++ CLI tests plus sibling-pin/build gates and Bash86 smoke/authoritative lane as appropriate, commit/push Bashy, then bump umbrella bashy+sh pins.

Pinned exact pushed `sh` head `3f7633163bfec71e0125c53ca4619aa18e397ab3`.
Both binaries rebuild successfully; the sibling-pin gate and focused Bash++
CLI/dialect tests pass. The previously accepted hermetic Bash86 result remains
86/86; this pin changes only opt-in Bash++ syntax/interpreter behavior.
