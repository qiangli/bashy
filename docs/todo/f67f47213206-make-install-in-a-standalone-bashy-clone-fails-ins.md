---
id: f67f47213206
kind: bug
title: 'make install in a standalone bashy clone fails: installbashy reads man pages from ../sh and ../coreutils'
seq: 414
status: todo
priority: p1
labels:
    - release
    - install
created: 2026-10-08T21:11:09.311987Z
sprint: 379
sprint_id: 908c2ac2-e7bc-55fe-86bd-d25046ac4684
sprint_title: 'bashy 1.0.0 feature list: bash + Bash# + Yoke'
---

Found by the S379 conductor 2026-10-08 installing from a clean worktree at bashy 7c377034 with GOWORK=off: the binary installs, then 'installbashy: install manual pages: read manual source ../sh/docs/man/man1: no such file or directory' (and ../coreutils/docs/man/man1 next), make exits 2. A standalone clone (and the R5 clean-account path) has no siblings. Fix: resolve the man sources from the go.mod-pinned modules (go mod download -json, like scripts/build-meet-spa.sh does for yoke) when the sibling dir is absent; keep the sibling path inside the umbrella. Red/green: tools/installbashy test with no ../sh and ../coreutils installs the pages from module dirs; missing pages in both places still fail loudly.
