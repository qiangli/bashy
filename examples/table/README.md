# Table: a shapeable records grid

This Linux example installs a pinned Apache-2.0 records server and registers
its web UI as a Bashy shaped app. Each paired requester gets a separate server
process and data directory. The source and the built runtime stay under
`BASHY_HOME/table`; no Bashy core code changes are needed.

## Install and run

Requirements: Linux, Bashy, Git, Go, Node 22, and Yarn 1. Installation downloads
source and build dependencies. The build forces the community edition and
rejects extension directories. From this directory:

```sh
bash register.sh
bashy table install
bashy table status
bashy table start
```

`start` runs the paired console in the foreground. In another terminal, run
`bashy app pair --allow table --principal requester --port 22750`; open its URL.
Set `TABLE_CONSOLE_PORT` on both commands to use another port. `table stop`
stops the console started by this command. The server starts on the first
request and may need a refresh while it warms up.

The install verifies the exact fork commit, reads `LICENSE.txt`, rejects any
`ee/`, `premium/`, or `enterprise/` directory, and forces the community build.
The source supplied to the shaping console excludes build dependencies and
development symlinks. The immutable built runtime remains separate from the
managed source. The small Go adapter selects that runtime and a private data
directory for each principal, and injects the revision's `view.css` into HTML.

## Shape a personal view

Use the paired principal's full `device:` identity. The same lifecycle is
available in the web Shape page.

```sh
bashy shape wish table 'Make my records easier to scan' --user device:requester
bashy shape verify table CANDIDATE --user device:requester
bashy shape preview table CANDIDATE --user device:requester --console CONSOLE_ORIGIN
bashy shape approve table CANDIDATE --user device:requester
bashy shape undo table --user device:requester
```

For a deterministic candidate, copy `bashy table source` to another directory,
edit `shape-adapter/view.css`, then pass that directory to `shape wish --from`.
Verification checks the pinned license, community marker, adapter, and source
revision. The console selects the candidate and private data by principal.

## Outbound behavior

App telemetry forwarding, the vendor tag script, and the server's version
lookup routes are disabled in the local source patch. The runtime also forces
telemetry and automatic version checking off. The web UI may offer explicit
links or actions that contact external services; opening them is an owner
action. The command itself uses the network only to install source and packages.
Normal records traffic stays on loopback between the console and the server.

Owner tracing uses OTLP HTTP only if `OTEL_EXPORTER_OTLP_ENDPOINT` is set when
`table start` runs. The adapter exports request method and status without
record contents or identities, accepts W3C `traceparent`, and propagates it to
the local server. No endpoint is configured by default. The console's own
Bashy OTel follows the same owner setting.

The S11 static scan on the clean pinned source reported 1,410 broad matches.
After `web-prefix.patch`, it reported 1,408: 773 application references, 338
locale strings, 289 test references, seven Storybook references, and one binary
test fixture. The application references include local telemetry settings and
event types, update UI and symbols, and support links; these are inert until a
user opens a link or opts into a feature. The vendor telemetry, tag manager,
and version lookup transports are disabled by the patch. Locale and test
references are not runtime calls. The runtime outbound check must be repeated
on the claimed Linux gate host; a prior dev-box container run is not
authoritative evidence.

## Source and checks

The pinned fork is [`qiangli/grist-core-shape`](https://github.com/qiangli/grist-core-shape),
based on [`gristlabs/grist-core`](https://github.com/gristlabs/grist-core), at
`a3c557ac1bc4bd55b9b9d4333842bec255586ff4`. Its actual
[`LICENSE.txt`](https://github.com/gristlabs/grist-core/blob/a3c557ac1bc4bd55b9b9d4333842bec255586ff4/LICENSE.txt)
is Apache-2.0 (SPDX `Apache-2.0`). The upstream repository API reported
12,092 stars and a 2026-11-13 push when checked on 2026-11-13. The pinned
tree has no `ee/`, `premium/`, or `enterprise/` directory. The upstream
package install defaults to downloading an optional non-community extension;
this command explicitly prevents that path.

Targeted command checks: `bash table_test.sh`. The Linux runtime build uses
`GRIST_EDITION=community yarn build:prod` after a frozen lockfile install.

## Data verbs (Sprint 632 A3)

Data commands use Python 3.9+ (standard library only) and the pinned app's
REST API, the service the page itself uses. Set `TABLE_URL` to the **running
instance for the selected principal** (numeric loopback address, port and
optional proxy prefix) and `TABLE_TOKEN` to that principal's existing app API
key. Neither is a Bashy pairing token; the command never logs in, mints or
stores a key. Environment proxies and redirects are disabled so the key cannot
be forwarded elsewhere. Writes only ever call the app API: no database file is
opened, so the app's own access rules, validation and undo history apply.

```sh
bashy table docs list --json
bashy table docs add 'Budget' --workspace 2 --dry-run --json
bashy table tables add DOC People --columns Name,Age:Int --json
bashy table columns add DOC People Email --type Text --label 'E-mail' --json
bashy table records add DOC People --set Name=Ada --set Age=36 --json
bashy table records list DOC People --where Age=36 --sort=-Age --limit 20 --json
bashy table records set DOC People --where Name=Ada --set Age=37 --dry-run --json
bashy table records rm DOC People 4 5 --json
bashy table views list DOC --json
bashy table export DOC People --format csv > people.csv
bashy table import DOC People --file people.csv --json
```

`--json` returns the app response unchanged; app errors (for example the
access-denied result a non-owner gets on the page) pass through byte for byte
with exit 1. Local errors are `{code,message}`: invalid arguments exit 2,
missing key or transport failure exit 1. Writes take `--dry-run`, which prints
`{dryRun,method,url,body}` (or `{dryRun,requests:[...]}` for multi-step verbs)
and performs no I/O. It previews the request, not an authorization decision.

`--where COL=VAL` is equality; repeat a column for any-of. Values parse as JSON
when valid (`3`, `true`), else text. `records set|rm` take row IDs or `--where`,
never both and never neither; with `--where` the matching rows are read through
the API first, then written by row ID. Zero matches write nothing. `views list`
reads the document's view metadata (read-only). `export --format csv` prints the
app's CSV; with `--json` it is `{format,content}`. `import` maps CSV headers to
column IDs or labels, coerces Int/Numeric/Bool cells by column type, and posts
500-row batches; an unknown header aborts before any write. Batches are not one
transaction: a failure part way leaves earlier batches in place. Use `--sort=-COL`
(with `=`) for descending.

Targeted checks: `bash table_test.sh` (16 client tests plus dispatch checks, no
fork or server needed). The fork and the no-phone-home gate are unchanged. The
runtime proof (API calls, denial for a non-owner, outbound traffic) is still
open: no gate-linux run was made for this change.
