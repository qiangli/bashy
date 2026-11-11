# kb-catalog — the existing KB as a browsable Viewer book, shared through Klatch

Sprint 614 S3 (example level, experimental). `kb-catalog.bsh` is one Bash#
skill-script that renders the KB ring a human should browse as **one Viewer
book** and hands the link to a Klatch conversation. It composes existing verbs
only — `bashy kb search|list|show|backlinks --json`, `bashy viewer add|append|open`,
`bashy klatch messages-send` — and writes no store of its own: no Library
database, no second wiki, no new link syntax. The KB record stays the
original; every catalog page says how to open it (`bashy kb show REF`).

```sh
bashy --bashsharp examples/kb-catalog/kb-catalog.bsh build --ring repo --kind runbook --json
bashy --bashsharp examples/kb-catalog/kb-catalog.bsh open
bashy --bashsharp examples/kb-catalog/kb-catalog.bsh share --klatch CONVERSATION
```

`register.sh` registers the same body as `bashy kb-catalog` (command + skill).
It is provided, not run, by the story: register it yourself when you want the
short name.

## Verbs

| Verb | Composes | Result |
|---|---|---|
| `build [TERM...] [--kind K] [--tag T]... [--ring repo\|host\|agent \| --dir D \| --base-dir ROOT] [--book ID] [--title T] [--k N] [--json]` | `kb search TERM... --json --k N [--type K] [--tags T,...]` when terms are given, else `kb list --json [--type K]` (tags filtered client-side); per record `kb show SLUG --json` + `kb backlinks SLUG --json`; `viewer add BOOK --title T --json` once (an existing book is reused), `viewer open BOOK --json` for the URL and page count, then `viewer append BOOK --title T -f PAGE.md --json` for the index page and one page per record | `{"ok":true,"book","url","index_url","scope","index_page","count","entries":[{ref,slug,title,page}]}`; human form: `BOOK URL&page=N (COUNT entries, scope S)` |
| `select ...` (same filters) | the selection step alone | `{"ok":true,"scope","filters","count","entries":[...]}` or `ref<TAB>kind<TAB>status<TAB>title` lines |
| `open [--book ID] [--json]` | `viewer open ID [--json]` | id, URL, status, pages — never content |
| `share --klatch CONVERSATION [--book ID] [--text T] [--json]` | `viewer open ID --json` then `klatch messages-send channelId=CONVERSATION text=...` | klatch's `{"ok":true,"result":...}` plus `book`, `url`, `conversation` |
| `--dry-run` (build, share) | runs only the kb reads | the plan: every viewer/klatch argv that would run, and with `--out DIR` the rendered pages |
| `help`, `--skill`, `--contract` | — | usage, the embedded skill guide, the `skill.dhnt` line |

