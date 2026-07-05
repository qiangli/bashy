# Phase 2 — `bashy foreman` (lined up)

Elevate one-shot `bashy chat` into a **persistent, steerable session** that both a
human (at a TTY) and a conductor-agent (over a control channel) can drive
incrementally — Layer 1 of `docs/conductor-steering-surface.md`. This is the
cockpit that then coordinates Layer 2 (gate-broker) and Layer 3 (`bashy browser`,
shipped in Phase 1).

## What exists (surveyed) — reuse, don't reinvent

- **`bashy chat`** (`coreutils/pkg/chat`, 361L) — the one-shot primitive:
  "invoke an agent with a single unattended instruction" (`Runner` iface +
  `Invoke`). Foreman *wraps* this: a session is a chat runner kept alive + fed.
- **weave control channel** (`coreutils/pkg/weave`) — a per-worker unix control
  socket `ctl/issue-N.sock` + `weave say` that injects input into a running
  agent. **This is the steering primitive to generalize** to a foreman-level
  control socket (`foreman/<id>.sock`), so `foreman tell/pause/…` reach the live
  session the same way `weave say` reaches a worker.
- **`bashy dag`** (`coreutils/pkg/dag`) — the literate task-graph engine (engine,
  contract, cache, dry-run, JSON report). Foreman *embeds* it: a session can run
  a `dag.md` as its backlog, steerable between steps.
- **ycode foreman** (`ycode/internal/cli`, `internal/commands`) — the reference
  **daemon state-machine**: per-project `foreman/state.json` + a `commands`
  queue, verbs `pause|resume|stop|skip|prio|tell|status` (Boss→Foreman→Worker).
  Port the *shape* (state file + command queue + verbs); don't couple to ycode.
- **`bashy` interactive stack** (`bashy/internal/cli/{interactive,forced_interactive,prompt}.go`,
  from `sh/interactive`) — the readline REPL to reuse for human drive mode.
- **`ycode/skills/ycode-foreman`** — the protocol/role skill to **merge into
  `bashy/skills/conductor`** (one playbook, one home — the conductor skill
  already moved to bashy).

## Surface

`bashy foreman` — a session with two co-equal drive modes over one state:

- **Human mode:** `bashy foreman [run <dag.md>]` with a TTY → a readline REPL
  holding the session; you type steering lines (`tell …`, `status`, `pause`,
  `resume`, `stop`, `skip`, plain text = a message to the agent). Reuses the
  bashy interactive loop.
- **Agent mode (control channel):** a detached session + a control socket, driven
  by sub-verbs from any process (a conductor-agent, a cron, another foreman):
  - `bashy foreman start [--detach] [run <dag.md>] --goal "<…>"` — begin a session.
  - `bashy foreman tell <id> "<msg>"` — inject a steering message (via the socket,
    like `weave say`).
  - `bashy foreman status <id>` / `list` — reconciled state (idle/working/blocked/
    done), current step, backlog, last output; `--json`.
  - `bashy foreman pause|resume|skip|prio|stop <id>` — the ycode verb set.
- **`chat` becomes `foreman --once`** — the one-shot path is the degenerate
  session (start → one turn → exit), so there's one code path.

## State & continuity

Per-session dir `~/.bashy/foreman/<id>/` = `state.json` (goal, mode, status,
current step, drive lease) + `commands` queue (append-only, drained by the
session loop) + `ctl/<id>.sock` (live input) + `log`. Mirrors weave's on-disk,
server-less model — resumable, inspectable, no daemon-server. A session that
outlives a sitting is re-attachable (`foreman resume <id>`), and `bashy schedule`
can self-wake it (the conductor-autopilot pattern already documented).

## Sequenced steps (each a commit)

1. **`pkg/foreman` core + state.** Session type over `pkg/chat`'s `Runner`:
   `state.json` + `commands` queue + status reconcile. Gate: unit tests of the
   state machine (start→tell→pause→resume→stop) with a stub Runner (no live agent).
2. **Control channel.** A foreman control socket generalized from weave's ctl
   pattern (share the helper if clean); `foreman tell/pause/resume/skip/prio/stop`
   write commands, the session loop drains them. Gate: tell-reaches-session test
   over a real socket with a stub runner.
3. **`bashy foreman` verb + agent mode.** `start [--detach]`, `status`, `list`,
   `--json`. Gate: `foreman start --detach` + `foreman status <id>` round-trips;
   `foreman --once` reproduces today's `chat`.
   **Home = AgentOS front-door, NOT `cmds/all`.** foreman drives agents (like its
   `chat` parent, and like `weave`/`dag`) — it is a front-door verb, not a POSIX
   userland tool. It also imports `pkg/dag` (step 5), and `pkg/dag`'s own tests
   blank-import `cmds/all`, so listing foreman in `cmds/all` forms an import cycle
   (`pkg/dag` test → `cmds/all` → `cmds/foreman` → `pkg/foreman` → `pkg/dag`).
   Register it by importing `cmds/foreman` directly in `bashy/internal/agentos`
   (mirroring the `cmds/graph` exclusion) + add `"foreman"` to `alwaysShimVerbs`;
   it routes as `bashy foreman` through the tool-registry dispatch fallthrough and
   stays out of the bare `cmd/coreutils` multicall.
4. **Human REPL mode.** `bashy foreman` on a TTY → readline loop (reuse the
   interactive stack) mapping typed lines to commands/messages. Gate: forced-
   interactive test feeding scripted lines.
5. **Embed `bashy dag`.** `foreman run <dag.md>` drives the dag engine as the
   backlog, pausing for steer between steps; `skip`/`prio` act on dag targets.
   Gate: run a 2-node dag, `tell` between nodes, assert order + steer applied.
6. **Merge the skill.** Fold `ycode/skills/ycode-foreman` into
   `bashy/skills/conductor` (a `## Foreman — the steerable session` section: the
   Boss→Foreman→Worker protocol, the verb set, when to use foreman vs weave).

## Acceptance / gate

`CGO_ENABLED=0 go build ./...` clean; `go test ./pkg/foreman/... ./cmds/...`
green (incl. `pkg/dag` — the cycle is broken); `bashy foreman --once --agent X
--instruction "…"` matches today's `chat`; `foreman start --detach` + `tell` +
`status` round-trip with a stub runner (no live model needed in tests); `bashy
foreman` routes through the front-door dispatch (foreman is NOT in `cmds/all`);
`make test-bash` stays 86/86; the conductor skill carries the foreman section.
Deferred: the gate-broker auto-routing (Phase 3), multi-host session sharing.

## Why partly sequential, partly parallel (conductor's call)

Steps 1–5 are **one cohesive session subsystem sharing heavy state code** →
sequential (one strong agent, or the conductor), each commit gated. Step 6 (the
**skill merge**) is independent docs/prose → the conductor does it directly in
parallel. Recommended: **codex sequential on 1–5** (dogfooding Phase-0 `fleet
--auth` pre-flight + the explicit-headless launch discipline), **I do step 6**
and gate each of codex's commits + the final converge.
