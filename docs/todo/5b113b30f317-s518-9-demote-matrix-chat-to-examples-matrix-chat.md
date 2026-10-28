---
id: 5b113b30f317
kind: chore
title: S518.9 demote Matrix chat to examples/matrix-chat as a custom registered command
seq: 2817
status: done
priority: p2
labels:
    - klatch
created: 2026-10-28T05:42:56.424506Z
due: 2026-10-31T00:00:00Z
assignee: claude-sonnet5.5
sprint: 518
sprint_id: bbe9153b-b299-5be8-8ef8-215ad002deb9
sprint_title: 'Klatch MVP: rebranded Databag chat server + web + federation on one satellite (replaces Matrix), due Sat 10-31'
closed: 2026-10-28T07:43:33.416928Z
closed_by: s518-manager
---

S518.9 (bashy side) Demote Matrix chat to a self-contained example (custom registered command), since Klatch replaces it as the bashy chat.

Repo: this workspace is bashy. Today: examples/matrix-chat/ (register.sh registers `bashy matrix-chat` via commands add + skill add) depends on the managed tool `tuwunel` (yoke pkg/fleet/baseline/tools/tuwunel.yaml) and the optional unit `matrix-chat` (yoke pkg/fleet/optional/matrix-chat.yaml); skills/bashy/commands/feed-setup.md documents an optional "Self-hosted Matrix chat" step.
Do (bashy repo only; a later yoke run removes the yoke files):
1. Make examples/matrix-chat self-contained: add copies of the tool declaration (tuwunel.yaml) and the service unit (matrix-chat.yaml) under examples/matrix-chat/ and change register.sh so it also registers them (use the existing bashy verbs for adding a custom tool declaration / service unit from a file — e.g. `bashy service add -f <file>`; find the tool-registration equivalent in `bashy tool --help`; if no verb exists for tools, document the manual step instead of inventing one). README: state that Matrix chat is a community example, not part of core; bashy's chat is Klatch (examples/klatch).
2. skills/bashy/commands/feed-setup.md: replace the optional Matrix chat section with a short pointer to examples/matrix-chat (community example) and to Klatch for bashy chat.
3. Do not delete builds/tuwunel (signed fork pipeline stays; note in builds/tuwunel/README.md that it now serves the example only).
4. Do NOT touch S513 story files or other sprints' docs/todo.
Verification: bash -n on scripts; existing tests that reference matrix-chat/feed-setup (grep for them under skills/ and internal/agentos) still pass — run those packages' go tests by import path; paste counts.

== RULES (all S518 workers) ==
- Your cwd is your isolated workspace (a full git clone). Stay inside it; relative paths only; never follow git remotes. Do not push. Do not switch branches. git add files BY NAME. Keep the bashy-injected BASHY_AGENT and the commit trailers weave gives you.
- Plan of record: umbrella docs/sprint-518-master-execution-plan.md (Klatch = our rebranded fork of Databag, github.com/qiangli/klatch, pinned to upstream main 45343338582b394f4f5fdb4b7e271ea1dc8dd5fd; deadline Sat 2026-10-31 PDT).
- Dragon (this machine) is for editing + short unit tests only: no podman/docker, no full builds of large trees, keep disk use small (delete node_modules you created before finishing).
- Public repos (klatch fork, bashy, yoke): NEVER write real hostnames, IPs, user names or emails into files or commit messages. Hosts come from env/config only.
- Remote Linux access when your story needs it: read `bashy claim list | grep s518` — the host:alien:s518-klatch intent carries the ACCESS line (outpost ssh ...). Use only /opt/s518-klatch on that host, loopback ports 7100/7101 (7000 is taken). Never touch cloudbox, DKS, k3s or the 80/443 front there.
- Credit: Klatch is based on Databag by Roland Osborne (github.com/balzack/databag), Apache-2.0. Keep upstream LICENSE and copyright headers.
- Done = the story's verification passes and is committed. State EXACT measured results in the commit message. If stuck after 3 serious attempts, commit what works plus a BLOCKERS.md with diagnosis, then exit 0. Headless: exit cleanly (exit 0) when done. COMMIT YOUR WORK.
