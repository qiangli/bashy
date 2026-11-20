---
id: 4c60ebe893cd
kind: bug
title: 'bashy''s in-process git applet rejects common worker forms: commit -q, commit --allow-empty, some -c forms'
seq: 3267
status: todo
priority: p1
created: 2026-11-20T21:02:51.300239Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

Found by muse (S576 1391d568, 2026-11-20): inside a weave-launched shell with bashy shims, 'git' resolves to bashy's in-process git applet; 'git commit -q -m X' fails with pflag 'unknown shorthand flag: q in -q'. Also lacks --allow-empty and refuses some -c forms (TestRunWithCommitsIsNeverClassifiedNoOp red on the dev box for this reason). Real agents type these forms constantly, so workers silently fail. Root cause first; fix in the applet (product), or make the shim fall through to host git for unsupported flags with a hint — never silently drop flags.
