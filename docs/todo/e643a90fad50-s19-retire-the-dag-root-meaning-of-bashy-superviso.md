---
id: e643a90fad50
kind: chore
title: 'S19 Retire the dag-root meaning of bashy supervisord: dag roots become units; scratch image entrypoint becomes bashy supervisord --pid1'
seq: 1888
status: done
priority: p2
labels:
    - bashy
created: 2026-10-11T05:07:52.624501Z
weave: 27
assignee: codex-gpt6-sol
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
closed: 2026-10-11T09:06:18.457145Z
resolution: fixed
closed_by: codex-gpt6-sol
---

Design docs/bashy-service-design.md: inetd/systemd split - bashy service owns unit definitions and per-unit verbs; bashy supervisord (a bashy command, like inetd) is the service monitor; outpost supervisord (init, like systemd) starts and watches only the outpost daemon and bashy supervisord. bashy supervisord DAG.md TARGET is the FROM-scratch image PID 1 today. After S16 the command is the service monitor; a dag root is a unit whose exec is bashy dag DAG.md TARGET. Update bashy/scripts/verify-bashy-scratch.sh (entrypoint bashy supervisord --pid1 with a unit for the root), bashy/docs/plan-supervisord.md, plan-bashy-scratch-runtime.md, airgap-image.md, the commands table and atlas, and the e2e/shim tests. The restart-with-bounded-backoff, TERM/INT forwarding and PID1 reaping contract is kept as unit semantics and proven by porting the existing supervisord tests. Acceptance: the old DAG.md TARGET form is refused with a pointer to the unit form; verify-bashy-scratch passes in the FROM-scratch image on the Linux venue with the new entrypoint; ported tests green.
