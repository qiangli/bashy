# Board: a shapeable web app

This example wraps a pinned, permissively licensed board app (lists,
cards, and a web UI) as a Bashy custom command. It builds the web UI,
bundles the API into a single file with vendor telemetry stubbed out,
imports a local adapter branch into a shaped app, and lets the app
console select a version and data directory for each paired principal.
It targets Linux and the web UI.

## Install and run

Requirements: `bashy`, Go, Node 24 or later, Corepack, a PostgreSQL
toolkit (`initdb`, `pg_ctl`, `postgres`, `psql`), and Git. The install
downloads source and frontend packages. From this directory:

```sh
bash register.sh
bashy board install
bashy board status
bashy board start
```

`start` runs the paired console in the foreground. In another terminal,
use `bashy app pair --allow board --principal requester` to open the
app. Set `BOARD_CONSOLE_PORT` for a different console port; pass that
port to `app pair` as well. `bashy board stop` stops the console
started by this command. A board server is built from the selected
revision and starts on its first request.

The install creates a local `shape-board` branch from the exact fork
pin. Its small adapter changes the web app to a `/board/` base path,
lets the API port come from the environment, and removes the web
analytics loader. The API is bundled to one file with the vendor
telemetry SDKs aliased to a no-op stub, the web UI is built, and both
are committed into the managed source with the server manifest and
declared checks. Each principal gets an isolated Postgres cluster and
auth secret under their own data directory. No app source is added to
Bashy core.

## Shape a personal version

Use the paired principal's full `device:` identity with the CLI. A web
user can run the same actions from `/shape/board/`.

```sh
bashy shape wish board 'Show my cards first' --user device:requester
bashy shape verify board CANDIDATE --user device:requester
bashy shape preview board CANDIDATE --user device:requester --console CONSOLE_ORIGIN
bashy shape approve board CANDIDATE --user device:requester
bashy shape history board --user device:requester
bashy shape undo board --user device:requester
```

For a deterministic candidate, make a copy of the managed source, edit
its frontend and run `bashy shape wish board 'My change' --from COPY
--user device:requester`. The candidate's preview is private to that
principal.

## Shapeable evidence

- **Registered:** `bashy app list --json` reports `board` with a
  `/shape/board/` launch path.
- **Checks:** `checks/board.bsh` runs during verification and fails if
  the pinned license, source marker, API bundle, frontend bundle, or
  server manifest is missing or regressed.
- **Scope:** the console selects a revision and app data by paired
  principal.
- **Undo:** `bashy shape history` records approval and undo; another
  principal keeps the base page.
- **Interface parity:** the web Shape page and `bashy shape` use the
  same candidate lifecycle.

The source is the GitHub fork
[`qiangli/kaneo-shape`](https://github.com/qiangli/kaneo-shape) of
[`usekaneo/kaneo`](https://github.com/usekaneo/kaneo), pinned to
`5069fb56ce309d5572ca3ed60d4d3c322a03a452`. Its actual
[`LICENSE`](https://github.com/qiangli/kaneo-shape/blob/5069fb56ce309d5572ca3ed60d4d3c322a03a452/LICENSE)
is MIT (SPDX `MIT`). The upstream GitHub repository reported 9,798
stars and a 2026-11-13 push when checked through its repository API on
2026-11-13. The pinned tree contains no `ee/`, `premium/`, or
`enterprise/` directory.

Targeted command checks: `bash board_test.sh`. Targeted source checks:
`checks/board.bsh` from the installed source.

## No phone-home

`script/no-phone-home.sh` on the pinned source reports hits in these
groups; each is removed, off by default, or not shipped:

- Web analytics loader (Plausible, `apps/web/index.html`): removed by
  `board-shape.patch`. It only loaded on vendor hostnames before, and
  now the tag is gone entirely.
- Vendor telemetry SDKs (`@sentry/node`, `@sentry/profiling-node`):
  absent from the shipped API bundle. The install bundles the API with
  esbuild aliasing both packages to `sentry-stub.ts` (no-op functions),
  and the wrapper never sets `SENTRY_DSN` or related variables, so the
  SDKs can never initialize.
- Web telemetry SDK (`@sentry/react`): still imported by the web
  bundle, but its `init` runs only when `VITE_SENTRY_DSN` is compiled
  in. The install never sets it, so initialization is skipped and no
  event, breadcrumb, or check-in can send.
- Cron check-ins and error-report calls (`captureCheckIn`,
  `captureException`, `addBreadcrumb`): inert without initialization;
  scheduler failures still log to the owner's own stderr.
- Update checks (`latest-version`, `api.github.com ... /releases`):
  only in end-to-end tests and the marketing site's CLI install
  script. Neither ships with the board app.
- Maintainer tooling (`scripts/provision-sentry-*.sh`,
  `sentry/*.json`, `renovate.json`, CI workflows): not shipped.
- Owner-configured integrations (chat webhooks, Git hosting, SMTP,
  billing, captcha): off unless the owner sets them up; traffic then
  goes only to endpoints the owner chose.
- No remote fonts or CDN scripts in the web app.

A run of the installed app with connection tracking showed only
loopback traffic between its own processes.
