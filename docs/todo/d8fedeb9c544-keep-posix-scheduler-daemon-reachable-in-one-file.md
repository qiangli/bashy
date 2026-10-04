---
id: d8fedeb9c544
kind: bug
title: Keep POSIX scheduler daemon reachable in one-file base Bashy
seq: 405
status: done
priority: p0
created: 2026-10-03T03:18:02.686432Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-04T07:13:49.853414Z
closed_by: codex-gpt6-sol
---

Native Profile D runner invokes staged bashy schedule start/status/stop before TCC for POSIX at/batch/crontab. Base Bashy currently treats schedule as a script path and fails before the awk focused set. Route the existing Coreutils schedule command through base dispatch without AgentOS imports; reserve its name and verify start/status/stop on a private state path.

## Sprint 355 acceptance evidence 2026-10-04

POSIX schedule daemon base route is merged at 92cae7b; full6 setup and at/batch/crontab sets completed without new failures. Historical raw journals and any pending formal certification decisions are unchanged.
