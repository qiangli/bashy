---
id: 95657355b5b0
kind: task
title: 'define: uid/#seq output; scopeLookup wired (weave roots + user); atlas 2.8; e2e-refs kb+todo cases'
seq: 302
status: todo
priority: p1
created: 2026-09-16T18:56:59.556974Z
sprint: 202
---

Sprint #202 story 3 (bashy). Depends on coreutils stories 1 + 2 (new pin).

- internal/agentos/refs.go: build ONE scopeLookup and pass it to kb and todo RegisterRefs. Repo name -> root from weave's known queue roots (weaveQueue.Root over all queues), cwd checkout first; two roots sharing one basename = error naming both (plan-168 D7 rule); "user" -> the personal store. No registry, no new kind.
- bashy define <ref>: text output shows a uid line and a #seq line when the Node carries them; --json carries uid/seq. Unknown scope reports the scope name.
- docs/command-atlas.md section 2.8 (the public ref contract): the entity sentence (an entity is anything with a ref), the three-handle table (seq / uuid / slug, scope), the scope segment rule, and "seq is accepted, never emitted".
- Umbrella script/e2e-refs.sh: add cases — kb by seq, by uuid prefix, by urn:dhnt:kb:<uuid>, by <repo>/<slug> from outside the repo; todo by seq, by slug, by <repo>/<seq> from outside the repo; a uuid-shaped kb slug is refused by kb add. GATE GREEN on the new pins (BASHY_BIN=<built bashy>).
- make build && make test; rebuild + install on this host (make install, never hand-copy).

Out of scope: emitting scoped refs in sprint show / todo list (separate story later).
