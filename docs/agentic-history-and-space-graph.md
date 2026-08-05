# Agentic history and the space graph

**Status: P0 + P1 + the kb pipe shipped (2026-08-05).** The recorder, both
planes, the five read verbs, promotion into kb, and the evidence drill-down are
in. Recipes are not, and are deliberately last.

> **Read `../../docs/knowledge-substrate-reconciliation.md` first.** It corrects
> the framing this document originally shipped with. `execlog` and `spacegraph`
> are NOT stores of record: **kb is the one truth**, `execlog` and the OTel
> spool are **streams** that feed it, and `spacegraph` is a **view**. Nothing
> here is an answer on its own — the answer is a kb page.

## What it is

bashy dispatches every command an agent runs and, until now, forgot all of it.
The bash `history` builtin is bash-exact and therefore **interactive-only**: it
records nothing on the script path, nothing on `-c`, and nothing on the
ExecHandler path an agent drives (`histSync` no-ops unless armed). So the one
process that sees everything an agent does kept no usable record of it.

This is that record — and it is two planes, not one:

| Plane | Question | Where | Layer | Shareable |
|---|---|---|---|---|
| **TIME** — the episode journal | *what ran, in what order, with what outcome* | `~/.bashy/exec/<day>/<episode>.jsonl` | stream | no |
| **SPACE** — the entity graph | *what this environment IS, and how it connects* | `~/.bashy/skills/edges.jsonl` (`0600`) | view | **never** |
| **CLAIM** — what it all amounts to | *what is known, and on what evidence* | `~/.bashy/kb/pages/*.md` | **truth** | yes, once validated |

The second is the point. `ssh -p 2222 user@remote.host` is not remembered as a
string; it teaches that **this host reached that endpoint, as that account** —
and the next agent gets to know it.

```
$ ssh -p 2222 user@remote.host      # succeeds, three times over two days

$ bashy graph reached
remote.host:2222                n=3   ok=3    as user@remote.host
    first=2026-08-03 09:12  last=2026-08-05 15:09

$ bashy graph space
host:dragon                  host      out=1  in=0
endpoint:remote.host:2222    endpoint  out=1  in=1
host:remote.host             host      out=0  in=1
account:user@remote.host     account   out=0  in=1
```

## Verbs

```
bashy graph history  [--episode E] [--cmd C] [--since D] [--failed] [--limit N] [--json]
bashy graph history  --graph            what usually follows what, and what fixes what
bashy graph history  --forget (--episode E | --before DURATION)
bashy graph space    [--kind host|endpoint|account|repo|path|net] [--json]
bashy graph reached  [--json]
bashy graph learn    [--dry-run] [--json]     stream  -> kb candidate pages
bashy graph evidence <exec://...> [--json]    kb page -> back to the raw records
```

`learn`, not `promote`: the candidate→validated rung is `bashy kb validate`,
whose own help calls that act "promote" — and two verbs both meaning promote,
doing different rungs of one ladder, is a footgun in a surface an agent drives.
(There is no `bashy kb promote`.)

## The claim, and the round trip

`graph learn` writes one **candidate** page per pitfall the evidence supports —
3 distinct sessions across 2 distinct days, no more recent success, excluding
benign and opaque outcomes:

```yaml
type: gotcha
title: go test ./hub/...
description: fails on darwin (compute) — 3 failures across 3 sessions on 3 days
tags: [exec-observed, go, compute]
scope: {os: darwin}
status: candidate                       # never `validated` — see below
evidence: exec://2026-08-01..2026-08-04#n=3
```

The page holds the **claim**; `evidence` is an **address into the stream**, not a
copy of it. `graph evidence exec://...` walks it back to the records. Copying
them into the page would put 30k lines a day into a wiki whose index has to stay
small enough to read.

**Promotion is a candidate generator, never an author.** kb already has the
ladder and `kb validate` requires evidence to climb it. A recorder able to mint
validated knowledge is the confabulation vector with a store attached. The pipe
also backs off — and says so — the moment a human validates or supersedes a
page: a person's judgement outranks a recount.

They live under `graph` because this is the **execution subgraph of the same
knowledge graph** the code and wiki layers already sit in — same id space, same
append-only-log → derived-view shape. Not a second graph, and not a sixth store.

Gated by `BASHY_EXECHIST` (on by default under an agent, off for interactive
humans), with `BASHY_AGENTIC` as the master kill. `cmd/bash` links none of it
and `--posix` is inert — both ratcheted.

## The rules that are load-bearing

**Redaction is structural, not conventional.** `Writer.Append` takes a
`Scrubbed`, and only `Scrub` can construct one — a writer that forgets to redact
does not compile. Three passes in a fixed order: credentials by value, foreign
home directories by shape, then local identity by tag. Reversed, the hostname
inside `postgres://u:p@host/db` gets tagged, the URL stops matching the
credential pattern, and the password ships.

