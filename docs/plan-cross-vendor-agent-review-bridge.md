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
  "review_id": "a campaign droplet",
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
bashy review show a campaign droplet
bashy review timeline a campaign droplet

# A new author revision supersedes the prior head and wakes the reviewer.
bashy review revise a campaign droplet --head <new-sha>

# Approval applies only to the exact current tuple and recorded gates.
bashy review approve a campaign droplet --head <new-sha>
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

---

# Review and recommendations — Profile C conductor, 2026-08-22

Verdict: **right problem, right instincts, wrong size — and it rests on a
component that has never run.** Keep the review object, the SHA pinning, the
authority rules and the adapter contract. Cut the rest of v1 hard.

## What is right and should survive any rewrite

- **SHA-pinned review identity** `(repo, base_sha, head_sha)`, approval void on
  any head movement. Strongest idea in the document, and it is exactly the
  failure we hit in sprint #74: a stale `.vsc-phase-gate` approval, and a
  `provision.revision` that got re-stamped instead of recorded. "An approval
  names an exact revision or it is void" is worth building on its own merits.
- **Composition over a new god-verb** — `sprint` owns ownership, bus owns
  delivery, `meet` owns dispute.
- **Capabilities measured, never inferred**, with `delivery_blocked` instead of
  a false success. This matches the fleet evidence invariant: no success state
  may be reached by the ABSENCE of evidence.
- **Author cannot approve; a message is a request, not authority; no automatic
  merge in v1.** Keep all three.
- **Append-only events; supersede rather than rewrite.**

## Blocking 1 — the daemon contradicts the decision of record

`docs/agent-reachability-im-layer.md` line 5 records the taken decision: sidecar
model **(a)+(c)**, *"No host daemon."* Option (b), an always-on host daemon, is
explicitly **deferred to the cross-host phase** where it may earn its keep. Line
271 names the property being protected: *"there is no daemon to be down."*

This plan introduces a bridge daemon as core infrastructure for the **local,
same-host** case — precisely the case that decision excluded — without citing or
overturning it.

**Recommendation:** no daemon in v1. Either ride the sidecar, or amend that doc
with an explicit argument for why local delivery now needs a daemon.

## Blocking 2 — the delivery layer's one required component has never run

Measured on this host while writing this review:

| Measurement | Value |
|---|---|
| `bashy bus subscriptions` | **42** |
| `bus sidecar` processes running | **0** |
| `bus sidecar --adopt` | **does not exist** |
| `bashy bus pending` | `no notifications` |

The reachability doc's "what is genuinely missing" section names exactly this —
the sidecar and the self-join, with `bus sidecar --adopt` as the R0 item. It is
still unbuilt. The situation has moved from *0 subscriptions* to **42 standing
subscriptions with nothing holding them**: registered intent, no reader.

The current bottleneck is therefore not protocol expressiveness. A daemon, two
verbs, a ten-event protocol and two vendor plugins stacked on a delivery path
that has never delivered will reproduce the `meet tell` failure one level up —
exit 0 into a channel nobody reads.

**Prerequisite before anything else:** ship `bus sidecar --adopt`, attach it to a
live session, and prove exactly one real notification lands in that session's
input.

## Blocking 3 — live delivery does not apply to the first deployment

The reachability doc, line 198: push works for **bashy-launched sessions only**.
Neither conductor in the motivating example is bashy-launched — Codex and Claude
Code are started by their own CLIs. So in the first concrete deployment the
"inject at next turn boundary" path is unavailable and every event degrades to
resume-or-start: the bridge spawns a **new turn** instead of reaching the running
one.

That is materially different from what the architecture diagram implies, with
real failure modes — two turns racing, duplicated cost, loss of in-session
context. State it plainly, and either require both conductors to run under
`bashy chat` (governed sessions, already listed as the generic fallback) or
accept that v1 is resume-based.

## Should change — two new verbs against an explicit lean mandate

`docs/orchestration-verb-consolidation-audit.md`: `GroupOrch` is already **28
verbs, 17 with no §2.2a justification**, nearly 3× the next group, against a
stated goal that bashy stay lean. This plan adds two more.

- **`review` earns its place** — a genuinely new noun with its own state machine
  and lifetime; nothing else models it.
