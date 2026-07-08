---
name: force-agent-shell
description: 'Verify agentic CLIs run their shell commands through bashy (not the system shell) so the pure-Go userland, the space-time advisor, and OTel apply to everything an agent runs. Use before an unattended fleet run (weave/conductor/meet), after wiring a new agent, or as a convergence gate. Run `bashy skills run force-agent-shell`; exit 0 iff the contract holds. Companion prose kb page force-agent-shell-bashy.'
metadata:
  requires: "has=claude"
  check-wireda: "bashy install-agent claude --check"
  check-foroceda: "[ \"${BASHY_FORCE_AGENT_SHELL:-1}\" != \"0\" ]"
---

# force-agent-shell — attested agent-shell routing check

The runbook counterpart of the kb page `force-agent-shell-bashy`, graduated to
a **dual bundle**: beside this prose sits `skill.dhnt`, the canonical machine
face carrying a content identity, a success contract (`wired ∧ forced`), and a
read-only effect cap. Any Skills-capable tool follows this file as prose; a
dhnt-aware runtime (bashy) executes and *attests* it.

## Use

    bashy skills run force-agent-shell

- The contract is machine-verified — exit 0 iff the anchor agent (claude) is
  wired to bashy (`install-agent claude --check`) **and** launcher forcing is
  enabled (`BASHY_FORCE_AGENT_SHELL` ≠ 0) — and every run appends a re-checkable
  attestation to the host-local store.
- Effect cap is read-only (`efefecato reada`): the skill only inspects wiring;
  it declares no write authority, so the pre-flight audit refuses any variant
  that adds steps without raising the cap.
- Environment-gated (`requires: has=claude`): hosts without the anchor agent are
  never offered this skill.

## Why (the boundary this demonstrates)

`bashy kb` holds *declarative* knowledge as prose (lessons/gotchas/facts/
decisions); dhnt skills hold *procedures* whose value is a **convergence
contract** — the same author-once artifact any executor can run and attest. The
`force-agent-shell-bashy` kb page is a runbook (procedure-shaped) with a
verifiable contract, so it graduates here while the kb page stays the
discoverable prose pointer. Prose → kb; procedures → dhnt skill.

## Forcing (the actions the contract verifies)

Two layers force bashy (see the kb page for the source-verified per-agent
matrix):

1. **Launcher (automatic):** `bashy meet`/`chat`/`weave`/`sdlc` inject
   `PATH=~/.bashy/shims:…`, `SHELL=<bashy>`, `CLAUDE_CODE_SHELL=<bashy>` into
   every spawned agent (covers claude, aider, opencode, agy).
2. **Durable wiring:** `bashy install-agent <agent>` — claude/opencode config
   keys, aider `$SHELL`, gemini/copilot/agy PATH shim, **codex** the login-shell
   `chsh` (codex reads `/etc/passwd`, so the launcher can't route it per-process).

## Bindings

Concrete commands live in this file's `metadata` (`check-wired`, `check-forced`)
— the executor-side half of the dual bundle. Per-host overrides learned by
`run --adapt` are stored beside the skill store, never written back into this file.