**Two renderings from one scrub.** Stored argv keeps co-reference tags
(`‹user:5af6›`) so the evidence still reads as a sentence. The **template** —
the node key — uses bare classes (`<USER>@<HOST>`), because a tag is derived
from the local hostname and would give two machines two different nodes for the
same command.

**Time is never in a key.** An edge is keyed by `(src, relation, dst)` and
nothing else; counts and timestamps are attributes. This is the failure mode the
whole design is arranged around and it is **silent**: put a clock in the key and
every observation lands at a fresh address, the store fills with `n=1`
singletons, and nothing errors.

**Failure teaches nothing.** Edges record on success and on payload failure. A
*transport* failure is unattributed — `ssh` can fail on a wrong port, a wrong
login, a rebooting host, or a dropped VPN, and in three of those the graph was
right. Correction happens by supersession on positive evidence.

**Facts have no export path, and that absence is the enforcement.** Every node
names something real about somebody's machine. There is no `Export`, no `Sync`,
no marshaller that emits the set — the same discipline as `craft`'s fact store.

**Every read verb reports its coverage.** An empty answer has at least four
causes — nothing matched, recording was off, the days were pruned, the records
died unflushed — and reporting the first when the truth is the third is a
conclusion drawn from the *absence* of evidence:

```
corpus: 12,430 records, 2026-07-22 .. 2026-08-05 (14 days); recording: ON
pruned: 41,209 records deleted by retention
lost:   88 records stamped but never flushed (process died)
```

`Seq` is stamped at **creation**, not at flush, which is what makes that `lost`
line countable instead of invisible.

**The fade.** A claim outlives its evidence, on purpose. The stream is pruned on
a retention policy; the kb page is not. So resolving an address whose records
have aged out is not a failure — it is the answer, and `graph evidence` says so
and exits 0:

```
evidence pruned — the claim stands, its records do not
  exec://2026-07-02..2026-07-04#n=9
```

What must never happen is a claim quietly losing its provenance. A page that
cannot say *"the records behind me were deleted"* is asserting something it can
no longer support — the absence-of-evidence failure with a citation stapled to
it.

## Limits, stated rather than hidden

- **Builtins are invisible.** They never reach the ExecHandler, so `cd` — the
  most cwd-relevant command there is — is not recorded. cwd is reconstructed
  from the next external command: correct, but coarse.
- **Nothing crosses `execve`.** `make test` spawns a compiler, a linker and a
  test binary and bashy sees none of them. Those records carry `opaque: true`
  and render `[OPAQUE — children not observed]`; they are never counted as
  leaves.
- **No stderr at the seam.** Error signatures are exit-class only. The design
  doc's "stderr hash + snippet" cannot be produced here.
- **`https://` teaches no host.** `craft`'s git spec deliberately excludes it —
  a token is a different credential world from a key — so an https clone
  contributes no edge. Conservative on purpose.
- **A canonicaliser bump is irreversible.** Raw argv is never stored, so
  templates cannot be re-derived. `CanonVer` is part of the key and a bump is a
  corpus reset, not a migration.
- **`0600` is a no-op on Windows.** The store inherits the user profile's ACL;
  this package does not promise a guarantee it cannot keep everywhere.

## Cost

One `write(2)` on an already-open fd, no lock, no fsync — against a measured
p50 dispatched-command time of **0 ms** (59% of dispatches complete
sub-millisecond; those are the in-process coreutils calls that are bashy's whole
advantage). The audit log's flock+fsync model is not affordable here at any
price, which is why this is a sibling store rather than a field on that record.

Episode, self-host, scrubber and pid are each resolved once via `sync.OnceValue`.
Nothing is opened until the first record; when the recorder is off, no
middleware is appended to the chain at all.

Retention is a **directory delete of a past day** — never a rename. Rotate-by-
rename lands on a file other processes hold open: on unix their writes vanish
into an unlinked inode, on Windows it fails outright, and neither reports
anything.

## Source

| Path | What |
|---|---|
| `coreutils/pkg/execlog/` | TIME plane: record, writer, canonicaliser, prune, `Promote`, `Transitions` |
| `coreutils/pkg/execlog/evidence.go` | the address into the stream, and the fade |
| `coreutils/pkg/execlog/tokb.go` | the pipe: stream → kb candidate page |
| `coreutils/pkg/spacegraph/` | SPACE plane: edges, bi-temporal store, `Observe` |
| `coreutils/cmds/graph/space_verbs.go` | the five read/write verbs |
| `bashy/internal/agentos/exechist.go` | the middleware |
| `bashy/internal/agentos/exechist_diag.go` | the advisor→recorder diagnosis hand-off |

Design of record: `../../docs/knowledge-substrate-reconciliation.md` (the layer
model, and what is still open), then
`../../docs/execution-knowledge-graph-design.md` (the original node/edge design).
