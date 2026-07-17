# bashy chat — the governed interactive launcher

**Status:** design + implementation (2026-07-17).
**One-liner:** `bashy chat` is the preferred front door for launching a supported
third-party agent CLI *interactively* — you get the tool's **native UX**, but with
the **fleet-selected model**, under **full bashy governance**, and **registered so
the rest of the fleet can reach it** (steer / interrupt / observe / attach / coach /
meet). `invoke` stays the one-shot question.

## Why

Today, to use an agent CLI interactively you run it directly (`claude`, `codex`,
`opencode`, `agy`). That bypasses everything bashy provides:

- **model selection** — you get whatever the tool's own config names, not a
  fleet-pegged model chosen by nick or band;
- **governance** — the operator's vault secrets leak into the child, no single-key
  scoping, the agent's shell isn't forced to bashy, no principal identity, no OTel
  span, no audit record;
- **addressability** — a directly-launched agent is a black box: nothing else in
  the fleet can steer it, coach it off a loop, observe it, or seat it in a meet.

`bashy chat` closes all three. An agent launched this way is a **first-class fleet
citizen**: it carries a control socket and a live-sessions registry entry, so the
same primitives weave/meet/foreman already use (`agentpty` TextFrame/VerbatimFrame
over the ctlsock) reach it seamlessly. The reason to prefer `bashy chat <tool>` over
bare `<tool>` is that the second one can't be helped by anything.

This is the codebase's own doctrine finally implemented (`pkg/chat/session.go`):
*"Invoke is a QUESTION… Session is a CONVERSATION."* `chat` was wrongly an alias of
`invoke` (a question); it now hosts the conversation.

## Not a bashy REPL — a transparent passthrough

The UX is **identical to running the original tool**. There is no `you>`/`Ada>`
wrapper prompt. `agentpty.Run` already does raw-mode local-TTY passthrough
(`pkg/agentpty/pty.go`: parent stdin is a TTY → `term.MakeRaw` + bidirectional
`io.Copy` between the local terminal and the child PTY), so the agent's own TUI is
what you see and type into. bashy's contribution is *around* the session — model,
governance, registration — never *in front of* it.

## Selection — a specific one, or any one

| flag | meaning | resolution |
|---|---|---|
| `--agent NICK\|name\|binding` | a **specific** agent | `Catalog.Binding` (nick/name/family-alias) → `ResolveAgent` fallback |
| `--band N` | **any** operable agent pegged ≥ N | `meet.SeatByBand`, strongest first, take the top |
| `--tool T` | **any** operable agent whose tool is T | `SeatByBand` filtered to `tool==T` |
| `--band N --tool T` | best operable **T** at band ≥ N | filtered `SeatByBand` |
| *(none)* | the default agent | parity with `invoke` |

Selection reuses the exact fleet machinery `bashy agents list` and `meet --min-band`
already use — `Catalog.Binding`, `SeatByBand`, `capability.Operable` — so "who is
routable" means the same thing everywhere. An unreachable agent is skipped loudly,
never silently (the absence-of-evidence rule).

## Transport — reuse `chat.Session`, run it in the foreground

`chat.Interact` is the foreground twin of `chat.Session.Start`. It shares the SAME
`resolveLaunch(steer:true)` and `agentChildEnv` (the single choke point for
secret-scrub + single-key grant + shell-forcing + principal), so governance cannot
drift between a programmatic session and a human one. The only difference: it runs
`agentpty.Run(cmd, logSink, {CtlSock, Capture:false})` **in the foreground** so the
human's terminal is the session, until the tool exits.

**One `agentpty` change:** the interactive branch (`parentTTY && !Capture`) wrote
PTY output only to `os.Stdout`, so a live session was invisible to observers. It now
*also* tees to `logSink` when one is supplied — native UX **and** a capture that
`chat attach`/`observe` can follow. Steering (the ctlsock) already worked in both
branches. (Caveat, inherited: a native-TUI capture is raw ANSI — good enough to
follow/steer, not a clean transcript. Headless capture, `invoke`, remains the clean
path for a recorded turn.)

## Registry — the connective tissue

`~/.bashy/sessions/<id>.json` — one file per live governed session:
`{id, binding, nick, tool, model, band, ctl_sock, log_path, pid, cwd, started}`.
Written at launch, removed at exit, stale-PID-pruned on read. This is the board that
makes a chat-launched agent *addressable*: every control verb (and, later, coach/meet
attach) resolves an id → its ctlsock + log through this registry. It is deliberately
fleet-wide (`~/.bashy/sessions/`), not chat-private — it is the live-agent board.

## Control surface — from any terminal

- `bashy chat sessions` (alias `ls`) — list live governed sessions.
- `bashy chat steer <id> <text>` — inject a line mid-turn (`agentctl.Say` → TextFrame).
- `bashy chat interrupt <id>` — ESC (VerbatimFrame) to break a tool loop.
- `bashy chat attach <id>` — follow the captured log + forward your stdin lines as
  TextFrames, `/detach` to leave (the agent keeps running) — the `weave attach`
  pattern over an arbitrary registered session.

Steering an interactive session where a human is already at the keyboard is for a
*second* party (a supervisor, a coach, a teammate on another host later). The human
driving the session just uses the tool.

## Phases

- **P0 (this change)** — the governed native-interactive launch by fleet selection
  (`chat.Interact` + `PickAgent` + the `agentpty` tee), the live-sessions registry,
  and the `sessions`/`steer`/`interrupt`/`attach` control verbs. `invoke` unchanged.
- **P1** — coach auto-attach opt-in (`chat --coach`): a human session can ask a
  banded coach to watch (the reflex coach + `BandGraduatedEscalator` already exist;
  this just points them at a registry id). Off by default — the human is the coach.
- **P2** — meet/foreman read the same registry, so a running chat session can be
  *pulled into* a meeting or handed to a foreman goal without relaunching.
- **P3** — cross-host: a registry entry relayed over cloudbox/matrix so a session on
  one machine is steerable from another (rides the existing control plane).

## Testability

- `PickAgent` — table test: specific/band/tool/both/none → binding; operability skip.
- registry — round-trip + stale-PID prune.
- routing decision `wantsInteractive(hasInstruction, isTTY, forced)` — table test.
- the live PTY launch — manual smoke (needs a real agent + a TTY); CI cannot drive a
  PTY (the `internal/cli` readline tests are already skipped on the Windows leg for
  this reason).
