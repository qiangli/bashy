# `bashy claim who` — who is holding this path, and how do I reach them?

Sprint 541 · Story #2960 (`d440c5f5a85c`). Spec of record: the dhnt umbrella's
`docs/sprint-541-master-execution-plan.md`.

## The question

An agent about to edit a file has no way to ask who else is already writing it.
Every fact needed to answer existed on this host, in four stores that never
met:

| source | store | what it proves |
|---|---|---|
| `claim` | coord ledger (`~/.bashy/coord`, `coord.List`) | an EXPLICIT hold whose `Roots`/`Members` path set covers the path |
| `room` | room board (`room.PeekMembers`) | a LIVE session whose card `Cwd` contains the path |
| `weave` | weave queue (`weave.HoldersOf`) | a dispatched run working the path's repo |
| `commit` | local git history (`@{u}..HEAD`) | a committed-but-unpushed commit that touched the path, attributed by its `Agent:` / `Story-ID:` / `Sprint:` trailers |

The last one matters most and is the one nothing else reports: an agent that has
already finished writing leaves no live process and no lease, only a commit.

## Shape

`bashy claim who [PATH...] [--json]`, a subcommand of the existing `claim`
verb (wired in `internal/agentos/agentos.go`, `case "claim"`), implemented in
`internal/agentos/claim_who.go`.

- **Composition only, no new store.** It reads the four stores above and joins
  them. Nothing is written, so a holder it reports cannot be stale in a way the
  owning store is not.
- **Every store is read ONCE** per invocation, then each path is answered from
  that one snapshot: two paths in one answer can never disagree about who is
  working here.
- **No PATH** means the dirty paths of the current checkout (`outgit.Status`
  over `weave.RepoRootOf(cwd)`) — the files a collision is actually about.
- **Per owner**: holder, source, intent/task, pid + liveness, age, the evidence
  that put it there, and a ready-to-run `bashy mb send <holder> "..."` line
  (one per holder, not one per source).
- **Liveness is the owning store's verdict**, never this command's guess.
  `claim` uses `Claim.Liveness`; `room` uses the card pid; `weave` and `commit`
  report `unknown`, because a queue records a run and a commit records history —
  neither proves a live process.
- **Unknown owner is reported honestly**: `no owner found; post bashy mb post
  --topic announce "taking <path> — speak up if you hold it"`. An invented
  holder is worse than none — it sends the asker to coordinate with somebody who
  was never there.
- **A store that cannot be read becomes a warning**, so an empty answer is never
  mistaken for "nobody holds it".
- `--json` emits the stable `bashy-claim-who-v1` envelope:
  `{schema_version, paths:[{path, abs, owners:[…], advice}], warnings}` —
  `owners` is always an array, never `null`.

Ordering: `claim` → `room` (deepest `Cwd` first) → `weave` → `commit`. An
explicit hold outranks a live cwd, which outranks a dispatched run, which
outranks history.

## Also in this story

`agentAssignment` (`internal/agentos/agents.go`) grew a `cwd` field, populated
from the room card `Cwd` on both the weave-joined and standalone-room rows. It
is JSON-only; the `bashy agent` table is unchanged. The repo basename alone
could not say WHERE an agent is writing — two weave workspaces of one repo share
a basename.

## Verification

`go test ./internal/agentos -run 'ClaimWho|AgentRoster'` — one test per source
(claim, room cwd + deepest-first, weave holder, commit trailer, no owner), plus
the no-PATH dirty-path default and the roster `cwd` field. Every test points
`BASHY_HOME` / `BASHY_COORD_DIR` / `BASHY_ROOM_DIR` and the weave state root at
throwaway directories, and none execs the bashy binary.
