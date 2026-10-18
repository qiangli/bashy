# Database and memory recipes

Two runnable Bash scripts plus the discovery surface and the limits you
need to use them correctly. Every command below is spelled from the
registered usage and flag parser of the pinned producer
(`github.com/qiangli/yoke`, `cmds/db/db.go`, `pkg/kbmemory`) and the
installed `bashy kb memory … --help` text. Nothing is provisioned
implicitly: every database, index, and owner session is named and
created explicitly.

## 1. Discovery

```bash
bashy db create --help                           # registered usage for every verb (bare `db --help` exits 2)
bashy db caps --engine sqlite --json             # --engine is required
bashy db caps --engine bbolt  --json
bashy db caps --engine bleve  --json
bashy db caps --engine bonsai --json
bashy db create --explain --engine sqlite --mode memory --json   # declaration + input schema, no effect
```

`db caps` prints the ordinary `bashy-db-result-v1` envelope with
`op: "caps"`; `result` carries `engine`, `categories`, `modes`
(per-mode `supported`, `via`, `lifetimes`, `sharing`, `exclusion`),
`transactions`, `exclusion` and `notes`. A bare `db caps` without
`--engine` fails `invalid_input`. `--explain` / `--dry-run` return the
operation's declaration (mutation class, effects) and `schema` without
touching storage.

Over MCP (`bashy mcp serve --tools all`), the same surface is the typed
`db_lifecycle` tool: arguments name `verb`, `subject`, `engine`, `mode`,
and either `path` (persistent) or `session` + `name` (memory), plus the
verb's own fields (for example
`{"verb":"obj","subject":"get","engine":"sqlite","mode":"memory","session":S,"name":N,"bucket":"assets","key":"k"}`).
Destructive verbs additionally need the server started with
`--allow destroy`.

## 2. Capability table (from `db caps`)

| Engine | Categories | memory mode | persistent mode | Native backup / restore |
|---|---|---|---|---|
| sqlite | sql, documents, vectors, objects, timeseries | native (owner heap) | native, WAL, multi-process | backup + restore, both modes |
| bbolt | kv, objects | supplemental adapter `memkv` | native, single-process flock | backup persistent only; no native restore |
| bleve | fts | native | native | unsupported — use top-level `db export` / `db import` |
| bonsai | graph | supplemental adapter `memgraph` | native | backup persistent only; memory → `db graph export` |

All eight categories (sql, documents, kv, fts, vectors, objects, graph,
timeseries) are served in both modes. Memory databases live in an
explicit owner process (`db session start`); other processes reach them
only through that owner. Logical interchange is per category:
`db export|import` at the top level picks the category from the target
(this is how SQL is exported — `db sql` has only
`query|exec|schema show|schema apply`), and `db doc|vec|obj|graph|ts
export|import` exist for those categories.

## 3. Targets

Every database verb takes the target either as flags or as one
positional locator:

```text
--engine E --mode persistent --path PATH          db:E%3Afile%3APATH
--engine E --mode memory --session S --name N     db:E%3Amem%3AS/N
```

The scripts use flags held in Bash arrays (`"${T[@]}"`) so they split
correctly; `kb memory` and `kb context` take the locator form. A memory
target is only reachable while its owner session runs.

## 4. Script: SQL lifecycle in both modes

Bash (arrays are required; this is not POSIX sh or zsh). Needs
`python3` only to read the session ID out of JSON.

```bash
#!/usr/bin/env bash
set -euo pipefail
W=$(mktemp -d)
SESSION=$(bashy db session start --idle-ttl 10m --json |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["id"])')
trap 'bashy db session stop --session "$SESSION" --yes --json >/dev/null || true' EXIT

for MODE in persistent memory; do
  if [ "$MODE" = persistent ]; then
    T=(--engine sqlite --mode persistent --path "$W/app.db")
    C=(--engine sqlite --mode persistent --path "$W/copy.db")
  else
    T=(--engine sqlite --mode memory --session "$SESSION" --name app)
    C=(--engine sqlite --mode memory --session "$SESSION" --name copy)
  fi
  # create
  bashy db create "${T[@]}" --json
  bashy db sql schema apply "${T[@]}" \
    --sql 'CREATE TABLE notes(id INTEGER PRIMARY KEY, body TEXT NOT NULL)' --yes --json
  # populate (values are bound, never interpolated)
  bashy db sql exec "${T[@]}" --sql 'INSERT INTO notes(id, body) VALUES (?, ?)' \
    --param int:1 --param text:hello --json
  bashy db sql exec "${T[@]}" --sql 'INSERT INTO notes(id, body) VALUES (?, ?)' \
    --param int:2 --param text:world --json
  # query (bounded by --limit / --max-bytes)
  bashy db sql query "${T[@]}" --sql 'SELECT id, body FROM notes ORDER BY id' --limit 10 --json
  bashy db show "${T[@]}" --json
  bashy db check "${T[@]}" --json
  # logical export/import (export --out must be a new directory)
  bashy db export "${T[@]}" --out "$W/export-$MODE" --json
  bashy db create "${C[@]}" --json
  bashy db import "${C[@]}" --in "$W/export-$MODE" --replace --yes --json
  bashy db sql query "${C[@]}" --sql 'SELECT count(*) AS n FROM notes' --json
  # native backup (--out must be a new file) and restore over an existing target
  bashy db backup "${T[@]}" --out "$W/backup-$MODE.db" --json
  bashy db sql exec "${T[@]}" --sql 'DELETE FROM notes' --json
  bashy db restore "${T[@]}" --in "$W/backup-$MODE.db" --replace --yes --json
  bashy db sql query "${T[@]}" --sql 'SELECT count(*) AS n FROM notes' --json
  # drop (confirmed)
  bashy db drop "${C[@]}" --yes --json
  bashy db drop "${T[@]}" --yes --json
done
echo "artifacts in $W"
```

