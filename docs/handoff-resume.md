# `bashy handoff` / `bashy resume` — a session that outlives its tool

Status: **v1 shipped 2026-07-12.** Feature doc + the use cases that shape what comes next.

## The problem

Every agentic tool ships a `/resume`, and every one of them is a prison. Claude Code resumes from
`~/.claude/projects/…`, Codex from its own store — each a **proprietary transcript format, on ONE
machine, in ONE tool**. A session is captive to the tool that made it. You cannot resume a Claude
session in Codex. You cannot resume your laptop's session on the build box. You certainly cannot hand
your session to a colleague.

bashy is already the **shell underneath every one of those tools** (`bashy install-agent`, the forced
shell env), which makes it the one layer that sees all of them and belongs to none. So it is the layer
that can own a session record which **outlives the tool that wrote it** — and the machine, and
eventually the person.

## The rule that makes it portable

> **The record is an ARTIFACT, not a POINTER.**

Nothing in a record may reference a tool's private session store, a transcript id, or a path that
means something to only one program. Everything a successor needs is *in* the record:

- the **continuity brief** — what I was doing, why, what I learned;
- the **next action**, stated so a cold agent in a *different tool* can act without re-deriving the plan;
- the **in-flight working tree** — the diff against `HEAD` (staged *and* unstaged together) plus
  untracked files carried **by content**.

A record is a file. Files travel: `scp`, the mesh, a commit, an issue comment.

### The piece nothing else had

`sprint handoff`, `weave baton write` and the cloudbox session lease all record **prose**. A successor
inherited a *narrative*, not a *working tree*. That is not hypothetical in this project: one session
found an unexplained edit in the tree and had to guess whose it was, and another swept a third
session's staged submodule pins into a commit — landing an untested engine regression that took the
release gate from 86/86 to 85/86.

Three deliberate choices in the capture, each a lesson from that failure:

1. **The index is not preserved.** Staged and unstaged are captured together (`git diff HEAD`). The
   index is precisely the shared mutable state that let one session commit another's staged work. A
   handoff carries what was *changed*, not what someone had half-decided to commit.
2. **Untracked files travel by content.** A patch does not contain them, and the new file the agent
   just wrote is routinely the entire point.
3. **Capture is a READ.** It does not stash, reset, or clean. An agent being killed mid-edit must not
   have its work moved by the very command meant to preserve it.

## What it composes with (and does not replace)

| concern | owner |
|---|---|
| the prose brief / continuity | `bashy sprint` (the board) |
| isolation + live control | `bashy weave` |
| the future wake-up | `bashy schedule --prompt` |
| **the portable record + the working tree** | **`bashy handoff` / `resume`** |

Once work is handed to another tool, **weave owns the session — the tool is just the process inside
it**. So from *any* session, in *any* tool, you can `weave status` / `weave log` (watch), `weave say`
(steer — it types into the agent's TUI), `weave attach` (take the keyboard), `weave kill` (stop it;
workspace, branch and commits are preserved), and `weave start --resume --issue N -- <any tool>` (take
it over with a *different* tool, in the same workspace).

## Use cases

### 1. Stop for the day *(shipped)*
`bashy handoff -m "…" --next "…"` — parked. The work waits, intact. Tomorrow, in any tool:
`bashy resume`.

### 2. Switch tools mid-task *(shipped)*
Claude hands to Codex; the record carries the diff; Codex continues in an isolated weave workspace.
Claude keeps watching and steering with `weave say` / `weave attach`.

### 3. Pivot this agent to something more urgent *(shipped)*
Handoff parks the current work cleanly, so the agent can take a different task without the first one
rotting in an unattributed dirty tree.

### 4. Hand to an autonomous scheduler *(shipped, via `schedule --prompt`)*
The brief arrives *with* the job, so the future agent wakes up with the task in hand.

### 5. **Team handoff: User A hands off with Codex, User B resumes with Claude** *(NOT YET — see below)*
The corporate case, and the one that makes this a product rather than a convenience: an issue is
handed off by one engineer and picked up by another, on a different machine, in a different tool,
possibly the same day.

