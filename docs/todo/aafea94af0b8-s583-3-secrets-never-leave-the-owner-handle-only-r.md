---
id: aafea94af0b8
kind: feature
title: 'S583.3 Secrets never leave the owner: handle-only, redact at egress, TTY reveal'
seq: 3134
status: done
priority: p0
labels:
    - boundary
    - secrets
created: 2026-11-06T21:45:55.628507Z
assignee: opencode-glm-5.3
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control (KISS): owner by default, circles from the member list, checked where data leaves'
closed: 2026-11-10T06:08:42.525385Z
closed_by: claude-opus5.5-s606
---

Secrets never leave the owner. KISS rescope 2026-11-09. Depends on: S583.1.
Build only on what exists: yoke/pkg/secrets ScrubAgentEnv (firewall.go), Redactor (redact.go), handle use (secret://), and the TTY approve gate (bashy/internal/agentos/approve_src.go).
- Verify, then close gaps only: (1) agents and children get secrets by handle, never plaintext env; (2) the Redactor runs on each member-facing egress point in the S583.1 inventory that exists today (LLM door responses/logs, cross-host mb/bus posts, handoff, feed); (3) plaintext reveal verbs refuse non-TTY and agent callers.
- No new secret engine.
Acceptance: a secret used by handle never appears in any captured egress surface; agent and non-TTY reveal refused; tests under a temporary BASHY_HOME.