- **`bridge` does not, yet.** `herald` already exists, already carries one of the
  few *substantial* §2.2a justifications in the audit (11 lines), and already
  owns peer reach: `add`, `list`, `rm`, `discover`, `send` (delegate a task to a
  peer **and gate the result**), `acp` (serve this machine as an ACP agent).
  "Install a vendor adapter, register a session, check health" is the same
  concern. Prefer `herald install|status|doctor|repair` over a 29th verb.

## Should change — phasing is inverted against risk

Phase 0 freezes `bashy-review-v1` with ten event types and golden fixtures for
every transition before one real message has crossed between two vendors. The
riskiest unknowns are whether an agent can be reached at all and whether the loop
is useful. Freezing the schema first optimises the cheap part.

## Recommended v1 cut

One to two days. No daemon, no plugins, no MCP, no new top-level verb:

1. `bus sidecar --adopt`, plus proof that one notification lands in a live
   session.
2. A file-backed, append-only review log in the repo keyed by
   `(repo, base_sha, head_sha)`, with `request`, `finding`, `revise`, `approve`,
   `supersede`. **Four events, not ten.**
3. Each conductor reads its inbox at **turn start** — no wake, no resume. The
   operator still says "go", but no longer carries content. That removes most of
   the real cost for a small fraction of the build.
4. Approval hard-bound to `head_sha`; any movement voids it automatically.

Run the sprint #74 B/C review through that, then decide whether wake/resume, a
daemon, MCP and vendor plugins are worth it — with measured evidence of where the
remaining friction actually is.

## Smaller notes

- **Same-vendor must remain legal.** Both conductors here may be Claude with
  distinct identities. "Warn on same model family" is fine; do not let it harden
  into a same-vendor block.
- `max_review_rounds: 5` — exhaustion should escalate with the full timeline,
  never silently close.
- "Prose-only findings are claims rather than proofs" is a good distinction —
  make it a **required field** on the finding, not a convention.
- The negative-test list is strong. Add: **reviewer approves a revision it never
  fetched** — the approval path must prove the reviewer read the exact SHA it is
  approving.
- Digest the **gate script** as well as the evidence, or an approval can be
  replayed against a mutated gate.

## Bottom line

Ship the sidecar first. It is the one thing everything else assumes and the one
thing that has never run.

---

# Counter-proposal — Profile C conductor

## The reframe

**What we are doing is not chat. It is delegated review with a deliverable.**

The plan models the problem as instant messaging: presence, a live session, an
inbox, wake/resume, a daemon that reaches a running foreign process. That is why
it needs five phases — it is fighting a property it cannot change.

The property: **two independent third-party interactive CLIs never yield turn
control to an outside process while idle.** Nothing bashy does can make an idle
Codex REPL read a message. That is not a bashy gap; it is what "independent
third-party tool" means. Every mechanism in the plan — daemon, plugin, lifecycle
hook, resume fallback, start fallback — exists to work around it.

But a review does not need the reviewer to be live. A review is:

```
(immutable revision, scope, declared gate)  ->  findings
```

That is a **task with an artifact**, not a conversation. And a *fresh* reviewer
is strictly better for review than a live one: clean context, no contamination
from its own workstream, reproducible, and structurally unable to write to the
author's branch.

## Measured: the primitive already exists and already works

From inside a Claude Code session, with no new bashy code, no plugin, no MCP
server, no daemon:

```sh
bashy invoke --agent codex-gpt-5.5 --read-only --role reviewer \
  --task 'review' --instruction '<brief>' --json
```

Result: Codex launched headless, executed, answered, returned a structured
envelope. **8.3 s wall clock.** `--read-only` strips the write authority rather
than asking to keep it, so it passes the launch guard on an uncontained host —
an agent that only has to ANSWER needs no filesystem write.

The gated variant also already exists:

```sh
bashy herald send <peer> '<prompt>' --gate './scripts/profile-c-behavior-test.sh'
```

Exit 0 only when the gate passed; a peer's own "completed" is a claim, not
evidence.

Note the direction: in sprint #74 the Profile C conductor is the **author** and
Codex is the **reviewer**. Author-invokes-reviewer is exactly the direction that
works today.

## Proposal

### Level 0 — today, zero code