## What the team case needs that v1 does not have

v1 assumes **one person, many tools and machines**: the store is host-local (`~/.bashy/handoff/`), and
the record travels by whatever means you choose. The team case is **many people**, and it surfaces four
requirements — none of which the record format blocks, which is the point of the artifact rule:

1. **A shared transport.** Today a record is a file you move yourself. For a team it must land where
   the *issue* lives. Three candidates, in increasing order of coupling:
   - **the forge** — attach the record to the issue (a comment or an artifact). This is the natural
     home: the handoff is *about* an issue, and `bashy sdlc` already drives issue lifecycles through
     labels and comments. Zero new infrastructure.
   - **the repo** — commit the record under a known path. Simple, auditable, versioned; but it puts
     working-tree diffs into history, which is usually wrong.
   - **cloudbox** — a shared store with real auth. Most capable, most coupled; breaks standalone-first.

   The forge is almost certainly right, and it composes with `sdlc` rather than duplicating it.

2. **Identity that means something across people.** `From`/`ResumedBy` already carry a
   `principal.Ref` (tool, agent, episode, host). For a team it must resolve to a *person*, not just a
   tool on a host — otherwise "who handed this to me?" has no answer.

3. **A single-claim guarantee.** The record is stamped `ResumedAt`/`ResumedBy` so it cannot be picked
   up twice — but that stamp is local. Two colleagues resuming the same handoff *concurrently* need a
   **shared** claim, which is exactly the coordination work already planned (a lease over the shared
   store). Without it, the team case reproduces the very collision this whole line of work exists to
   prevent — only now between *people*.

4. **Redaction, and it is not optional.** **A handoff record contains real source code** — the diff,
   and whole untracked files. In a corporate setting that is an IP and secrets surface: an in-flight
   diff can trivially contain an API key an engineer was mid-way through wiring up. Sharing a record
   with another person is therefore a *disclosure*, and it must be treated as one. The atlas already
   classifies both verbs with the `read` effect for this reason. A shared transport **must not ship
   before the redaction pass** — the ordering is a safety property, not a preference.

**Sequencing implication:** the shared transport is not the hard part; the *claim* and the *redaction*
are. Build them first, and the team case falls out of the same record format that already works.

## Non-goals

- **Not a transcript.** The record carries what a successor needs to *continue*, not a replay of the
  conversation. A transcript is a tool's private business; the working tree and the intent are not.
- **Not a merge tool.** `resume` refuses to apply onto a dirty tree. Landing a patch on top of someone
  else's uncommitted edits manufactures a conflict that neither agent understands and neither can
  attribute — the exact failure this feature exists to end.
- **Not a live channel.** Handing off is not attaching. `weave attach` is same-host today; extending it
  over the mesh is a *reach* change (proxy the control socket + PTY over the tunnel), tracked
  separately, and deliberately kept out of the record format so the transport can change without
  breaking portability.

## Implementation

- `coreutils/pkg/handoff/` — `record.go` (the `bashy-handoff-v1` schema), `capture.go`
  (`CaptureWork` / `Apply`), `store.go` (atomic save; `Pending` finds a handoff by **path-set
  intersection**, so a session that spanned several repos is discoverable from *any one of them*),
  `cli.go` (both verbs).
- Front door: `bashy handoff`, `bashy resume` — both **cross-stage** on the SDLC spine (you hand off
  work at any stage), effects `read`+`write`.

### Two bugs worth remembering

- **`git apply --3way` needs the patch's blob SHAs in the *target's* object database.** That holds for
  a clone; it fails outright (exit 128) on a genuinely foreign repo — another machine, a fresh
  checkout — which is *the only case this feature exists for*. `--3way` is an optimisation; **plain
  apply is the guarantee**.
- **A git patch must end with a newline.** The git helper trimmed trailing newlines, which corrupted
  every patch. A one-byte bug that would have made every non-empty handoff fail to apply, while
  looking perfectly fine in the record.
