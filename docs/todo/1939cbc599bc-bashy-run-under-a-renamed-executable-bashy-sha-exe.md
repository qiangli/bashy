---
id: 1939cbc599bc
kind: task
title: bashy run under a renamed executable (bashy-<sha>.exe) hands genie/self to the PATH bashy instead of itself
seq: 2838
status: done
priority: p1
created: 2026-10-29T20:06:12.170682Z
assignee: claude-sonnet5.5
sprint: 531
sprint_id: f7bc2570-ca8e-59b0-8163-c3c51ba23610
sprint_title: 'Genie core harness: drive vendor agent CLIs as tools'
closed: 2026-10-29T23:26:45.082813Z
closed_by: s531-manager
---

Found by S531.D1f (codex, ycode docs/sprint-531-d1f-windows-delegate.md) 10-29 on elephant. internal/agentos bashySelfPath() recognizes the running executable only as bashy/bashy.real(.exe) or a test binary, and otherwise falls back to 'bashy' on PATH unless BASHY_SELF is set. So 'C:/.../bashy-81e7.exe genie ...' ran genie on outpost's older bashy.exe: red 'genie doctor --json' .bashy = C:/Users/liqia/AppData/Local/outpost/bashy.exe; green only with BASHY_SELF. A renamed copy of bashy is still bashy: use os.Executable() for the running binary whenever it is a bashy build (e.g. a build-info/embedded marker check), the same on every OS. Red/green unit test; focused go test only.
