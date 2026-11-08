---
id: e97a9a5d792f
kind: task
title: install-agent muse does not route muse's shell through bashy
seq: 3194
status: todo
created: 2026-11-08T02:32:03.157812Z
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
---

After 'bashy install-agent muse' (shims in ~/.bashy/shims), a new muse session's shell tool still runs /bin/sh ($0=/bin/sh, $BASH empty). claude (CLAUDE_CODE_SHELL), codex and agy were routed. Find muse's shell setting and wire it; extend --check to detect this. Cold start 2026-11-08, muse 1.4.3.