The author calls `bashy invoke --read-only --role reviewer` with the diff and
the declared gate. No operator relay for the review direction at all.

### Level 1 — the one thing worth building (~half a day)

A thin `bashy review` that wraps Level 0 and adds only what the primitive lacks:

- pins `(repo, base_sha, head_sha)` and builds the brief from the real diff;
- calls the existing invoke/herald primitive read-only;
- appends findings to an **append-only log in the repo**, keyed by that tuple;
- **voids approval automatically when `head_sha` moves.**

That is one new noun, no transport, no store, no daemon. It keeps the single
best idea in the original plan — SHA-pinned approval that cannot survive a
rebase — and drops everything that exists to reach a live process.

### Level 2 — only on measured need

The reverse direction (a reviewer initiating against the author) uses the same
primitive; the reviewer's harness calls bashy, because bashy is just a CLI.

For the author to *receive* asynchronously, the cheapest honest mechanism is:
the reviewer appends to the shared log, and the author reads it at **turn
start** (one line in the project instructions). The operator types "go" and
carries no content. That removes the clipboard without any presence machinery.

## What I would drop from the plan

The bridge daemon, the vendor plugins, the MCP server, wake/resume, the session
registry, presence tiers, and the ten-event protocol. Every one of them exists
to reach a live foreign session, which the review workflow does not require.

Also drop `bridge` as a verb: with delegation handled by `invoke`/`herald`,
there is nothing left for it to own.

## The honest limit

True real-time bidirectional conversation between two **live** third-party
sessions — where either can interrupt the other mid-work — is not achievable
while both are launched by their own vendor CLIs. Someone must own the turn
loop. If that is genuinely wanted, the answer is not a bridge: it is launching
both conductors under `bashy chat`, where they become governed sessions and the
existing sidecar/control-socket path applies. Between tools you started
yourself, no design fixes it.

For review and collaboration, that limit does not bind — which is why the
delegated-task shape is the right one.

---

# Revised proposal — knowledge transfer is a READ problem, not a messaging problem

The operator's framing is the correct generalisation: **instant knowledge
transfer from one live agent of one vendor to an agent of another vendor.** This
is a common power-user situation — several agents, one or many vendors, each
holding working knowledge the others need. The review case is one instance.

The earlier counter-proposal (harvest the knowledge into a hand-written brief)
is a workaround: manual, lossy, and after the fact. The bridge plan is the other
extreme: it treats the need as messaging, which forces it to reach a **live**
foreign process, which forces the daemon, the plugins, the hooks and the
wake/resume ladder.

Both miss the same fact.

## The fact

**Every mainstream agent CLI already writes a structured transcript to local
disk.** Measured on this host:

| Vendor | Store | Shape |
|---|---|---|
| Claude Code | `~/.claude/projects/<slug-of-cwd>/<session-uuid>.jsonl` | 66 sessions for this cwd alone; records typed `user` / `assistant` / `system`, each carrying `sessionId`, `timestamp`, `cwd`. Current session: 1,888 lines / 3.0 MB |
| Codex | `~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<session-id>.jsonl` | 1,583 files; a `session_meta` record carries `session_id`, `cwd`, `originator`, `cli_version`; then `event_msg`, `response_item`, `turn_context`, `world_state`. Plus `~/.codex/history.jsonl` |

Both are append-only JSONL, timestamped, and tagged with the working directory —
which means they are **joinable on repository and time** without either vendor
cooperating.

So the transfer is a **read**, not a delivery. That single change removes every
hard constraint in the bridge design:

| | bridge (messaging) | transfer (read) |
|---|---|---|
| source agent must be live | **yes** | no |
| vendor cooperation | plugin + MCP + lifecycle hooks | none — read what it already writes |
| new daemon | yes | no |
| works after the source finished or compacted | no | **yes** |
| per-vendor cost | an installed integration | a parser (~100 lines) |
| artifact | event log | file with provenance + digest |

The row that matters most is the fourth. Knowledge transfer is most needed
exactly when the source session is *gone* — and that is precisely when a
messaging bridge can do nothing.

## Proposed surface

Small, and it extends a decision already taken rather than inventing a lane:

```sh
bashy sessions ls [--cwd .] [--tool codex|claude|…] [--since 2d]   # cross-vendor index
bashy sessions show <id> [--tail N]

bashy transfer <id|selector> --about '<topic>' \
    [--paths scripts/] [--since 3d] [--redact] [--json]
```

`transfer` emits a primeable brief with a provenance header — source vendor,
session id, cwd, time range, line range, content digest — so the receiving agent
can cite where its knowledge came from, and a reviewer can audit it.

## This is already the plan of record, one layer down

`docs/lifetime-context-design-2026-08.md` decided **six stores → three**:
`sessions` (a verbatim mirror), `kb` (byte-capped), and `handoff`. It also found
that harness transcripts are *"the only record carrying agent REASONING and
command OUTPUT"* and are **deleted on a 30-day timer**.

So a session-reading transfer verb is not a new lane. It is that workstream's
payoff, and it simultaneously rescues the store that analysis identified as the
most valuable and the most at risk.

## Honest limits

1. **Pull, not push.** "Instant" means *on demand at the receiving agent's turn
   start*, not an interrupt into a running turn. Reaching a live foreign session
   remains impossible. The trade is strictly favourable: the source need not be
   awake, installed, cooperating, or even still running.
2. **Distillation costs a model pass** over a large file (3 MB for one session).
   Scope by topic, path and time; cache by `(session, topic, digest)`.
3. **Secrets are mandatory to handle, not optional.** Transcripts contain command
   output. `secret-output-firewall.md` already says match secret **values**, not
   names; `coreutils/pkg/redact` exists. Redaction on by default, refuse to emit
   on redaction failure.
4. **Vendor formats drift.** Probe the format version and fail loudly; never
   silently half-parse a transcript, which would transfer a confidently
   incomplete picture.
5. **It transfers what an agent SAID, not what it would say next.** It does not
   replace the source agent's judgement on new material — it removes the human
   from moving what already exists.

## v1

Two parsers, `sessions ls`, `transfer --about`, redaction, provenance header.
Days, not phases. No daemon, no plugin, no MCP, no protocol freeze.

It can be validated immediately against the case that motivated it: pull the
Profile B conductor's reasoning about the harness durable path out of its own
Codex transcript, with no clipboard and no cooperation from that session.

---

# Correction — peer collaboration is not subordinate orchestration

`meet`, `chat`, `consult`, `invoke`, `herald send` all share one property: **the
caller launches the other agent.** `--participant codex-gpt-5.5` spawns a codex
process owned by the caller — no independent session, no own operator, no
history of its own. That is *control*, and it is already solved.

The requirement here is different and harder: **two peers, each independently
operated inside its own vendor CLI, each with its own accumulated session,
collaborating as equals after a one-time connection.** Neither launched the
other. Neither may control the other.

## The invariant

> **Whoever owns the turn loop owns the agent.**

A subordinate's turn is owned by its invoker, so it is trivially schedulable. A
peer owns its own turn loop, so **nothing outside it can schedule it.** No
daemon, bus, plugin, socket or MCP server changes that — they can all deposit a
message, and none can cause a turn to happen.

This is why the bridge plan escalates to a daemon with wake/resume/start
fallbacks: it correctly saw that peers cannot be reached, and tried to solve it
by **force** — reach in and make a turn happen. Against an interactive REPL
owned by another vendor, that ladder ends in "start a replacement turn", which
is no longer the peer at all. It is a subordinate wearing the peer's name, and
it has none of the accumulated context that made the peer valuable.

The alternative is to solve it by **consent**: each peer reaches *out*, from
inside its own loop.

## What that makes bashy responsible for

Not the transport, and emphatically not the peer's turn loop. Two things:

1. **A shared channel with conversation semantics** — participants, whose turn
   it is, open/closed, convergence and termination conditions, an append-only
   transcript, and bounded rounds. This is a *rendezvous with a protocol*, not a
   message queue. The dhnt graph work already names the property: for agents
   with no `select()`, the rendezvous must ARRIVE rather than be visited — but
   for a peer, "arrive" can only mean "be present when the peer next looks".
2. **A tiny per-vendor opt-in shim that runs inside the peer's own loop** — it
   checks the channel at the peer's own turn boundary and yields a slice to the
   collaboration. Not a daemon that reaches in; a few lines the peer's harness
   already runs.

