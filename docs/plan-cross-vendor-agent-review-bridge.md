# Plan: cross-vendor agent review bridge

**Status:** proposed implementation plan

**Date:** 2026-08-22

**Scope:** Bashy AgentOS only; independent of Bash/POSIX certification Profiles B and C

## Outcome

Two independently operated agent tools from different vendors can coordinate,
review revisions, return findings, revise, and re-review without a person copying
messages between their user interfaces.

The first concrete deployment is a Codex conductor responsible for one workstream
and a Claude Code conductor responsible for a related workstream. The design must
not encode either vendor into the review protocol. Codex, Claude Code, YCode, GLM,
Agy, OpenCode, and future tools are replaceable execution backends.

The operator performs a one-time installation and identity assignment:

```sh
bashy bridge install codex  --identity profile-b-conductor
bashy bridge install claude --identity profile-c-conductor
bashy bridge doctor
```

After that, Bashy delivers actionable review events directly, resumes an idle
agent when its tool permits it, preserves the complete review record, and only
escalates decisions that actually require human authority.

## Decision

Do not make `meet`, `mb`, `chat`, or `sprint` independently pretend to be the
whole solution. Compose the facilities that already own each concern:

| Facility | Responsibility |
|---|---|
| `sprint` | Workstream ownership, status, dependencies, and continuity |
| bus / `mb` | Durable addressed delivery, subscriptions, cursors, and receipts |
| `chat` session registry | Addressability and steering of Bashy-governed live sessions |
| `weave review` and `pair` | Clean-room verification and executable adversarial evidence |
| `meet` | Deliberation when a finding is disputed or a design decision is genuinely shared |
| **new `bridge`** | Vendor adapter installation, session registration, delivery, wake/resume, and health |
| **new `review`** | Vendor-neutral review object, revision state machine, findings, and approval gate |

`bridge` is infrastructure. `review` is the user-facing collaboration workflow.
Neither introduces a second message store: both use the existing bus and its
sidecar model described in
[`agent-messaging-attention-design.md`](agent-messaging-attention-design.md).

## Why an MCP server alone is insufficient

MCP is the common tool boundary and should expose the same Bashy operations to
each vendor. An MCP server can let a running agent read and write its inbox, but
it cannot universally wake a vendor client after that client has stopped or is
waiting outside an agent turn.

The complete integration therefore has two parts:

```text
vendor plugin: tools + instructions + lifecycle registration
bridge daemon: durable inbox + delivery + wake/resume + retry
```

For a live Bashy-governed session, the subscriber sidecar injects a small delta at
the next turn boundary. For a stopped session, the adapter uses the vendor's
supported resume or non-interactive entry point. For a tool with neither, Bashy
starts a fresh governed turn with the durable Sprint and review context. No
agent is prompted to remember to poll.

## Architecture

```text
 Codex plugin/adapter ----\
 Claude plugin/adapter ----+--> Bashy bridge daemon --> existing durable bus
 YCode adapter ------------/            |                       |
                                         |                       v
                                  session registry       review event log
                                         |                       |
                                         +---- wake/resume <-----+
                                                                 |
                                                        Weave/pair gates
```

### One neutral adapter contract

Every adapter implements the same small interface:

```text
Probe()                         installed, authenticated, capabilities
Register(identity, session)    record a reachable vendor session
Deliver(session, event)        inject at a safe boundary when live
Resume(session, event)         resume a stopped durable session
Start(identity, event)         start a replacement turn when resume is unavailable
Collect(invocation)            normalize result, usage, and terminal status
```

Capabilities are measured rather than inferred:

```json
{
  "live_delivery": true,
  "resume": true,
  "noninteractive": true,
  "mcp": true,
  "lifecycle_hooks": false
}
```

The daemon selects the strongest available delivery path. A missing capability
causes an explicit fallback or a visible `delivery_blocked` state, never a false
success.

### Vendor packages

#### Codex

The one-time installation should use Codex's supported plugin and MCP surfaces:

- a Bashy review skill;
- the Bashy bridge MCP server;
- locally scoped identity and policy configuration;
- session discovery/registration; and
- queued delivery to a live session, with resume/start fallback owned by the
  bridge daemon.

The installed Codex version must be probed at install time. Optional hooks or
app-server features must not be assumed merely because a later Codex release may
support them. The adapter records precisely which delivery capabilities passed
its probe.

