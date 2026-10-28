---
id: a4c64d0eb5d3
kind: feature
title: S518.4 builds/klatch signed build recipe (linux server + web bundle)
seq: 2812
status: done
priority: p0
labels:
    - klatch
created: 2026-10-28T05:42:55.118964Z
due: 2026-10-31T00:00:00Z
assignee: claude-sonnet5
sprint: 518
sprint_id: bbe9153b-b299-5be8-8ef8-215ad002deb9
sprint_title: 'Klatch MVP: rebranded Databag chat server + web + federation on one satellite (replaces Matrix), due Sat 10-31'
closed: 2026-10-28T07:25:54.589965Z
closed_by: s518-manager
---

S518.4 builds/klatch: signed build recipe in the bashy fork-build-sign pipeline.

Repo: this workspace is bashy. Read builds/README.md, builds/render.sh, builds/check.sh, builds/_shared/ and the existing Go-service recipes (builds/tuwunel, and Go ones like builds/victoria-*, builds/zot, builds/seaweedfs) and copy the closest pattern. Upstream for Klatch is OUR fork github.com/qiangli/klatch (Apache-2.0, fork of balzack/databag), pinned commit 45343338582b394f4f5fdb4b7e271ea1dc8dd5fd (no upstream release tag fits; pin by commit or by a tag we create later — make the pin a single field in tool.env so the manager can bump it to the rebrand commit).
Build: server = net/server, `CGO_ENABLED=1 go build` (sqlite via gorm sqlite driver needs cgo) for linux amd64 + arm64 (MVP; darwin/windows later); web = app/client/web with node 22 + yarn (`yarn --frozen-lockfile && yarn build`, output dist/) packaged as a separate tarball asset; include net/transform scripts in the server archive. Release assets named klatch-<ver>-linux-<arch>.tar.gz and klatch-web-<ver>.tar.gz, signed like the other tools. No-egress test as the shared pipeline does; cargo/npm audit equivalent: govulncheck (report-only ok) + `yarn audit` report-only.
Do NOT push the rendered branch or tags (manager does). Commit only bashy files.
Verification: `builds/check.sh` passes (paste result) and `builds/render.sh klatch` renders the fork tree; actionlint/shellcheck clean as check.sh requires.

== RULES (all S518 workers) ==
- Your cwd is your isolated workspace (a full git clone). Stay inside it; relative paths only; never follow git remotes. Do not push. Do not switch branches. git add files BY NAME. Keep the bashy-injected BASHY_AGENT and the commit trailers weave gives you.
- Plan of record: umbrella docs/sprint-518-master-execution-plan.md (Klatch = our rebranded fork of Databag, github.com/qiangli/klatch, pinned to upstream main 45343338582b394f4f5fdb4b7e271ea1dc8dd5fd; deadline Sat 2026-10-31 PDT).
- Dragon (this machine) is for editing + short unit tests only: no podman/docker, no full builds of large trees, keep disk use small (delete node_modules you created before finishing).
- Public repos (klatch fork, bashy, yoke): NEVER write real hostnames, IPs, user names or emails into files or commit messages. Hosts come from env/config only.
- Remote Linux access when your story needs it: read `bashy claim list | grep s518` — the host:alien:s518-klatch intent carries the ACCESS line (outpost ssh ...). Use only /opt/s518-klatch on that host, loopback ports 7100/7101 (7000 is taken). Never touch cloudbox, DKS, k3s or the 80/443 front there.
- Credit: Klatch is based on Databag by Roland Osborne (github.com/balzack/databag), Apache-2.0. Keep upstream LICENSE and copyright headers.
- Done = the story's verification passes and is committed. State EXACT measured results in the commit message. If stuck after 3 serious attempts, commit what works plus a BLOCKERS.md with diagnosis, then exit 0. Headless: exit cleanly (exit 0) when done. COMMIT YOUR WORK.
