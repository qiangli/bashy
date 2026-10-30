# desk viewer — view-only books of agent reports

S537.V1. The desk tool a human uses to **read** what an agent wrote: status
reports, recaps, plans, close-outs, documents. View-only by construction —
there is no `set`/`edit` verb (exit 2 / HTTP 400). A human edits text in
**notepad**; a human reads in **viewer**. In chat, an agent gives a 2-4
line summary plus the viewer link instead of pasting the full report.

Named `viewer`, not `view` (`view` is vim read-only in the userland).

## Data separate from template; the agent never writes HTML

```
agent: data JSON (+ short Markdown prose fields)  ──or──  a Markdown body
  → viewer fills a shipped, shapable template (Go text/template → Markdown
    with ```chart fences, ![](rows.csv) embeds, live sources)
  → kit renderer (desk-kit/render: goldmark, no raw HTML, bluemonday)
  → sanitized HTML in the kit shell
```

1. Each template declares its data shape in `templates/NAME.schema.json` (a
   small JSON-schema subset: object/array/string/number/integer/boolean,
   `required`, `enum`, `maxLength`, `maxItems`, `format: markdown`,
   `x-source`). `--data` is validated first; a mismatch is refused (exit 2)
   with the path of the first bad value (`data.metrics[0].value: want a
   number, got a string`), never rendered broken. Unknown fields are refused.
2. Logic-light `text/template` only: no code execution, no remote fetch. Every
   string value is escaped so it renders as the literal text it is — no
   emphasis, links, embeds or HTML. `format: markdown` prose fields keep
   Markdown, with raw HTML shown as text. Funcs: `json` (for chart fences;
   `<`, `>`, `&`, backticks escaped), `code`, `link` (http(s) only), `join`,
   `default`, `check`, `count`, `add1`.
3. `x-source: sprint` fills a property from the live card
   (`bashy sprint show N --json`) when the data omits it. The agent writes
   only the narrative: `{"summary": "...", "done": [...], "next": [...]}`.
4. Data keys that name a credential are refused (the desk no-secrets rule).

Shipped templates: `status`, `plan-summary`, `recap`, `close-out`. A shaped
copy (both files) under `<data dir>/templates/` overrides the shipped one
or adds a new name.

## Books

An item is a **book**: ordered, append-only pages. `add --sprint N` makes
`sprint-N` (the same id across manager handoffs); `add --session S` makes
`session-S`; `add [ID]` any other. Pages are never edited — a correction is
the next page, so the history a reader saw stays true.

The page shows first / prev / next / last (also ← →), an index, a
jump-to-page field (`p3`, `3`, `first`, `last`, `index`), and **changed since
previous**: each block (paragraph, list, table, fence) new or changed since
the previous page *of the same kind* (same template, or both plain Markdown)
is highlighted in place, with a banner counting changes and removals.
Documents whose rendering spans blocks (footnotes, `[toc]`) render whole and
show only the counts.

Assets: `--asset FILE` (repeatable) stores a file beside that page only
(`items/<id>/assets/<page>/`); `![](rows.csv)` embeds resolve there and
nowhere else. Assets are served sandboxed (`CSP: sandbox`).

## Doors

```bash
bashy viewer serve                                   # 127.0.0.1:18096
bashy viewer add --sprint 537                        # sprint-537 http://…/?id=sprint-537 pending
bashy viewer append sprint-537 --template status --data notes.json
bashy viewer append sprint-537 -f design.md --asset load.csv
bashy viewer open sprint-537                         # id URL status — never content
bashy viewer show sprint-537 --page 2 --json         # Markdown + template data
bashy viewer export sprint-537 -o s537.html          # self-contained HTML (assets inlined)
bashy viewer export sprint-537 --to pdf -o s537.pdf  # headless Chrome print (CHROME_PATH)
bashy viewer list | share ID | close ID
```

The HTTP API is the CLI 1:1: `POST /api/action {"action": "append", "id":
…, "template": …, "data": {…}}`, `GET /api/state?id=` (contentless),
`GET /api/export?id=&page=` (download). Every JSON result carries
`schema_version: bashy-viewer-v1`; errors are `{"error": {"code",
"message"}}`. Common options: `--json --fields --no-input --explain
--dry-run --help=mcp`.

Storage dir: `--dir DIR` / `VIEWER_DATA` (default
`$BASHY_HOME/desk/viewer`). It is `--dir`, not the kit's usual `-data`,
because `--data` is the template data file.

PDF: humans use the page's **Print / PDF** button (print CSS hides the
shell). Agents use `export --to pdf`, which prints the self-contained export
with headless Chrome; `bashy browser` has no print-to-PDF subcommand yet.

## Verify

```bash
cd examples/desk/viewer && GOWORK=off go test ./...     # contract (view-only) + red/green + unit
PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs CHROMIUM_PATH=… \
  VIEWER_BIN=/path/to/viewer node browser-test.mjs
```

Red/green: `-break accept-set|open-emits-content|allow-secret` turn the
view-only contract red; `no-escape` turns the data-neutralisation test red
(the renderer still blocks script — the second layer); `no-validate` turns
the bad-data test red.