#### Claude Code

Package a normal Claude Code plugin:

```text
bashy-bridge/
├── .claude-plugin/plugin.json
├── .mcp.json
├── hooks/hooks.json
└── skills/bashy-review/SKILL.md
```

Use supported lifecycle hooks to register a session at start/resume, publish
terminal state at stop/end, and attach concise pending context. Use the bridge
MCP server for explicit inbox, acknowledgement, and review operations. The
daemon uses the supported session-resume/non-interactive CLI path for inbound
work when no interactive turn is available.

#### Other tools

A new vendor integration supplies only an adapter manifest and the operations
above. It does not define new message types, review semantics, or storage.
Tools that cannot install plugins may be launched through `bashy chat`, whose
governed session registry and control socket provide the generic fallback.

## Installation and lifecycle

### Commands

```sh
bashy bridge install <tool> --identity <name> [--scope user|project]
bashy bridge uninstall <tool> --identity <name>
bashy bridge list
bashy bridge status <identity> [--json]
bashy bridge doctor [<identity>] [--json]
bashy bridge repair <identity>
```

Installation must be transactional and idempotent:

1. Probe the CLI version, authentication readiness, plugin mechanism, MCP
   support, live delivery, and resume support.
2. Show the exact files and vendor configuration entries that will change.
3. Install through the vendor's supported plugin/configuration command where
   one exists.
4. Preserve unrelated user configuration; never replace an entire settings
   file.
5. Start or connect to one user-level bridge daemon.
6. Launch or resume the vendor once to register a session.
7. Send a nonce-bearing loopback message and require its acknowledgement.
8. Persist the measured capability record and installation receipt.

Uninstall removes only entries named in that receipt. Secrets, bearer tokens,
and vendor session identifiers remain in user-local protected state, never in a
repository.

### Project declaration

A repository may commit non-secret intent:

```yaml
# .bashy/bridge.yaml
participants:
  profile-b-conductor:
    tool: codex
    role: reviewer
  profile-c-conductor:
    tool: claude
    role: author

channels:
  - posix-bc-review

policy:
  require_exact_sha: true
  invalidate_on_new_head: true
  author_cannot_approve: true
  max_review_rounds: 5
```

This file declares roles and policy; it does not grant authority or contain a
delivery endpoint.

## Review protocol

### Review identity

A review is pinned to an immutable tuple:

```text
(repository identity, base commit SHA, head commit SHA)
```

Branch names are informational because they move. Any change to `head_sha`
creates a new review revision and invalidates the previous approval. Movement of
the declared base also invalidates the verdict until the change is rebased or
the reviewer explicitly reviews the new comparison.

### Event envelope

All adapters exchange one versioned, vendor-neutral envelope:

```json
{
  "schema": "bashy-review-v1",
  "event_id": "01K...",
  "review_id": "vsc-s74-17",
  "revision": 3,
  "event": "review.requested",
  "from": "profile-c-conductor",
  "to": "profile-b-conductor",
  "repository": "vsc-pcts-harness-kit",
  "base_sha": "332e7ce",
  "head_sha": "2ab2697",
  "scope": ["shared durable launcher", "Profile C artifact validation"],
  "gates": ["./scripts/profile-c-behavior-test.sh"],
  "reply_to": null,
  "created_at": "2026-08-22T00:00:00Z"
}
```

The first event set is intentionally small:

```text
review.requested
review.acknowledged
review.finding
review.changes_requested
review.revised
review.approved
review.disputed
review.superseded
review.delivery_blocked
review.closed
```

Every event is append-only. Corrections supersede earlier events; they do not
rewrite them.

### Findings

A finding records severity, affected path, evidence, and whether it is blocking.
Whenever practical, the evidence is executable: a failing test or probe produced
through `pair`, followed by the declared gate. Prose-only findings remain useful
but are identified as claims rather than proofs.

The reviewer runs in a fresh worktree or clean clone with read-only access to the
author's published revision. If an adversarial pair writes test evidence, that
evidence lives in an isolated review workspace and is returned explicitly; it
does not silently alter the author's branch.

### State machine

```text
requested -> acknowledged -> reviewing
    -> changes_requested -> revised -> reviewing
    -> approved -> closed

reviewing -> disputed -> meet -> reviewing | human_decision
any nonterminal state -> superseded
delivery failure -> delivery_blocked -> retry | human_attention
```

