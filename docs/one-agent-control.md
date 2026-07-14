# One agent control

*Shipped 2026-07-14.*

Every command that drives an agent CLI — `invoke`, `weave`, `meet`, `foreman` —
now steers it through one primitive, over one wire.

## What was there before

Three commands claimed to steer an agent. They meant three different things, and
only one of them steered anything.

| command | what it actually did |
|---|---|
| `weave say` | wrote keystrokes to a live run's pty control socket. **Real steering.** |
| `meet say` | wrote to the same kind of socket — but a meeting's turns were headless one-shots, so **nothing was listening**. |
| `foreman tell` | queued the message and spawned a **brand-new agent**, replaying the whole conversation as its prompt. |

The second is the interesting failure. `meet say` reported success every time:

```
→ Sable (round 2): stay on the gate question — you are re-litigating the schema
```

The socket existed. The write succeeded. The agent — a one-shot that had already
run its prompt and exited — never heard a word of it. **A control channel that
reports success and delivers nothing is worse than one that refuses**, because it
buys the operator's confidence with a lie.

`foreman tell` is the same failure wearing a suit. It *works* — you get an answer
— but it is conversation, not steering: it cannot interrupt, because by the time
the message lands the agent it was meant for is gone. From the outside the two are
indistinguishable. You type `tell`, the status goes to `working`, an answer comes
back.

## The primitive

`chat.Session` — a live agent you can talk to.

```go
sess, err := chat.Start(ctx, "Ada", chat.SessionOptions{Prompt: q, Cwd: dir})
sess.Say("stop — you are off the agenda")   // arrives as keystrokes, mid-turn
sess.WaitIdle(ctx, 25*time.Second)          // the turn ends on silence
text := sess.Turn()                          // what it said since the last mark
sess.Close()
```

**`Invoke` is a question. `Session` is a conversation.** Invoke gives you one
prompt, one answer, and a clean turn — stdout and stderr stay apart on a pipe, and
the process exit *is* the turn boundary. Session gives you an agent you can
interrupt, and charges you for it (below).

It lives in `chat`, and that placement is load-bearing: `agentChildEnv` is what
scrubs the operator's vault secrets out of the child, grants back only the one API
key that model declared, forces the agent's shell to be bashy, and stamps its
principal identity. A session launcher built anywhere else would silently drop all
four, and nothing would fail loudly enough to notice.

## The wire

One protocol had grown three implementations — `agentpty`, `agentlaunch` (its own
copy of the same twenty lines), and `weave` (hand-rolled base64 frames). They had
already begun to diverge: `BrokerSay` flattened newlines and escaped NULs; weave's
attach loop did neither and sent whatever the operator typed straight down the
socket.

`agentpty` now owns the encoding **and** the transport:

| | |
|---|---|
| `TextFrame(s)` | a sentence, delivered as typing, ending in Enter |
| `VerbatimFrame(b)` | a **keystroke** — raw bytes, Tab, bare Enter, an escape sequence |
| `SendFrame(sock, f)` | the transport, with the long-path file fallback |

Two frame kinds, because *"send the agent a sentence"* and *"press this key at the
agent"* are genuinely different acts. `weave say --raw/--enter/--tab` needs the
second; nothing else does.

## What each command does now

**`foreman tell`** holds the agent open and types at it. When the tool has no
interactive launch it still falls back to replay — that is the only thing that
works there — and **the state says so out loud**:

```json
{"steering": false, "steer_why_not": "tool \"aider\" declares no interactive launch (steer_exec)"}
```

A silent downgrade would recreate the exact failure this change exists to fix.

**`meet --steerable`** holds each speaker open for its whole turn, so `meet say`
reaches a running agent. Without the flag, `meet say` now **refuses**:

```
this meeting's turns are headless one-shots — the agent runs its prompt and exits,
so there is nobody to interrupt.
Start a meeting with --steerable to hold each speaker open for its turn.
```

**`weave say` / `weave attach`** are unchanged in behaviour and now speak the
shared wire, so a paste with an embedded newline is escaped the way `meet say`
would have escaped it.

**`judge`** and **`sdlc`** keep using `Invoke`, and should: a judge asks one
question and returns a verdict, read-only, with nothing to interrupt.

## Why `--steerable` is a flag and not the default

A headless turn ends when the process exits — an exact boundary, for free.

A live turn has **no boundary at all**. The agent just stops typing. So it ends on
silence (`WaitIdle`), which means every turn pays a quiet period on the way out
plus a TUI's startup on the way in. On a four-seat, three-round meeting that is
minutes of pure waiting. A pty also merges stdout and stderr, so the tool's own
chrome lands inside the captured answer.

So a chair who wants to interrupt asks for it and pays for it. This is the honest
trade, and it is stated in the flag's own help text.

It is also the sharpest argument for the **first-party harness**: with a real event
stream, a turn's end is a *fact the agent reports*, not a silence we interpret.

## The bug this found

Building it turned up one more instance of the absence-of-evidence class, in the
resolver itself. `agentlaunch.ResolveWithCatalog`'s unknown-tool fallback **ignored
`Steer` entirely**: with no catalog entry there was no `steer_exec` to check, so the
"cannot be steered" guard never ran, and the caller got a headless argv with
`TakesPrompt=false` — a process that exits immediately, plus a control socket
nobody is listening on. Every symptom would have looked like success.

`CanSteer("definitely-not-a-registered-tool")` returned **true**. A test found it;
nothing else would have.

> A steerable session cannot be resolved from a fallback. If the tool is not in the
> catalog, there is no interactive launch — say so.

## Still owed

- **ycode's control socket.** All five third-party harnesses steer; the first-party
  one does not. This is parity work now, not a differentiator.
- **A real turn boundary.** `WaitIdle` is a heuristic and is named like one. An
  agent that pauses long enough looks finished; one that streams a spinner never
  does. Certainty about what an agent *did* comes from the artifacts it left, not
  the shape of its output.