Rules this script depends on: `sql schema apply`, `restore` and `drop`
refuse without `--yes` (`confirmation_required`); importing into a fresh
copy uses `--replace --yes`, because a merge import requires an
identical existing table and otherwise fails `conflict`; `restore` over an existing target refuses without `--replace`
(`exists`); `backup` refuses an existing `--out`. A memory database's
contents end with its owner session — export or back it up before the
`trap` stops the session.

## 5. Script: lexical memory and `kb context` in both modes

Fully local: the sqlite collection is the source of truth, a bleve index
you create is the rank index, and retrieval is `lexical` only.

```bash
#!/usr/bin/env bash
set -euo pipefail
W=$(mktemp -d)
# Prerequisite: kb context reads named KB rings. An empty, private agent
# ring isolates this recipe from any repo or host KB (none needs to exist).
export YCODE_DATA_DIR="$W/agent-data"
mkdir -p "$YCODE_DATA_DIR/kb"
printf '# Notes\n\nBackups need a new output path.\n' > "$W/notes.md"
SESSION=$(bashy db session start --idle-ttl 10m --json |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["id"])')
trap 'bashy db session stop --session "$SESSION" --yes --json >/dev/null || true' EXIT

for MODE in persistent memory; do
  if [ "$MODE" = persistent ]; then
    MEM="db:sqlite%3Afile%3A$W/memory.db"
    IDX="db:bleve%3Afile%3A$W/memory-index"
  else
    MEM="db:sqlite%3Amem%3A$SESSION/memory"
    IDX="db:bleve%3Amem%3A$SESSION/memory-index"
  fi
  # init creates the database and its memory collection (fails 'exists' if present)
  bashy kb memory init "$MEM" --json
  bashy kb memory select "$MEM" --json
  # the rank index is never provisioned for you: create it explicitly
  bashy db create "$IDX" --json
  # capture
  bashy kb memory add "$MEM" --title "Backup rule" --type lesson --tags recipes \
    --body "db backup needs a new output file; restore needs --replace." --json
  bashy kb memory add "$MEM" -f "$W/notes.md" --tags recipes --json
  # recall never syncs: reconcile the index, then retrieve
  bashy kb memory index-sync "$MEM" --index "$IDX" --json
  bashy kb memory retrieve "$MEM" --index "$IDX" --text "backup" --tags recipes \
    --mode lexical --limit 5 --json
  bashy kb context --for "backup" --rings agent \
    --memory-target "$MEM" --memory-index "$IDX" --memory-mode lexical \
    --memory-tags recipes --budget 700 --json
  # keep the records before the owner session ends
  bashy kb memory maintain export "$MEM" --out "$W/memory-export-$MODE" --json
done
echo "artifacts in $W"
```

`kb context --rings agent` reads only the agent ring under
`$YCODE_DATA_DIR/kb`; without `--rings` it also expects the default repo
ring (`docs/kb` in the current repository) and fails when that is
missing. Not every ring is memory-backed: the memory target is added
alongside whichever rings you name. Lexical search matches indexed terms, not
semantic paraphrase: `--for "backup"` finds these records, while a
paraphrase such as "how do I back up a database" returns no blocks. `kb context` runs before the session stops because a memory target is
unreachable afterwards (`lifetime_ended`). `--mode semantic|hybrid`
(and `--memory-mode semantic|hybrid`) report `unsupported`; no embedding
provider or service is started or contacted. Edits after capture use
`kb memory update MEM ID --if-revision N` (a stale `N` is a `conflict`,
never a lost update); `supersede` and `forget` close validity without
deleting.

