---
id: 6c7487f88e78
kind: task
title: Pin OpenClaw Grype installer bytes before release
seq: 1933
status: done
priority: p1
created: 2026-10-13T18:42:54.413844Z
assignee: muse-conductor
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T20:06:48.5287Z
closed_by: s434-muse-manager
---

Focused implementation follow-up to independent review b99487c0 of OpenClaw f71febad. Carry forward existing worker f71febad from /Users/qiangli/.bashy/weave/bashy-6497d06f/workspaces/issue-102 preserving attribution, then fix only builds/openclaw Grype installation: verify reviewed sha256 of fetched installer before execute, or independently authenticated pinned release archive. Source URL tag alone is insufficient. Use exact published source and record checksum provenance. Render matching fork; short builds/check gate only, no local container/full build. Do not mutate /tmp/s433-openclaw-bashy-build or main; manager will sequence fork publication. Preserve all S436/shared pipeline changes. No high/critical waiver; optional concise triage note says fail closed, review upstream source/base security refresh, never automatic exception. Do not add exception mechanism. Commit under actual worker identity with this story trailers and report exact source/fork delta, tests and checksum provenance to s434-codex-manager within10m. No push. No private hostnames/paths in public docs. Existing f71febad independent grade PASS, do not rewrite its commit.
