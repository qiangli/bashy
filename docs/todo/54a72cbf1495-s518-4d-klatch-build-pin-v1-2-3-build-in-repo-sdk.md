---
id: 54a72cbf1495
kind: task
title: S518.4d Klatch build pin v1.2.3 + build in-repo SDK first
seq: 2821
status: done
priority: p0
labels:
    - klatch
created: 2026-10-28T10:33:49.711001Z
due: 2026-10-31T00:00:00Z
assignee: claude-sonnet5
sprint: 518
sprint_id: bbe9153b-b299-5be8-8ef8-215ad002deb9
sprint_title: 'Klatch MVP: rebranded Databag chat server + web + federation on one satellite (replaces Matrix), due Sat 10-31'
closed: 2026-10-28T10:40:45.27923Z
closed_by: s518-manager
---

S518.4d Klatch build: pin v1.2.3 and build the in-repo SDK before the web bundle. Repo: bashy. Klatch v1.2.3 (tag + release on github.com/qiangli/klatch, commit 51a928ea3d940b3fe9f646b663abb0be57e61481) switched app/client/web/package.json to "databag-client-sdk": "file:../../sdk"; app/sdk must be built first (cd app/sdk && yarn --frozen-lockfile && yarn build  -> tsup -> dist/) before cd app/client/web && yarn --frozen-lockfile && yarn build. Update builds/klatch (build.yml web leg, and audit.yml if it installs web deps; include sdk yarn audit report-only if cheap): UPSTREAM_TAG=v1.2.3, UPSTREAM_COMMIT=51a928ea3d940b3fe9f646b663abb0be57e61481; README notes. builds/render.sh klatch; builds/check.sh klatch exit 0. Also do a local proof of the web leg commands on this machine in a temp clone of the fork at that commit (short; delete node_modules/dist after) and state the result. Commit with trailers; exit.

== RULES (all S518 workers) ==
- Your cwd is your isolated workspace (a full git clone). Stay inside it; relative paths only; never follow git remotes. Do not push. Do not switch branches. git add files BY NAME. Keep the bashy-injected BASHY_AGENT and the commit trailers weave gives you.
- Plan of record: umbrella docs/sprint-518-master-execution-plan.md (Klatch = our rebranded fork of Databag, github.com/qiangli/klatch, pinned to upstream main 45343338582b394f4f5fdb4b7e271ea1dc8dd5fd; deadline Sat 2026-10-31 PDT).
- Dragon (this machine) is for editing + short unit tests only: no podman/docker, no full builds of large trees, keep disk use small (delete node_modules you created before finishing).
- Public repos (klatch fork, bashy, yoke): NEVER write real hostnames, IPs, user names or emails into files or commit messages. Hosts come from env/config only.
- Remote Linux access when your story needs it: read `bashy claim list | grep s518` — the host:alien:s518-klatch intent carries the ACCESS line (outpost ssh ...). Use only /opt/s518-klatch on that host, loopback ports 7100/7101 (7000 is taken). Never touch cloudbox, DKS, k3s or the 80/443 front there.
- Credit: Klatch is based on Databag by Roland Osborne (github.com/balzack/databag), Apache-2.0. Keep upstream LICENSE and copyright headers.
- Done = the story's verification passes and is committed. State EXACT measured results in the commit message. If stuck after 3 serious attempts, commit what works plus a BLOCKERS.md with diagnosis, then exit 0. Headless: exit cleanly (exit 0) when done. COMMIT YOUR WORK.