Retries are at-least-once, so `event_id` is the idempotency key. The receiver
must acknowledge the durable event before its cursor advances. Repeated delivery
must never duplicate a finding or launch two reviews for the same revision.

### Human authority

Routine review traffic does not involve the operator. Escalate only when:

- the agents disagree after the configured round limit;
- requested scope crosses an ownership, license, secret, or destructive-action
  boundary;
- the declared gate is missing or was already red before the review;
- an exact revision cannot be obtained or provenance cannot be established;
- a merge or publication requires authority neither agent has; or
- delivery remains blocked after bounded retries and adapter repair.

The bridge never turns a peer message into additional authority. An addressed
message is a request, not authorization.

## CLI workflow

```sh
# Author/conductor creates a review request.
bashy review request \
  --sprint 74 \
  --from profile-c-conductor \
  --to profile-b-conductor \
  --repo vsc-pcts-harness-kit \
  --base 332e7ce \
  --head 2ab2697 \
  --scope 'shared A/B/C durable path' \
  --gate './scripts/profile-c-behavior-test.sh'

# Either agent or the operator can inspect the durable record.
bashy review show vsc-s74-17
bashy review timeline vsc-s74-17

# A new author revision supersedes the prior head and wakes the reviewer.
bashy review revise vsc-s74-17 --head <new-sha>

# Approval applies only to the exact current tuple and recorded gates.
bashy review approve vsc-s74-17 --head <new-sha>
```

Do not add automatic merge in the first release. The owning conductor continues
to use the repository's existing protected merge path, including
`weave pull --require-review` where applicable. A later `bashy review merge` may
be considered only after review approval can be consumed without weakening
repository-specific transfer and mainline policies.

## Delivery and attention policy

Use the urgency tiers already defined for the subscriber sidecar:

| Event | Delivery |
|---|---|
| requested, revised, changes requested | Next safe turn boundary; wake if idle |
| ordinary finding or status | Next turn boundary; never interrupt work |
| revision withdrawn, provenance invalid, scope changed | Direction-changing interrupt where supported |
| discussion/chitchat | Idle surface or Meet room only |

Empty inbox checks add no prompt tokens. A delivered notification contains only
the event summary and review identifier; the agent fetches full findings through
the MCP tool when needed. This preserves the prompt prefix and avoids flooding
the agent's working context.

## Security and isolation

- Prefer a user-owned Unix-domain socket locally. Require authenticated TLS and
  short-lived credentials if the broker is exposed across hosts.
- Bind an identity to its measured vendor session and OS principal. A message's
  `from` field cannot be supplied freely by the calling model.
- Allowlist event types and validate every envelope. Never treat message content
  as a shell command.
- Gate repository and path access through existing Bashy workspace policy.
- Keep author and reviewer identities distinct. Warn on the same model family;
  reject literal self-review.
- Never transmit secrets, proprietary artifacts, raw environment variables, or
  unrestricted transcripts through the review bus.
- Store content digests for attached evidence and verify them before use.
- Bound message size, retries, review rounds, and concurrent wakeups.
- Record delivery, acknowledgement, adapter invocation, gate results, and
  authority decisions in the audit log.
- Preserve vendor permission prompts. Installation of a bridge is not permission
  to bypass a vendor sandbox or approval system.

## Implementation phases

### Phase 0 — freeze the contract

- Define `bashy-review-v1`, adapter capabilities, exit codes, and audit events.
- Add golden fixtures for every event and state transition.
- Decide the review store location by extending the existing bus/board store;
  do not introduce a second database.
- Document identity and authority mapping to existing agent bands and roles.

**Gate:** malformed, replayed, out-of-order, and stale-revision fixtures fail
closed; valid fixtures round-trip byte-stably through JSON output.

### Phase 1 — bridge daemon and generic adapter

- Add the local bridge service, durable delivery queue, acknowledgements,
  bounded retry, and dead-letter reporting.
- Integrate with the live-session registry and subscriber sidecar.
- Implement a fake adapter capable of live, stopped, missing, and failing
  sessions.
- Add `install`, `uninstall`, `list`, `status`, `doctor`, and `repair`.

**Gate:** a fake stopped agent is resumed exactly once; a duplicate event is
acknowledged without duplicate work; daemon restart loses no accepted event.

### Phase 2 — Claude Code adapter

