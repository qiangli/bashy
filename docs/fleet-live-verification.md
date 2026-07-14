# Fleet live verification — `bashy agents verify --live`

*Shipped 2026-07-13. Mechanism: `coreutils/pkg/agentctl` (classifier) +
`coreutils/pkg/chat` (launcher) + `coreutils/pkg/fleet` (the verb).*

## The problem, in one sentence

**A declared binding that does not run looks exactly like one that does.**

`bashy agents verify` is *structural*: it confirms that an agent's tool and model
both resolve **in the catalog**. It never asks the tool whether it will accept the
model. So an agent can be listed, banded, routable, ranked — and completely
incapable of speaking.

That is not hypothetical. On 2026-07-13, live-probing the fleet for the first time
found that **two of eight agents had never worked**:

| binding | what actually happened |
|---|---|
| `agy:gemini3.1` | `--model gemini-3.1` is rejected outright. The id agy takes is the display string `Gemini 3.1 Pro (High)`. |
| `*:deepseek-v4` | *"The supported API model names are deepseek-v4-pro or deepseek-v4-flash, but you passed deepseek-v4."* |

Both had been failing on **every run, for months**. And because nothing ever
launched them to find out, the fleet wrote the failures down as *properties of the
models*: agy's ledger read `reliability: medium — needs interactive sign-in first`.
It was never signing in. It was exiting in two seconds on a flag error.

Three further defects hid behind the same blindness:

- **`api_key_ref` was declared and never read.** The credential firewall
  (`secrets.ScrubAgentEnv`) correctly strips vault secrets from a spawned agent —
  and nothing granted the one key back, so every kimi agent failed to authenticate
  while the fleet recorded it as unreliable. (deepseek only looked fine because
  aider caches that key in its own config.)
- **Operability was `exec.LookPath`.** The router's gate asked *"is the binary on
  disk"*, never *"does this agent run"*. `agy:gemini3.1` sat in the capability
  matrix at `operability: 0.996` across 8 observations while dead.
- **The first version of this very verifier passed on absence of failure** — see
  below. It is the bug it exists to catch, and it was in the catcher.

## What `--live` does

```
bashy agents verify              # structural: does the binding resolve?
bashy agents verify --live       # actually LAUNCH each agent and read what comes back
bashy agents verify --live claude-opus4.8
bashy agents verify --live --timeout 3m
```

It asks each agent a question a working agent cannot fail — *"Reply with exactly
PROBE_OK"* — in a scratch directory, and classifies what comes back:

| verdict | meaning | what to do |
|---|---|---|
| `ok` | launched headless and answered | nothing |
| `bad-model` | the tool rejected the **model id** | re-peg the model's `model:` in the registry |
| `stale-contract` | the tool rejected a **flag** | the launch template has drifted from the installed CLI |
| `needs-auth` | it is waiting for a human, or a credential was refused | `bashy secrets`, or log in |
| `failed` | it ran and did not answer | read the output it prints |

`bad-model` and `stale-contract` are separated on purpose: **the fix is different**,
and an operator told only "it failed" has to go and work out which.

## The three rules that make it trustworthy

**1. A pass requires POSITIVE evidence. Only `PROBE_OK` passes.**

The first version of the classifier passed anything that produced output and
matched no known failure signature. It duly reported `aider:deepseek-v4` as **ok** —
an agent that dies on every single run, with an error message that happened not to
be on the list.

> A verifier that passes on the ABSENCE of a known failure is not a verifier. It is
> a list of the failures somebody already thought of.

The signature lists survive only for **diagnosis** — telling you whether to re-peg a
model, fix an argv, or go and log in. They never earn a pass.

**2. The probe launches through the SAME path a real turn takes.**

It calls `chat.Invoke`: same argv from the registry, same launch guard, same
read-only stripping, same child environment. A probe with its own launch logic
could pass while production failed — which is worse than no probe, because it makes
a dead binding look *verified*.

**3. It is `ReadOnly`, so it needs no privilege.**

Answering a question requires no write authority. Read-only *removes* the
approval-gate kill-switches rather than asking permission to keep them, so the
probe passes the launch guard **by construction** on an ordinary uncontained host.
Nobody has to set `BASHY_ALLOW_UNSAFE_AGENT_LAUNCH` to find out whether their fleet
works.

## It feeds the router

Each verdict is written into the capability matrix as **observed operability**,
keyed by the canonical `tool:model` (never a nickname — that would fragment one
agent's evidence across two rows):

```
capability.RecordProbe(agent, ok)   // OVERWRITES; does not average
```

Overwriting is deliberate. **Operability is a fact about now, not an opinion to be
refined.** An exponential moving average would let eight stale samples outvote the
one observation that actually tried it — which is precisely how a dead agent came
to hold `operability: 0.996`.

## What it does NOT measure

It measures whether an agent **can speak**. It says nothing about whether the agent
is any *good*.

Ranking models for steward/conductor work needs a **gated bake-off**: the identical
task to each candidate, in isolated workspaces, scored on objective evidence — did
the run converge, pass the gate, survive the judge, and leave a clean tree. That is
a separate exercise and it needs write authority.

The rankings in `dhnt/docs/fleet-model-role-ranking.md` predate this tool, and were
gathered while two bindings were dead — so their headline finding ("reliability
gated capability before raw intelligence ever got to matter") is *true but
misattributed*: it was not harness immaturity. **It was three unread registry
fields.** The fleet was measuring its own configuration bugs and recording them as
model reliability.

## Run it

```
bashy agents verify --live
```

Costs one small model call per agent. Run it after any registry change, after a
tool upgrade, and before trusting any ranking.
