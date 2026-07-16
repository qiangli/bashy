# `bashy delegate` — one verb for handing work to an agent (including yourself)

*Design of record, 2026-07-16. Brand-neutral (bashy/coreutils only).*

## The decision

**One verb: `delegate`.** It hands work to an agent — where the agent may be a
*different* one, **yourself**, or the **same tool with a different model**. There is no
separate fork/twin/decouple verb; delegating *to yourself* is simply a target, and
that target is the one that **inherits your full live context** (no handoff brief) and
runs **detached** so you stay responsive.

```
bashy delegate codex "<task>"          # a different agent: fresh context, task = brief
bashy delegate self "<directive>"      # yourself: inherit full live context, run detached
bashy delegate --model opus "<dir>"    # same tool, different model: my context, fresh eyes
```

### Why one verb

`delegate` is the word humans and agents actually reach for, and it reads for **every**
target ("delegate to codex", "delegate to yourself", "delegate to opus"). The
distinguishing property of the self / same-tool-different-model target — *context is
inherited and the parent stays free* — is what "delegate to yourself" **means**; it
does not need its own noun. "Decoupled" is the **behavior** of that target (the work is
decoupled from the interactive session), not a second verb.

Verbs considered and rejected as a *separate* self-fork verb:
- **`twin`** — names the mechanism ("a copy"), not the value; can't take a
  different-agent target.
- **`decouple`** — names the value well but reads awkwardly for other targets
  ("decouple to codex"); it is the behavior, kept in prose, not a verb.
- **`separate`** — collides with task *decomposition* (split work into pieces), which
  `weave`/`dag`/`sprint` already do.
- **`fork`/`branch`/`clone`/`split`** — collide with the delegated tools' own `/fork`
  `/branch`, bashy's git subcommands, and the coreutils `split` text tool.

## Why one verb, not two

Delegation is a spectrum of context transfer, and bashy only covered the low end:

| you say | context transferred | existed |
|---|---|---|
| delegate to a *different* agent | none — task is the brief | `invoke` / `weave start -- <agent>` |
| hand off, resume tomorrow | working tree + prose brief | `handoff` / `resume` |
| **delegate to yourself** | **full live context, no brief** | **nothing** |

The self case is genuinely uncovered (surface audit: every existing path re-seeds the
child from a prompt/brief/goal; `handoff` is closest in intent but is serial and, by
its own contract, "artifact not transcript" — it refuses to touch the transcript).
Rather than a new noun-y verb (`twin`/`fork`), the self case is just `delegate` with a
`self` target — the distinguishing property (context inheritance + parent-stays-live)
is what "delegate to yourself" *means*.

## Semantics (SOTA-aligned)

This is Pattern-2 "fork-to-delegate" from the agent-CLI SOTA (2026): a full-context
copy that runs in parallel while the parent stays live. Only Claude Code's
`/fork <directive>` implements it precisely today; everyone else's `fork`/`branch`
either switch-you-into the copy (Pattern 1) or spawn a fresh briefless child
(Pattern 3 — subagents). `delegate self` = Pattern 2.

Inherits, for the self target: full transcript/context, cwd, tool state, **and the
caller's capability/permission scope** (the SOTA gotcha: Claude's fork does *not*
carry scope, so the "no handoff" promise leaks at the permission boundary — for an
autonomous same-model spawn we inherit scope by default, `--scope` to narrow).

## The mechanism reality

bashy cannot reach inside a **third-party** agent's transcript (its own `handoff`
package calls that store "a prison" it won't touch). So `delegate self`:

- maps to each harness's **native** context-fork where one exists —
  `claude --fork-session` / `/fork`, `codex fork`;
- achieves **true brief-less** self-fork only on a **first-party harness** bashy
  controls (ycode, via its `--events`/session channel) — this is the first-party
  harness thesis paying off;
- degrades to a `handoff`-style working-tree + continuity brief for a tool with no
  fork primitive, and **says so** (no silent downgrade).

## Build sketch (phased)

- **P1** — `delegate` as the ergonomic verb over `invoke`/`weave` for the *different-
  agent* case (mostly aliasing + argument routing; atlas entry + synopsis + e2e
  dispatch). Cheap.
- **P2** — `delegate self` for harnesses with a native fork (claude/codex): resolve
  the caller's tool+model, invoke the native fork with the directive, run detached.
- **P3** — first-party brief-less self-fork on ycode via its session/event channel;
  scope inheritance; `--model` context-transplant to a different model.
- Atlas: `delegate` is a new verb → needs an atlas Entry (Stage/Group/Tier/Caps) + a
  security Effect + coverage/e2e-dispatch entries. `self`/`--model` are flags, no
  extra atlas work.

## References

- Agent-CLI fork/branch SOTA survey (this session): Claude `/branch` vs `/fork`,
  Codex `fork`, opencode/Amp/Warp — the four patterns (switch-into / delegate-parallel
  / fresh-child / checkpoint-rewind).
- `docs/one-agent-control.md` — the session primitive (`chat.Session`), why `meet
  --steerable` is a flag, `foreman interrupt`.
- `docs/first-party-harness.md` — why only a harness bashy controls can do brief-less
  context-fork.
