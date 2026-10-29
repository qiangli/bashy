---
id: 3123a6ccbabc
kind: chore
title: S518.4f Klatch build pin bump to v1.2.5
seq: 2826
status: done
priority: p0
labels:
    - klatch
created: 2026-10-29T04:15:59.047861Z
due: 2026-10-31T00:00:00Z
assignee: claude-sonnet5
sprint: 518
sprint_id: bbe9153b-b299-5be8-8ef8-215ad002deb9
sprint_title: 'Klatch MVP: rebranded Databag chat server + web + federation on one satellite (replaces Matrix), due Sat 10-31'
closed: 2026-10-29T04:19:59.0526Z
closed_by: s518-manager
---

S518.4f Klatch build pin bump to v1.2.5. Repo: bashy. builds/klatch/tool.env: UPSTREAM_TAG=v1.2.5, UPSTREAM_COMMIT=bdc7a7173a1049947b634e3019d7b69c6ae0a991 (annotated tag + published release on github.com/qiangli/klatch). README version note: v1.2.5 = v1.2.4 + mention caret fix. builds/render.sh klatch; builds/check.sh klatch exit 0; commit with trailers + measured result; exit.

== RULES (all S518 workers) ==
- Your cwd is your isolated workspace (a full git clone). Stay inside it; relative paths only; never follow git remotes. Do not push. Do not switch branches. git add files BY NAME. Keep the bashy-injected BASHY_AGENT and the commit trailers weave gives you.
- Plan of record: umbrella docs/sprint-518-master-execution-plan.md (Klatch = our rebranded fork of Databag, github.com/qiangli/klatch, pinned to upstream main 45343338582b394f4f5fdb4b7e271ea1dc8dd5fd; deadline Sat 2026-10-31 PDT).
- Dragon (this machine) is for editing + short unit tests only: no podman/docker, no full builds of large trees, keep disk use small (delete node_modules you created before finishing).
- Public repos (klatch fork, bashy, yoke): NEVER write real hostnames, IPs, user names or emails into files or commit messages. Hosts come from env/config only.
- Remote Linux access when your story needs it: read `bashy claim list | grep s518` — the host:alien:s518-klatch intent carries the ACCESS line (outpost ssh ...). Use only /opt/s518-klatch on that host, loopback ports 7100/7101 (7000 is taken). Never touch cloudbox, DKS, k3s or the 80/443 front there.
- Credit: Klatch is based on Databag by Roland Osborne (github.com/balzack/databag), Apache-2.0. Keep upstream LICENSE and copyright headers.
- Done = the story's verification passes and is committed. State EXACT measured results in the commit message. If stuck after 3 serious attempts, commit what works plus a BLOCKERS.md with diagnosis, then exit 0. Headless: exit cleanly (exit 0) when done. COMMIT YOUR WORK.
