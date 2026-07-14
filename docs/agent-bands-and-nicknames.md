# Agent bands and nicknames

*Shipped 2026-07-13. Mechanism lives in `coreutils/pkg/fleet`; surfaced by
`bashy agents`, `bashy models`, `bashy whois`, and `bashy meet`.*

Three things make a fleet of agents drivable by a human: you need to know **who is
worth asking**, you need to be able to **say their name**, and the name you write into
a record has to **still be true a year later**. This is how bashy does all three.

## The band

Every model carries a **band** — a capability peg from **L1** (basic) to **L4**
(frontier). An agent is a `tool:model` binding, so it **inherits** the band of the model
it binds and carries none of its own.

| band | means | send it |
|---|---|---|
| **L4** | frontier | the hardest, most ambiguous work |
| **L3** | advanced / thinking | diagnosis, judgment, complex multi-file change |
| **L2** | medium | scoped implementation, well-specified fixes |
| **L1** | basic | mechanical, single-file, unambiguous |

```
$ bashy agents list
NAME                      NICK     BAND  TOOL      MODEL            RELIAB      RESOLVES  RING
agy-gemini3.1             Anouk    L2~   agy       gemini3.1        medium      yes       embedded
claude-fable5             Sable    L4~   claude    fable5           high        yes       embedded
claude-opus4.8            Beatrix  L4~   claude    opus4.8          high        yes       embedded
codex-gpt-5.5             Arlo     L4~   codex     gpt-5.5          high        yes       embedded
codex-gpt5.6-terra        Rufus    L3~   codex     gpt5.6-terra     unmeasured  yes       embedded
opencode-deepseek-v4-pro  Ingrid   L3~   opencode  deepseek-v4-pro  medium      yes       embedded
ycode-deepseek-v4-pro     Elif     L3~   ycode     deepseek-v4-pro  unmeasured  yes       embedded
```

The `~` on every band means `band_source` is **not `measured`** — it is a declared
guess or an operator's judgment. **Nothing in this fleet is `measured` yet**, and the
tilde is there so nobody quotes a guess as a measurement.

**Bands are normalized across providers.** A vendor's own tier ladder is never mapped
positionally: if a provider ships four tiers and its best model performs at the L1
level, all four of its models are L1. A band is a statement about capability, not a
re-badging of someone's marketing.

**Band 0 is not a weak band — it is an unpegged model.** It renders as `-`, `bashy
models verify` warns about it, and it matches *no* band filter. An unpegged model can
never be swept into a roster by accident.

### Selecting by band

This is what the band is *for*. Instead of knowing your fleet by heart and naming
everyone:

```
$ bashy agents list --min-band 3     # everyone worth seating at a design discussion
$ bashy agents list --band 2         # exactly the mid-tier
```

and, the payoff — a meeting that seats itself:

```
$ bashy meet start --min-band 3 --topic "should the cache be write-through?"
seating 3 of 21 agents at band L3+:
  Sable      claude:fable5              L4  high
  Beatrix    claude:opus4.8             L4  high
  Ingrid     opencode:deepseek-v4-pro   L3  medium
skipped: agy-gemini3.1 (L2) — below the requested band
```

Two things that are deliberate:

- **Skips are reported, never swallowed.** A roster that quietly drops an unreachable
  agent reads, to whoever later opens the minutes, as though the whole band was
  consulted. A decision credited to a table that never sat is worse than no decision.
- **The operability gate is the same one the router uses.** An agent whose harness is
  not installed is not seated — the band says *should we ask it*, operability says
  *can we*.

`--min-band` and `--participant` are alternatives, not a blend: a band seats the table
for you; a participant list says you already know who.

## Nicknames

`opencode-kimi-k2.7-code` is an address, not a name. You cannot ask for
"Kimi-k2.7-code's read on this" without reading it off a screen first. So every agent is
**assigned** a name rather than configured with one — `Sable`, `Beatrix`, `Arlo`.

The assignment is **deterministic in the binding**: the same `tool:model` draws the same
name on every host, every run, with no state file and nothing to sync. That is the whole
reason it isn't random — two machines that disagreed about who "Johnny" is would make
the name worse than useless.

A nickname resolves everywhere a name is taken:

```
$ bashy whois Beatrix
claude-opus4.8  claude · opus4.8  (agent, claude:opus4.8)
$ bashy chat --agent Beatrix -m "review this diff"
$ bashy meet start --participant Sable --participant Arlo --topic "..."
```

Override it when you want to:

```
$ bashy agents set claude-opus4.8 --nick Bond
```

An explicit nickname always beats an assigned one, and the assigned one steps aside
rather than colliding — one name never means two things, or `whois` would have to guess.

## Version-explicit names, floating aliases

A model is named for the **exact version it is**, and the family name is **derived**:

```yaml
# models/opus4.8.yaml
name: opus4.8
family: opus
version: "4.8"
band: 3
model: claude-opus-4-8      # the id that reaches the wire
```

Nothing declares the alias `opus`. The catalog *computes* it as "the highest version in
family `opus`". Ship `opus4.9.yaml` and `opus` re-points itself — no alias list to edit,
no chance of it going stale.

```
$ bashy whois opus
opus4.8  Claude Opus 4.8  (model, subscription backend)
target:  claude-opus-4-8
```

Agents get the same treatment: `claude-opus` is a derived alias that follows the family,
so it keeps working across releases while the canonical `claude-opus4.8` names an exact
pair.

### Why bother — the rot this prevents

**Speak the alias; record the address.**

A record that says *"an L3 agent passed this gate"* rots: six months from now L3 means a
different model, and the record has quietly become a lie about who verified what. A
record that says *"claude:opus4.8 verified this"* is true forever.

The same trap catches a *floating model name*. If `opus` were the canonical name, then
every attestation saying `claude:opus` would silently change meaning on the vendor's
next release — the record would not even know it had rotted. Naming the version is what
makes the record honest.

So the rules are structural, not conventional:

- **A binding is canonicalized however you spell it.** `bashy agents add x --model opus`
  *persists* `model: opus4.8`. The identity that lands in a record is version-explicit
  even when the human typed the floating alias.
- **A derived name never shadows a declared one.** Name a model literally `opus` and the
  derivation yields to you.
- **A band is never persisted.** It is used to *choose*, never to *record*.

## Re-pegging

Bands are a snapshot of a moving landscape; they are meant to be re-pegged as models
ship. A new model is one file:

```
$ bashy models add opus4.9 --family opus --version 4.9 --band 3 \
    --provider anthropic --kind subscription --upstream claude-opus-4-9
```

Ring precedence is embedded → shared → cloud → local, so a shipped default can be
overridden by an org catalog, and an operator's explicit peg beats both.

One consistency rule worth remembering: **band and `quality:` must agree.** If an L4
model has a lower `quality` prior than an L3 one, every ranking will put them in the
wrong order and the band will look broken when the priors are what is wrong.
