---
id: aafea94af0b8
kind: feature
title: 'S583.3 Owner circle: secrets handle-only, secrets.Redactor across stores, TTY-only reveal'
seq: 3134
status: todo
priority: p0
labels:
    - boundary
    - secrets
created: 2026-11-06T21:45:55.628507Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Owner circle part 1: secrets and credentials never leave the owner. Build directly on existing yoke/pkg/secrets (ScrubAgentEnv in firewall.go, Redactor in redact.go, grant.go, exec.go) and yoke/pkg/accounts rather than introducing a duplicate secret engine. Account secrets, tokens, keys (bashy account, secret, token, fleet tokens, satellite/paired-host invites, login) belong to owner. Agents and children use them only by handle (secret://<account>/<name>) or per-invocation account materialization; bashy resolves at the exec boundary. Wire secrets.Redactor across stdout/stderr, transcripts, audit, exechist, mb/meet posts, kb/memory writes, handoff artifacts and feed so injected/known values never persist. Gate plaintext reveal verbs (secret get/env/export) behind the interactive TTY human approve gate (bashy/internal/agentos/approve_src.go); non-TTY or agent reveal refused. Each account in yoke/pkg/accounts binds the circles it may serve (consumed by S583.10).
Depends on: S583.2.
Acceptance: child process authenticates via handle while value never appears in any captured surface; agent reveal and non-TTY reveal refused; tests under temp BASHY_HOME.