"Connect once" is then literal and honest: on each side, the operator starts the
participant loop **once**. After that the two peers drive themselves, meet in
the channel, and converge without a human moving content.

## Per-vendor opt-in must be probed, never assumed

The shim's mechanism differs per vendor and is the only vendor-specific part:

- a supported turn-boundary hook that can continue rather than stop;
- a self-scheduled wakeup the session sets for itself; or
- failing both, a foreground loop the operator starts once.

Each is *consent* — the peer's own machinery choosing to look. None requires the
other side to hold a socket open, and none can be imposed by an external daemon.
Probe what the installed release supports and degrade visibly; a peer that has
stopped looping is simply unreachable, which is the correct and safe outcome
rather than a failure to paper over.

## Honest consequences

- **Latency is the peer's turn cadence**, not the network's. "Instant" means
  "at the other side's next boundary".
- **A peer that stops looping is unreachable.** That is a property, not a bug —
  nothing should be able to compel someone else's agent to run.
- **Rounds and spend must be bounded on each side independently**, because
  neither side can stop the other.
- **This is the only shape that preserves the thing that made the peer worth
  talking to** — its accumulated session context. Force-based delivery
  ultimately replaces the peer with a fresh subordinate and loses exactly that.

---

# The missing feature, and the hook that makes it a chat

## Measured gap

`bashy chat sessions` on this host returns exactly one row:

```
shell:claude:66321   claude   -   shell   66321   log-only   2026-08-12
  a row marked `log-only` has no control socket ... it was launched on a path
  that never opened one.
```

It cannot see the live Claude Code session (`397f4dc0…`) or the live Codex
session (`01a02ab3…`). **bashy's registry only knows sessions bashy launched;**
a foreign session is invisible, or at best observable and unsteerable. That is
the missing feature, stated precisely — not "no transport", but *no notion of an
externally-owned session as a first-class participant*.

## Hooks are the right bridge, and one hook is the crux

A vendor hook is *consent*: the peer's own harness runs it, at the peer's own
boundary. That is the only thing that can yield a turn slice without an outside
party controlling the session.

But note which hook does the real work:

| Hook | Job | Without it |
|---|---|---|
| `SessionStart` | `bashy peer join <room> --as <identity> --session <sid>` — self-register a foreign session | bashy cannot address the peer at all |
| `UserPromptSubmit` | drain inbound peer messages into context (no-op when empty) | the human must paste |
| **`Stop`** | **the turn-loop yield point** — if peer work is pending and rounds remain, block-and-continue with it | you have a **mailbox, not a chat** |
| `SessionEnd` | `bashy peer leave` — mark unreachable | stale peers look reachable |

`SessionStart` + `UserPromptSubmit` alone give asynchronous messaging where the
human still supplies every turn — better than a clipboard, but still one
push/one pull per prompt.

**`Stop` is what makes it a conversation.** It is the one supported point where a
session can decide "I am not finished — a peer is waiting", and continue on its
own. Two peers each running that hook produce a sustained loop that neither
controls and no daemon drives.

## What bashy must add — and it is small

Nothing that reaches into a peer. Only what a peer's hook can *call*:

```sh
bashy peer join <room> --as <identity> --session <sid> --tool claude|codex
bashy peer leave <identity>
bashy peer list [--room R]              # foreign sessions, self-registered
bashy peer send <peer> --room R <text|file>
bashy peer inbox --as <identity> --drain --json
bashy peer status --room R              # whose turn, round N/M, open|converged|closed
```

Plus room semantics the CLI enforces: participants, **whose turn it is**, an
append-only transcript, a round cap, a convergence/termination predicate, and a
per-peer budget.

The registry change is the load-bearing one: it must accept **self-registration
from a session bashy did not launch**, which is exactly what `log-only` cannot
do today.

## Why this is not the bridge plan

No daemon, no wake, no resume, no start-fallback, no MCP requirement. bashy never
reaches into a peer; each peer reaches out at its own boundary. The plugin is a
*client of bashy*, not an agent of a bashy daemon. A peer that stops looping is
simply unreachable — correct and safe, not a failure to retry around.

## Bounds this design must carry

