---
id: 09305802b6c5
kind: bug
title: Bound Prima diagnostic logs without process file-size limit
seq: 1911
status: done
priority: p1
created: 2026-10-13T15:02:28.40987Z
weave: 89
assignee: muse-conductor
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T15:16:31.868606Z
closed_by: s436-codex-manager
---

Hosted candidate -5 run 38418158569 Darwin arm64: ring PASS, single-rank engine dies SIGXFSZ (-25) after 2.2s because exercise.py sets process-wide RLIMIT_FSIZE=1MiB in child; this also constrains Metal/cache/model writes, not just captured stdout. Replace with a bounded log-draining/capture mechanism or periodic file-size monitoring that kills+fails on excess stdout without limiting engine-owned files; keep both-rank fail-closed, bounded deadlines, status evidence. Update generated fork and meaningful tests, commit story file with Sprint/Story/Story-ID trailers. No Dragon native build/container/full suite; no push. Manager will rerun hosted CI.
