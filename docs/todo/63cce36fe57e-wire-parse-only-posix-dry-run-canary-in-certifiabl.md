---
id: 63cce36fe57e
kind: bug
title: Wire parse-only POSIX dry-run canary in certifiable Bashy base
seq: 404
status: assigned
priority: p0
created: 2026-10-03T03:08:42.749674Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

Base profile excludes AgentOS but certification staging requires bashy --dry-run --posix -c. Register Bashy-only flags and CLI no-exec/strict-POSIX hooks in the base entry point; verify no command side effects and no optional import.
