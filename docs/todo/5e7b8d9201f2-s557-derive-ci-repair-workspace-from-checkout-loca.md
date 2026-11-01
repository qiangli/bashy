---
id: 5e7b8d9201f2
kind: bug
title: S557 derive CI repair workspace from checkout location
seq: 3038
status: done
priority: p1
created: 2026-11-01T17:24:08.102194Z
assignee: s557-manager
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
closed: 2026-11-01T17:26:29.060234Z
closed_by: s557-manager
---

Public CI router and default config assume obsolete legacy checkout. Default root to Bashy parent workspace, respecting explicit DHNT_CI_ROOT; preserve collector/dispatch/secret references and do not run any router or notification. Verify source/default and explicit-root resolution using bounded shell evaluation and syntax checks.
