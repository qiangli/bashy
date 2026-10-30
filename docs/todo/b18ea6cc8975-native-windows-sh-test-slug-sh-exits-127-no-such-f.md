---
id: b18ea6cc8975
kind: task
title: 'native Windows: ''sh test_slug.sh'' exits 127 (No such file) while ''sh ./test_slug.sh'' works'
seq: 2840
status: done
priority: p1
created: 2026-10-29T21:54:08.678233Z
assignee: codex-gpt6-astra
sprint: 531
sprint_id: f7bc2570-ca8e-59b0-8163-c3c51ba23610
sprint_title: 'Genie core harness: drive vendor agent CLIs as tools'
closed: 2026-10-29T23:26:47.111821Z
closed_by: s531-manager
---

Found in D3 Windows run 9 10-29 21:39Z, candidate bashy 75908e49 on elephant native Windows, inside genie's guarded envelope (@effects read,write,exec + @guard read,write,exec) in C:/Users/liqia/s531-d3/d3-win. 'sh test_slug.sh' failed twice with exit 127 'No such file or directory' though the file existed; 'sh ./test_slug.sh' worked. POSIX sh opens a relative script operand from cwd (it does not search PATH unless the file is absent and an implementation chooses to). Same on every OS: fix the Windows path (probably the operand resolved against PATH or a drive-relative form). Red/green unit test; focused go test only.