Errors are `{"ok":false,"error":{"code","message"}}` on stderr: `USAGE` (2),
`CONFIG_NOT_FOUND`/`TEMPLATE_NOT_FOUND` (2), `NO_ENTRIES`, `KB_FAILED`,
`VIEWER_FAILED` (1, carrying the viewer's own message, e.g. "start it with
viewer serve"). The klatch verb's exit status and envelope pass through.

**Pages are append-only.** A rebuild never edits a page: it appends a fresh
index page followed by the new entry pages after whatever the book already
holds, so the Viewer report-book history a reader saw stays true. Page
numbers inside a build are computed from `viewer open`'s page count; if the
book changed under the build, the index landing on the wrong page is refused.

**Scope is the KB's own.** `--ring repo|host|agent`, `--dir`, `--base-dir`
are passed straight to `bashy kb`, so a catalog shows exactly what that ring
grants the caller; nothing is copied across rings. Who may read the resulting
book is the Viewer's rule (`bashy viewer share|close`), and who may read the
Klatch message is the conversation's membership.

## What a page holds (and does not)

Each entry page (`templates/entry.md`) carries the record's title, `kb:` ref,
kind/status/tags, its description, the `bashy kb show REF` line, its
**references as links** and its **backlinks**:

- `[[slug]]`, `[[kb:slug]]`, `[[kb:repo/slug]]` and bare `kb:...` refs: a link to that
  record's page when it is in the same build, always with `bashy kb show kb:...`;
- `sprint:N`, `todo:ID`, `issue:ID`, `skill:NAME`, `episode:ID`, `out:...`,
  `session:...`: `bashy define REF` (the atlas resolver, `docs/command-atlas.md`);
- `http(s)://` URLs: Markdown links to the original;
- repo file paths (`docs/x.md`): named as files in the owning repo (`bashy lens -t md PATH`);
- backlinks from `bashy kb backlinks SLUG --json` with their relation type.

The record **body is not copied** by the shipped template (`{{body}}` exists
for shaping that wants it). The index page (`templates/index.md`) is a table
of the build: title → page link, kind, status, tags, `bashy kb show REF`, and
the exact `build` line that reproduces it.

## Shaping hook

- `templates/entry.md`, `templates/index.md` — page layout. Plain
  `{{placeholder}}` substitution; placeholders: `title ref slug id seq kind
  status tags tags_line summary scope scope_line refs backlinks body page url`
  (entry) and `title count scope filters_line built entries rebuild book`
  (index). `--template FILE` / `--index-template FILE` pick another layout.
- `catalog.json` — the filter config: default `book`, `title`, `ring`,
  `kind`, `tags`, `query`, `k`, `exclude_status`, `template`,
  `index_template`, `share_text` (with `{{url}} {{book}} {{count}} {{title}}`).
  Flags override it; `--config FILE` / `KB_CATALOG_CONFIG` select another
  file, so one checkout can carry a "team runbooks" and a "my notes" catalog.
- `KB_CATALOG_KB`, `KB_CATALOG_VIEWER`, `KB_CATALOG_KLATCH` — the composed
  commands (default `bashy kb`, `bashy viewer`, `bashy klatch`); the test uses
  them to substitute recording stubs.

## CLI parity (one-way: every page action has a CLI path)

| Page action (Viewer / Klatch) | CLI path |
|---|---|
| Open the KB catalog from a Klatch conversation | `kb-catalog share --klatch C` posts the link (= `bashy klatch messages-send channelId=C text=...`); `kb-catalog open` / `bashy viewer open kb-catalog` prints the same URL |
| Browse the catalog index and pages | `bashy viewer show kb-catalog --page N [--json]`; the selection itself is `kb-catalog select --json` / `bashy kb list --json` |
| Filter by kind / tag / ring / query | `kb-catalog build --kind K --tag T --ring R TERM...` (= `bashy kb search --type K --tags T` / `bashy kb list --type K`) |
| Follow a ref to the original record | `bashy kb show REF` (printed on every page), `bashy define kind:id`, the URL itself, `bashy lens -t md PATH` for repo files |
| See a record's backlinks | `bashy kb backlinks SLUG --json` (the same list the page shows) |
| Share / close the book, invite a reader | `bashy viewer share kb-catalog`, `bashy viewer close kb-catalog`, `bashy viewer join ID INVITE` |
| Export the catalog | `bashy viewer export kb-catalog [--to pdf] -o FILE` |
| Rebuild after KB changes | `kb-catalog build ...` (the index page prints the exact line) |
| Change a record | not a page action (the book is view-only); the KB verbs `bashy kb update\|supersede\|validate\|forget` are the only write path, never the store files |

Everything above is also reachable through the raw doors (`POST /api/action`
on the viewer, the Klatch helper), which stay valid but are not the parity
path.

## Verify

```sh
bash examples/kb-catalog/tests/test.sh
```

Hermetic contract / dry-run test: a temp KB ring written only through
`bashy kb add`, a stub viewer and stub klatch recording their argv, no server
and no registration. It checks the filters, the dry-run plan (add + index +
pages, no live call), the rendered pages (title/summary/kind/tags, original
line, refs as links, backlinks, no body copy), the append-only build after
existing pages, the rebuild path, that the KB store is untouched, `open`,
`share` in both modes, the shaping hooks and the actionable errors.

## Known limits / open questions

- The registered `bashy viewer` command refuses `--dry-run` (its atlas args
  schema lacks the kit's common options; the binary itself honours it). This
  wrapper therefore plans its own dry run instead of forwarding the flag.
  `bashy klatch` has no dry run at all; same answer. Candidates for S2/S6.
- `bashy define` resolves `kb:` and `sprint:` today; other `kind:id` refs are
  still printed as `bashy define REF` so the page tells the reader what to run
  once S2 adds the resolver.
- Klatch has no "open a page inside chat": the catalog link is a message, and
  the reader opens it in the Viewer page (loopback or the app-shell mount).
