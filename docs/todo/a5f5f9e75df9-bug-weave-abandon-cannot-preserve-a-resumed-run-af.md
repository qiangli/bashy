---
id: a5f5f9e75df9
kind: task
title: 'Bug: weave abandon cannot preserve a resumed run after its commit is amended'
seq: 238
status: done
priority: p1
created: 2026-09-06T03:40:25.53678Z
sprint: 127
closed: 2026-09-06T04:22:10.115105Z
---

Found while cleaning rejected weave run #5. The original killed run created refs/salvage/abandoned-5; a bounded resume amended the preserved WIP into a reviewed commit. `weave abandon 5 --force --yes` then refused because fetching the rewritten branch tip into the fixed salvage ref was non-fast-forward. A sprint manager cannot clean a superseded workspace through the supported command even though --force promises preservation. Add a regression covering killed/preserved -> resume/amend -> abandon --force, and preserve every distinct tip without destructive ref overwrite. No new flag.