## 6. Owner sessions and attachments

```bash
bashy db session start --idle-ttl 10m --json     # result.id is the session ID
bashy db session list --json
bashy db session show --session "$SESSION" --json
bashy db session attach --session "$SESSION" --attachment HANDLE --json
bashy db session attach --session "$SESSION" --attachment HANDLE --detach --json
bashy db session stop --session "$SESSION" --yes --json
```

`attach` and `stop` require `--session`. `--idle-ttl` ends an idle
owner. When the owner ends, every memory database it held ends with it;
`show` then fails `lifetime_ended`. Export first (§4, §5).

## 7. Concurrency: CAS, bounded waits, holds and epoch fencing

- **CAS.** `db kv cas --expect text:V|b64:V` (or `--expect-absent`), and
  `--if-revision N` / `--if-absent` on `doc put`, `obj put` and
  `graph node-put`, fail `conflict` on a stale expectation instead of
  overwriting.
- **Bounded waits.** `--wait` (default 5s) bounds lock, admission and
  hold waits; `--timeout` (default 30s) bounds the whole operation.
  `busy`/`timeout` are reported as retryable, never as silent success.
  `--request-id` deduplicates a mutation inside one owner session.
- **Exclusive holds.** `db hold TARGET` acquires whole-target exclusive
  use through an owner session and returns a claim `epoch`; a persistent
  target needs `--session S`, a memory target already names its owner.
  `db hold TARGET -- CMD …` holds for the life of a child command, which
  receives `BASHY_DB_HOLD_EPOCH`. Re-acquiring as the same holder
  refreshes the claim. `db release TARGET --session S --epoch E` acts
  only on the epoch it names, so a stale holder gets `stale_epoch` and
  cannot evict the current one.

## 8. Advisory versus enforced, local versus remote

- `bashy claim` / `bashy resource` coordination is **advisory and
  host-local**: it keeps cooperating agents on one machine from
  colliding and proves nothing about another machine.
- Database exclusion is **enforced** by the owner (memory) or the
  native lock (persistent), as each engine's `caps.exclusion` reports.
  `non_bashy_clients` is `blocked` for sqlite, bbolt and bleve, but
  `cooperative` for persistent bonsai (read-only opens bypass Badger's
  lock).
- Every target here is local. Remote reach is explicit and
  effect-declared: `db obj transfer` to a `--remote` target and
  `db ts remote query|ingest --endpoint URL` declare `net.*` effects
  and are refused under an effect cap without network; local
  `obj transfer --destination DIR` declares filesystem effects only.

## 9. Category boundaries

- **Graph.** Logical `db graph export|import` carries only the reserved
  typed subset (typed nodes, edges, revisions); export refuses a
  database holding other user DQL predicates. Native `db backup` of a
  persistent bonsai target is the fidelity path for arbitrary DQL data.
- **Time series.** Point timestamps (`ts`, nanoseconds) and int64
  values both travel as tagged strings, `{"t":"int","v":"…"}`, so int64
  round-trips exactly through
  `ingest`/`range`/`export`/`import`. `ts agg` computes `min, max, sum,
  avg` over float64 and warns they are approximate when int samples
  beyond ±2^53−1 are in scope; `count` is exact. `ts retain` needs
  `--yes`.
- **Objects.** Values are `text:…` or `b64:…`; binary bytes, content
  type and metadata survive `get`, `transfer` and export/import.
  Buckets must be created (`obj bucket-create`) before `put`.
- **Collections and buckets** in every category (`doc collection
  create`, `kv bucket create`, `vec collection add`, `obj
  bucket-create`) must exist before writes; graph edge endpoints must
  exist as nodes first.
- **Vectors.** Supplied vectors (`--vector`) are zero-network. `--text`
  embeds only with an explicit `--endpoint URL` (and model identity);
  there is no default endpoint.
- **Interchange fidelity.** Export/import is defined within one
  category's contract. There is no claim of arbitrary cross-engine
  fidelity, and no exactly-once delivery: retries are made safe by
  revisions, CAS and `--request-id`, not by the transport.

## 10. Evidence

These recipes were written against consumer `b1485db4` (producer yoke
`52e2e4f4c444`). The Sprint 453 verification record
(`docs/sprint-453-verification.md` in the umbrella) holds the gate
receipts for that build, including the installed-binary storage smoke
(objects, graph and time series over CLI and MCP in both modes, one
owner session). The two scripts above are not in that smoke; the
manager runs them against the installed build, and the results belong
in that record, not here.
