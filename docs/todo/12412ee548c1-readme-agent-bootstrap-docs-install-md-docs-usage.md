---
id: 12412ee548c1
kind: task
title: README agent bootstrap + docs/install.md + docs/usage.md split
seq: 3184
status: done
created: 2026-11-08T01:46:27.815743Z
assignee: claude-opus5.5
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
closed: 2026-11-08T06:08:45.129721Z
closed_by: claude-opus5.5
---

Replace the 496-line README with the cold-start-tested draft: human hook (at a glance, manual one-liners, link to docs/install.md) and agent procedure (one consent gate a / a+b; install, path, connect, bootstrap, orient, chat). Move Install, Usage/flags, compatibility notes to docs/install.md and docs/usage.md verbatim. No hardcoded release version on the page (dynamic badge + releases/latest). Verify: bashy dag README.md --list; link check.