1. **Ping-pong.** Two `Stop` hooks can continue each other indefinitely. A hard
   round cap and spend budget must be enforced **locally on each side**, because
   neither side can stop the other. The room's termination predicate is
   advisory; the local cap is authoritative.
2. **Cost.** Every continue is a real billed turn on both sides.
3. **Autonomy is visible.** A `Stop`-continue makes a session act with no human
   prompt. It must be explicit opt-in per room, and surface "peer round N/M".
4. **Secrets.** Peer text enters context; redact on send, not on read.
5. **Per-vendor probe, never assume.** Claude Code's hook surface is documented
   and currently unused on this host. Codex has `plugins/`, `skills/`, `rules/`
   and a `session_index.jsonl`, but whether it exposes a `Stop`-equivalent that
   can continue must be **measured before it is designed against**. If a vendor
   has no yield point, that peer degrades honestly to prompt-boundary delivery —
   a mailbox — and the room says so.

## The one-time connection, literally

```sh
# once per side, by the operator
bashy peer install claude --identity profile-c --room posix-bc
bashy peer install codex  --identity profile-b --room posix-bc
```

After that the two sessions converse and converge on their own, and the operator
appears only when a bound is hit or authority is genuinely required.

---

# Reviewer's closing note — this converges on the original plan

Read in order, the sections above look like competing designs. They are not, and
the record should say so.

**The plan already had the answer.** Vendor plugin + lifecycle hooks + session
registration + MCP inbox + a vendor-neutral envelope is exactly what its
"Vendor packages" and adapter-contract sections specify — `Register(identity,
session)`, `Deliver(session, event)`, `hooks/hooks.json`, register at
start/resume, publish terminal state at stop/end. My "peer/hook" section
restates that. I argued against the plan, went looking through transcripts,
`meet`, and delegated invocation, and came back to its architecture.

What the exploration actually contributes is **evidence and three corrections**,
not a different design.

## Evidence the plan was missing

- `bus`: **42 standing subscriptions, 0 sidecar processes, no `--adopt`** — the
  delivery substrate has never run.
- `bashy chat sessions`: one stale `log-only` row; it sees **neither** live
  session. The registry cannot represent a foreign session.
- Both vendors persist structured, cwd-tagged JSONL transcripts locally
  (`~/.claude/projects/…`, `~/.codex/sessions/…`, plus `session_index.jsonl`).
- `bashy invoke --read-only` drives Codex from a Claude session in **8.3 s**
  today, with no plugin, daemon or MCP.

## Three corrections to the plan

1. **Drop the daemon.** `agent-reachability-im-layer.md` decided sidecar
   (a)+(c), *"No host daemon"*, deferring one to the cross-host phase, and names
   the protected property: *"there is no daemon to be down."* The plan
   reintroduces a daemon for the local case without overturning that. Hooks make
   it unnecessary: the peer reaches out; nothing needs to reach in.
2. **Use `Stop` to continue, not merely to publish.** The plan uses stop/end to
   *publish terminal state*. That yields a mailbox — every turn still comes from
   the human. A `Stop` hook that can **block and continue** when peer work is
   pending is what makes it a conversation. This is the single highest-value
   line in the whole design, and bounding it (local, authoritative round and
   spend caps, since neither side can stop the other) is the main new risk.
3. **Fold `bridge` into the peer registry; keep `review`.** With hooks doing
   delivery and `invoke`/`herald` doing delegation, `bridge` has little left to
   own, and `GroupOrch` is already 28 verbs against a lean mandate.

## One genuine addition

**Transcript reading is complementary, not competing.** Hooks solve *live*
peering; they do nothing for a peer that is finished, compacted, or was never
connected — which is exactly when its accumulated knowledge is most needed. The
two compose: hooks carry the conversation, transcript reading seeds a
participant with what the other side already learned. It is also the only path
that works when the source session is gone.

## Recommendation

Adopt the plan, with the daemon removed, `Stop`-continue made explicit and
bounded, `bridge` folded into the peer registry, and the sidecar prerequisite
proven first. Sequence the vendor probe before the protocol freeze: whether
Codex exposes a yield point equivalent to `Stop`-continue determines whether
cross-vendor peering is a chat or a mailbox, and everything else is downstream
of that one measurement.
