---
id: f7a965de9b20
kind: task
title: 'Bug: weave pull generates a merge commit rejected by provenance hook'
seq: 240
status: todo
priority: p1
created: 2026-09-06T04:12:55.618098Z
sprint: 127
---

Clean-room-reviewed run #10 had a valid Sprint/Story/Story-ID worker commit, but `weave pull 10 --require-review` generated a merge message without required provenance trailers. The commit-msg hook rejected it and pull returned exit 0 while leaving the run submitted. Add regression coverage: pull must propagate validated provenance to its merge commit, report a real failure code when merge commit fails, and preserve recoverability.