- Build the plugin manifest, MCP declaration, review skill, and lifecycle hooks.
- Register start/resume/end and measured session capabilities.
- Exercise live delivery and stopped-session resume independently.
- Verify uninstall restores the prior configuration without losing unrelated
  settings.

**Gate:** one clean-machine install followed by a CLI restart passes registration,
loopback delivery, acknowledgement, resume, and uninstall tests.

### Phase 3 — Codex adapter

- Build the Codex plugin with the shared MCP server and review skill.
- Integrate session discovery and queued delivery where supported.
- Probe rather than assume hooks, app-server, remote-control, or resume features.
- Supply supervisor-driven start/resume fallback for capabilities not present in
  the installed release.

**Gate:** the same adapter contract suite used for Claude passes without changing
the review protocol or fixtures.

Phases 2 and 3 can proceed in parallel after the Phase 1 contract is green.

### Phase 4 — review workflow and Weave integration

- Add `review request/show/timeline/revise/approve/close`.
- Generalize clean-room `weave review` so it can consume a published external
  branch/SHA, not only a Weave-owned workspace.
- Feed `pair` evidence and gate results into structured findings.
- Invalidate approval automatically when either comparison SHA changes.
- Link review state to Sprint without making Sprint the transport.

**Gate:** the full author-reviewer scenario below succeeds without operator
message relay, and `weave pull --require-review` refuses a stale approval.

### Phase 5 — Meet escalation and additional vendors

- Auto-open or reuse a named Meet room only for disputed findings or shared
  design decisions.
- Add adapters for YCode/Agy/GLM/OpenCode according to measured demand.
- Consider authenticated cross-host relay after the local workflow is reliable.

**Gate:** adding a third vendor requires adapter code and packaging only; no
change to `bashy-review-v1` or the review state machine.

## End-to-end acceptance scenario

1. Register independent Codex and Claude Code conductors once.
2. The author publishes commit `H1` against base `B` and requests review.
3. Without operator input, the reviewer receives the request and acknowledges it.
4. Bashy creates a clean review checkout for `(B,H1)` and runs the baseline and
   declared gates.
5. The reviewer returns one executable blocking finding.
6. The author is resumed automatically, acknowledges it, and publishes `H2`.
7. Bashy marks the `H1` verdict superseded and refuses to apply it to `H2`.
8. The reviewer is resumed, reviews `(B,H2)`, and approves the exact revision.
9. A duplicate delivery produces no duplicate review, test commit, or response.
10. The responsible conductor performs the existing authorized merge procedure.
11. The operator receives one concise completion summary and was never used as a
    copy/paste relay.

Also test these negative cases:

- baseline gate red before either agent acts;
- head or base changes during review;
- author attempts to approve its own revision;
- vendor process exits between delivery and acknowledgement;
- plugin is installed but MCP is unavailable;
- bridge daemon restarts during a pending review;
- peer message contains shell syntax or an out-of-scope path;
- review exceeds its configured round limit; and
- two workstreams share a repository but have disjoint path ownership.

## Observability

Expose both human and machine-readable status:

```sh
bashy bridge status --json
bashy review list --state delivery_blocked --json
bashy review show <id> --json
```

Minimum measurements:

- delivery-to-ack latency by adapter and method;
- live-inject, resume, fresh-start, retry, and failure counts;
- duplicate/replay suppression;
- review rounds and time-to-approved;
- human escalations by reason;
- empty-inbox token delta and non-empty notification token size; and
- stale-approval rejections.

An adapter is not `ready` merely because its executable is on `PATH`. Readiness
requires authentication, plugin/MCP health, a registered delivery path, and a
successful nonce loopback.

## Documentation deliverables

- `bashy bridge --help` and adapter-author guide;
- `bashy review --help` and protocol reference;
- Codex and Claude Code one-time installation recipes;
- threat model and local/cross-host deployment guidance;
- recovery runbook for `delivery_blocked` and stale sessions;
- migration note explaining that `mb`, `meet`, `chat`, and `sprint` remain valid
  primitives with narrower responsibilities; and
- a worked cross-vendor review example using two harmless sample branches.

## Definition of done

The feature is done when two subscription-backed tools from different vendors
can complete the end-to-end acceptance scenario repeatedly, after a one-time
installation, with no copied chat messages and no hidden manual polling. Every
approval is pinned to an exact measured revision, every delivery is durable and
acknowledged, and every expansion of authority remains visible to the operator.
