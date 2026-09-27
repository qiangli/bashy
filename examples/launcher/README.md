# A launcher you can replace

This plain HTML example needs no framework, build step or dependencies. From
this repository, run:

```sh
bashy app serve --launcher examples/launcher
```

Open the address printed by bashy. The page lists stock and registered apps
alphabetically, adds a local HTTP server by name and port, and removes registered
apps. Start that server separately: registration never executes a program.

For a supervised launcher:

```sh
bashy app service start --launcher examples/launcher
```

Service start is a no-op when the daemon is already running. Stop it first
with `bashy app service stop` to change its launcher, then start it with the
new directory.

The service stores the absolute directory in `service.profile.json`; a later
bare service start reuses it. Keep the directory on disk. Pair/bind/port flags preserve the saved launcher; `--launcher ''` selects the
stock launcher.

## The contract

`GET /api/apps` returns the same projection as `bashy app list --json`:

```json
{"schema_version":"bashy-console-apps-v1","base":"/","apps":[
  {"name":"notes","label":"Notes","path":"/notes/","mode":"proxy",
   "port":8080,"auth":"system","source":"registered",
   "available":true,"status":"ready","start_hint":""}
]}
```

Rows include stock apps (`source` is `builtin` or `atlas`) and operator apps
(`registered`). `status` is `ready`, `stopped` or `unavailable`; liveness is cached
for three seconds. A row can also include `icon`, `tip`, `start`, `login_path` or
an unavailable `note`. Use `name` as its identity; presentation and ordering
belong to your launcher.

The server inserts `<base href>` into `index.html` for its current mount.
Build URLs relative to `document.baseURI`, as this example does: `api/apps`,
`api/apps/registered`, and an app's path with its leading slash removed.
This works at `/` and behind a host/tunnel prefix. Keep a literal `<head>` tag
or `<base href="...">` in your HTML so the server can insert or replace it.

`POST api/apps/registered` requires `Content-Type: application/json` and accepts a JSON record containing `name`, `port` and
optional `label`, `icon`, `tip`, `start` (argv array), `auth` or `login_path`.
It validates and saves through the same fleet store as `bashy app add/set`.
The default auth tier is `system`; selecting `public` or `custom` is operator
policy, just as with the CLI. POST creates or updates a record and returns
201 with its name. `DELETE api/apps/registered/<name>` removes a local record
and returns 204. Shared read-only records cannot be deleted here. Stock and
reserved mounts, unsafe fields and invalid ports are refused. Changes appear
in the list and proxy routes immediately, including changes made by the CLI.

The launcher, API, stock panels and proxies retain bashy's auth gate. Local
loopback access follows the existing development rule; a LAN listener requires
system login, and tunnel access requires its existing trusted identity.
Registry writes require an operator session; scoped paired devices cannot
change registration. A public/custom app opens only its own mount, never this
API. Failed requests appear on the example page as text.

## Grow your launcher

Edit the HTML and inline script; reload to see changes. Add search, styling or
icons using the same rows. Put additional files under `assets/` or `static/`,
which remain accessible to scoped devices. Existing stock asset URLs such as
`app.css`, `app.js`, `board.js`, `inbox.js`, `mb.js`, `term.js` and `vendor/`
are reserved for the stock panels. Explicit panel/API/proxy routes also take
precedence over launcher files. With no `--launcher`, bashy serves its stock UI.
