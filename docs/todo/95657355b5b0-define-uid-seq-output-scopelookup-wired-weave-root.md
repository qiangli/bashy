---
id: 95657355b5b0
kind: task
title: 'define: uid/#seq output; scopeLookup wired (weave roots + user); atlas 2.8; e2e-refs kb+todo cases'
seq: 302
status: done
priority: p1
created: 2026-09-16T18:56:59.556974Z
assignee: transom
sprint: 202
closed: 2026-09-17T01:48:52.480092Z
closed_by: transom
---

Sprint #202 S2 (bashy). Depends on S1 (coreutils #148, new pin). Plan: docs/sprint-202-master-execution-plan.md in the umbrella.

- internal/agentos/refs.go: build ONE scopeLookup and pass it to kb and todo RegisterRefs. Repo basename -> root from weave's known queue roots (all queues), the cwd checkout first; two roots sharing one basename = error naming both; "user" -> the personal store. No registry, no new kind.
- bashy define <ref>: text output shows a uid line and a #seq line when the Node carries them; --json carries uid/seq. Unknown scope reports the scope name.
- docs/command-atlas.md 2.8: the entity sentence (an entity is anything with a ref), the three-handle table (seq / uuid / slug, scope), the scope segment rule, and "seq is accepted, never emitted".
- Umbrella script/e2e-refs.sh gains cases: kb by seq, by uuid prefix, by urn:dhnt:kb:<uuid>, by <repo>/<slug> from outside the repo; todo by seq, by slug, by <repo>/<seq> from outside the repo; a uuid-shaped kb slug refused by kb add. GATE GREEN on the new pin (BASHY_BIN=<built bashy>).
- make build && make test; make install on this host (never hand-copy).

Out of scope: emitting scoped refs in sprint show / todo list.
