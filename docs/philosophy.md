# Philosophy — local first

> **bashy is all an agent needs.**
>
> An agent should be able to record what is wanted, do the work in isolation, decide
> whether it passes, judge whether it is any good, build it, and ship it — **on one
> machine, with no network, no account, no forge, and no cloud.**

## 0. Why this follows from what bashy *is*

bashy is the **narrow waist** between an agent and the machine. Bash is that waist for
humans — everything above it and everything below it need only agree on the one thin
interface in the middle — and every agentic tool already speaks it, because they were all
trained on it. It is the one interface an agent never has to be taught.

Which makes bashy the **irreducible floor**: not the only tool an agent uses, but the one
it can never *not* have — the universal fallback from which it can reach everything else.

**Local-first is not a separate principle. It is what "irreducible" means when you say it
out loud.** A floor that depends on something reachable-or-not is not a floor; it is a
client of somebody else's floor. "The one interface an agent can never *not* have" and
"needs a working network" are contradictory statements, and this document is about taking
the first one seriously.

---

## 1. The air-gapped room is a *test*, not a market

Nobody is asking for an air-gapped shell. That is not why this constraint exists.

The air-gap is how we **find out whether the tool is actually self-sufficient**, because
self-sufficiency is not something you can assess by reading a feature list. A tool that
quietly depends on a hosted API in one verb looks complete right up until the moment it
isn't — and that moment is never chosen by you. It is:

- the flight, the SCIF, the regulated bank, the factory floor;
- the vendor outage, the expired token, the rate limit, the region failure;
- the day the API you built on changes its pricing, its terms, or its mind;
- the hotel wifi.

**If it works in the air-gapped room, it works everywhere else.** So the air-gap is the
test we run — and the network is the *extension* we allow, never the *foundation* we
assume. The cloud is a relay: useful, and replaceable. The shell is the floor, and a floor
does not get to have dependencies.

## 2. The claim is falsifiable, and it is enforced

A philosophy in a README survives until the first well-meaning commit that reaches for a
hosted service "just for this one verb". Nobody notices, because nobody re-reads the
README.

So this one lives in the code. Every command in bashy must declare its **security
effects** (`read`, `write`, `net`, `exec`, `spend`, …) in the Command Atlas — the build
literally will not start if a verb forgets. Which means the local-first claim is not a
belief. It is a **query**:

| stage | verb | effects | network? |
|---|---|---|---|
| **plan** | `issue` | `read, write` | **no** — a committed register, no forge |
| **plan** | `sprint` | `read, write` | **no** |
| **code** | `weave` | `exec, write, spend` | **no** — isolated workspaces, local git |
| **test** | `gate` | `exec` | **no** — run the command; the exit status is the verdict |
| **test** | `check` | `exec` | **no** |
| **cross** | `dag` | `exec, write, remote` | **no** — the build/test/deploy runner. `remote` is the *opt-in* `--fleet` mode; the default runner is local, and one host is an ordinary fleet size |
| **cross** | `kb` | `read, write` | **no** — what this host has learned |
| **cross** | `skills` | `exec, read, write` | **no** — what this host knows how to do |

**Not one verb in the lifecycle loop declares `net`.** The loop closes on one machine, by
construction.

And `pkg/atlas/localfirst_test.go` keeps it that way: **a verb in the loop that starts
reaching for the network fails the build.** The escape hatch is deliberately narrow — to
do it, you must *delete the verb from the list*, in a diff, where a reviewer can see the
philosophy being traded away and ask what it bought.

## 3. The one thing bashy cannot compute for itself

**Inference.** `judge`, `invoke` and `meet` need a model, and no amount of Go will
conjure one.

This is the single honest hole in the story, and it has a local answer that ships **in
the box**: `bashy ollama` is a managed *local* runtime. So the air-gapped lifecycle has
no gap in it — only a prerequisite, and the prerequisite is included. A test asserts
that too: if bashy ever stops shipping a local inference runtime, every LLM-shaped verb
silently becomes network-only, and that test is the only thing that would say so.

Everything else that touches the network falls into one of three shapes, and **each has
a local answer**:

