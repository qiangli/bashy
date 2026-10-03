---
id: d8fedeb9c544
kind: bug
title: Keep POSIX scheduler daemon reachable in one-file base Bashy
seq: 405
status: assigned
priority: p0
created: 2026-10-03T03:18:02.686432Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

Native Profile D runner invokes staged bashy schedule start/status/stop before TCC for POSIX at/batch/crontab. Base Bashy currently treats schedule as a script path and fails before the awk focused set. Route the existing Coreutils schedule command through base dispatch without AgentOS imports; reserve its name and verify start/status/stop on a private state path.
