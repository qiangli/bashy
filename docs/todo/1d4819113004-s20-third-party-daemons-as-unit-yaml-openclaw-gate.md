---
id: 1d4819113004
kind: feature
title: 'S20 Third-party daemons as unit YAML: OpenClaw gateway and codync-host examples, supervised and exposed with no outpost code'
seq: 1889
status: todo
priority: p2
labels:
    - bashy
created: 2026-10-11T05:07:54.951199Z
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
---

Design docs/bashy-service-design.md: inetd/systemd split - bashy service owns unit definitions and per-unit verbs; bashy supervisord (a bashy command, like inetd) is the service monitor; outpost supervisord (init, like systemd) starts and watches only the outpost daemon and bashy supervisord. Add bashy/examples/fleet/services/ with unit YAML for an OpenClaw gateway and codync-host (download+exec, pinned download records, declared health, expose app and/or mesh, secrets_env for bot tokens). These are the supervised gateways the synergies need (S5 agent as LLM service, S6 messenger input, S11 Codync, S13 mesh reach). Acceptance: bashy service add -f examples/fleet/services/openclaw-gateway.yaml then enable brings the gateway up under bashy supervisord, its health is reported, and it is reachable over the tunnel or mesh as declared; file stamped verified only after a live run.

Operator rod-not-fish requirement 2026-10-10 PDT: implement reusable service declaration/configuration/enablement machinery shared by built-in, internal and external services. Built-ins are seeds, not service-name branches. Internal capabilities retain their subsystem behind a generic runtime provider and use the same registry, desired state, health/status/events and exposure contracts; no separate internal feature-flag path or fake child process. Acceptance includes seeded built-in + internal capability + fresh custom external declaration using the same service verbs, plus an operator override, without per-service supervisor/toggle/exposure code. Agree the S15/S16/S18 provider contract first; preserve complete existing feature coverage and no legacy compatibility/migration.

Conductor verify: inspect pinned download records, run `go test ./internal/agentos/...` in bashy on claimed native venue, then live add/enable/health/exposure tests for both examples and retain gateway logs. Own `examples/fleet/services/` only. A YAML example alone is partial until the live S16 monitor and S18 route work. Commit with `Agent:` trailer.

Continuation 2026-10-11 07:38 UTC: first run #24 preserved source `57c2183` on `origin/agent/sprint418-s20-partial`; both YAML files passed `fleet.ParseService` and `Validate` on the claimed Linux host. The OpenClaw tarball SHA-256 matches the recorded digest, but `download.member: package/openclaw.mjs` extracts only that file while the entrypoint imports sibling package files and the official install provisions Node dependencies. This recipe cannot reach healthy state as written. Use a generic declaration/provisioning mechanism with an exact package and dependency pin, never an OpenClaw-specific supervisor branch. Verify current Node runtime requirements on the venue. Complete the S16/S18 live add/enable/health/exposure gate and retain raw logs before claiming S20. The first run was failed as a spec gap, not accepted.