| what needs the network | why | the local answer |
|---|---|---|
| `go`, `node`, `python`, `cargo`, `clang`… | download the toolchain | **first fetch only.** Verified, cached, then offline forever. Pre-seed the cache and the air-gap is intact. |
| `loom`, `zot`, `seaweedfs`, `kopia` | download the binary | **first fetch only** — and then they are a *local* git forge, a *local* registry, *local* object storage, *local* backup. |
| `sdlc` | needs a forge | **`loom` is a forge**, and it runs here. |
| `login`, `tessaro`, `sphere`, `doctl`, `kubectl` | genuinely remote | **optional by design.** These are how you *leave* the local machine, on purpose. They are the extension, not the floor. |

## 4. Three pillars

The thesis decomposes into three layers, and **each is worthless without the one below
it.**

### I. A shell it can trust — *compatibility*

A real Bash 5.3, not a shell-shaped thing: **86/86** on Bash's own conformance suite, with
the POSIX-conformance work continuing beyond it. This is the floor, and it is not
negotiable — an agent that writes a bash script and watches it die on a parameter
expansion has learned that the tool lies, and nothing built on top of that matters.

Compatibility is the *floor*. It is never the differentiator, and performance may never
be bought by breaking it.

### II. A userland it can rely on — *capability*

The pure-Go coreutils, running **in-process**. `grep | sort | wc` behaves identically on
macOS, Linux and Windows, with no forks and no system dependencies — because the
alternative is an agent whose pipeline works on the developer's laptop and dies in CI.

This is also where the performance story lives, and it is a *pipeline* story, not a
single-command one: zero forks means the whole script wins even where one Go command
loses to its C ancestor.

### III. A lifecycle it can drive — *agency*

The SDLC spine, end to end, with governance woven through it:

```
issue  →  weave  →  gate  →  judge  →  dag
record    isolate   pass?   good?     build/ship
```

plus the things that make it safe to hand to a machine: `claim` (two agents cannot
silently stomp each other), `audit` (a tamper-evident record of everything an agent
ran), `secrets` (no plaintext keys in an rc file), the advisor (stop the doomed retry
loop), and the effect declarations that let an agent *see* what a command will do before
it runs it.

Agency is the point. The other two pillars exist to make it trustworthy.

## 5. Six venues — one machine is *complete*, not degraded

Work runs in one of six venues, from a single native process out to a hosted cloud:

| # | venue | scope |
|---|---|---|
| 1 | **userland** | this machine, native |
| 2 | **workspace** | this machine, git-isolated (`weave`) |
| 3 | **sandbox** | this machine, OS-isolated (`podman`) |
| 4 | **sphere** | your machines, peer-direct |
| 5 | **cluster** | your machines, orchestrated |
| 6 | **cloud** | someone else's machines |

Local-first means something precise here: **venue 1 is a complete product, not a
fallback.** You are never running a crippled version of bashy because you happen to be
offline; you are running the whole thing, and venues 4–6 are how you *choose* to
spend more machines when you have them.

The corollary matters for the fleet runner: **one host is an ordinary fleet size.** Not a
degenerate case, not a fallback path — the same code, with a pool of one.

## 6. What this philosophy forbids

A philosophy that forbids nothing is decoration. This one has teeth:

- **No lifecycle verb may require the network.** Enforced by the atlas ratchet.
- **No lifecycle verb may require an account, a forge, or a cloud.** `issue` is a
  register in the repo, not a GitHub client. `gate` is a command, not a CI service. `kb`
  is a directory, not a SaaS.
- **No non-permissive dependency may be compiled in, linked, embedded, or vendored.**
  MIT/BSD/Apache only. Download-and-exec is not bundling; linking is.
- **No CGo in the core.** Pure Go, or it does not cross-compile, and if it does not
  cross-compile the agent on the other platform does not have it.
- **Compatibility may never be traded for performance.** An optimization that breaks
  bash ships as an opt-in extension or it does not ship.
- **Every command declares its effects.** A command an agent cannot reason about before
  running is a command an agent cannot safely run.

## 7. Why "local first" and not "offline"

"Offline" is a mode. **"Local first" is an ordering.**

The local machine is where the truth lives: the register is a file in your repo, the
knowledge base is a directory on your disk, the gate is a command on your box, the run
happened in a workspace you can look at. The network can *add* to that — more machines,
bigger models, a shared board, a team — and everything in venues 4–6 exists to let it.

But the network is never *where the thing is*. It is somewhere the thing can also go.

That is the whole difference between a tool you own and a tool you rent.

---

*Companions: [`command-atlas.md`](command-atlas.md) — the effect axis this philosophy is
enforced through; [`licensing-supply-chain-policy.md`](licensing-supply-chain-policy.md) —
the dependency rules that keep the binary self-contained.*
