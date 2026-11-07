---
id: 3d719e5e2f1f
kind: bug
title: Private kb add records this machine's name in source.host; use a generic value
seq: 3181
status: todo
priority: p3
labels:
    - kb
created: 2026-11-07T22:33:26.740427Z
---

Found 2026-11-07 (Sprint 588 retro): bashy kb add in the host ring writes the machine's real hostname into a page's source.host frontmatter, so a page copied or exported from the host kb carries it (pages had to be scrubbed by hand). Expected: record a generic or hashed host identity (or omit it unless asked) so knowledge can be shared without leaking host names; the repo ring already records no host. Fix in yoke pkg/kb with a red/green test.
