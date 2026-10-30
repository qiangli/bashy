# Chart

Quick local statistics on one page. Server-side SVG, standard-library Go,
shared desk kit, no CDN or runtime network dependency. No query language,
accounts or layout editor. All source and the shipped page template are editable.

## Start

```sh
bashy ./register.sh
bashy chart serve
# In another terminal:
bashy chart add --input testdata/otlp.json --json
printf '[12,18,15,24]' | bashy chart add --input - --widget gauge --json
```

Run these from this directory before registration; `bashy chart` then works
from anywhere. The Bash# entry point builds the native Go server/CLI into the
user cache. Go 1.27 matches the desk kit. The only module dependency is
`desk-kit v0.0.0`, replaced by `../kit`.

Default listener: `127.0.0.1:18098`. Store: `$BASHY_HOME/desk/chart/` (the kit's
XDG/home fallback applies when unset). `serve --addr`, `--data`, `--prefix`
select another listener, storage directory or mount. CLI requests need the
same `--addr` and `--prefix`. Registration adds both the `chart` command atlas
entry (`bashy help chart --format=mcp`) and launcher tile. The launcher proxies
the root server; a direct page link remains the stable loopback URL.

## Desk operations

`add|open|show|set|list|share|close` all accept `--json`, before or after the verb.
`add` accepts `--id NAME`; otherwise it generates a stable id. `open ID` prints
exactly id, URL and status, with no data. `show ID` returns the saved data,
rendered HTML/SVG, and metadata. `set ID --input FILE` replaces the snapshot.
`--title` and `--body` set the title and optional description.

Creation returns an `ownerKey` once. Preserve it privately and supply it using
`CHART_KEY` or `--key` for reads and mutations. `share ID --json` creates a
single-use invite URL. Open that URL in a browser to join without an account;
`revoke ID` invalidates all invitations and browser memberships. `close ID`
freezes the item and stops refresh. The browser can also create charts, copy
links, switch theme, share, revoke and close. An owner may enter their key on
the item's page; it stays in that tab's session storage.

CLI and browser call the same `POST /api/action` JSON operations. Example:

```json
{"action":"add","title":"Requests","chart":{"input":"[1,4,8]","format":"json","widget":"counter"}}
```

`chart` is an optional structured field alongside the shared kit's `body` and
`fields`; an item with only a body remains a valid empty desk item. Unknown
request fields, second JSON values and secret-typed/named input are rejected.
`GET /api/state?id=ID` is contentless. Content requires ownership or a joined
participant cookie. Files persist atomically with private permissions.

## Inputs and widgets

| Input | Shape |
|---|---|
| JSON | Numeric array, numeric object, or array of records |
| CSV | Header row followed by records |
| NDJSON | One record or OTLP export per line |
| OTLP JSON | `resourceMetrics[].scopeMetrics[].metrics[]` |

Generic records use `time`, `timestamp`, `x` or `label` for the horizontal
axis, with numeric columns becoming separate series. Times can be RFC3339 or
seconds. A `name` plus `value` record names a series. Without times, rates use
one-second sample spacing. Records with textual columns render as a table.
Explicit `--format json|csv|ndjson|otlp` overrides detection. Input is bounded
to 1 MiB, 128 series and 4096 points/buckets/rows.

| `--widget` | Display |
|---|---|
| `auto` | Uses the OTLP type; generic numbers use line, mixed records use table |
| `counter`, `sum` | Total and per-second rate line |
| `gauge` | Latest value and sparkline |
| `updowncounter` | Value over time |
| `histogram`, `exponentialhistogram` | Latest distribution and estimated p50/p95/p99 |
| `line`, `bar`, `table`, `stat` | Generic ready-made views |

OTLP resource, scope and point attributes identify distinct streams. Monotonic
sums map to counter; nonmonotonic sums map to updowncounter. Cumulative totals
show the latest observation, delta totals sum the supplied intervals. Rates
use timestamps and delta start times; reset rates need a known start time.
Absent-value points are skipped. Histograms show the latest sample per stream,
not a sum of overlapping cumulative distributions. Percentiles interpolate
within finite buckets and show ≤/≥ bounds for open tails. These are estimates,
not raw-sample percentiles. Semantics follow the
[OTel metrics data model](https://opentelemetry.io/docs/specs/otel/metrics/data-model/).
Legacy Summary metrics are rejected. OTLP integer values use float64 for plot
coordinates; exact integers above 2^53 cannot be represented without rounding.

## Snapshot, refresh and shaping

Snapshot is the default: the source file is read once and its content saved.
`--refresh 5s --input /absolute/file.json` or
`--refresh 5s --input http://127.0.0.1:PORT/metrics` opts in to re-reading while
viewed (or on `show`). Intervals range from 1 second to 24 hours. Stdin cannot
refresh. URLs require a literal loopback IP; redirects, proxies and credentials
are refused. A failed read or parse retains the last good snapshot and shows an
error. Closing stops refresh. The server does not poll unattended items.

Copy `templates/page.html`, edit it, then pass `--template my-page.html` with
`add` or `set --input ...`. The file is captured in the item so later edits do
not unexpectedly change existing pages. Template fields: `.Title`,
`.Description`, `.Widgets` (server-generated SVG). Templates are trusted local
HTML supplied by the owner; ordinary data and titles are escaped. Widgets take
their look from the desk kit tokens (`--desk-ink`, `--desk-panel`,
`--desk-font`) and series from the kit data palette (`--desk-data-1..8`,
`kit.DataPalette`), so a chart and a ```` ```chart ```` embed in notepad or
the viewer share colours in light/dark; standalone SVGs fall back to the
light palette. Tool changes follow the existing Software Shaping
source/version/rollback workflow; no in-page template or layout editor.

## As a shaped app

`shaped.json` declares chart's server, so it can be registered as a
shaped app and get per-user source history, preview, restore and undo:

```bash
bashy app add chart --type shaped --source "$PWD"
```

The console builds that server from each user's active revision and proxies
`/chart/` to it. Item URLs follow the mount: a request that arrives through
the app shell (`X-Forwarded-Prefix`) gets `…/chart/?…`, never the server's
internal loopback address.

## Verify

```sh
bashy dag gate
# Equivalent focused gate:
GOWORK=off go test -count=1 -race .
```

Includes `contracttest.Run`, ten per-widget SVG goldens, one OTLP fixture with
five goldens, numeric semantics, parsing, refresh bounds, escaping, persistence
failure and CLI/HTTP parity tests. To deliberately regenerate reviewed SVG
baselines: `UPDATE_GOLDEN=1 GOWORK=off go test -run 'TestSVGGoldens|TestOTLPFixture' .`.

`browser-test.mjs` is written for the later browser host with Playwright already
installed: it builds an isolated server and checks human creation, SVG, copy,
theme, sharing, revoke and close. It is not run on Dragon.
