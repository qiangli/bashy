# Whiteboard: a shapeable visual collaboration web app

This example wraps a pinned, permissively licensed collaborative canvas as a Bashy
custom command. It builds the web UI, imports a local adapter into a shaped app,
and lets the app console select a version and data directory for each paired principal.
It folds in LAN whiteboard sketching and brainstorming canvas workflows. It targets
Linux and the web UI.

## Install and run

Requirements: `bashy`, Go 1.27.2, Git, Node, and Yarn. The install downloads
source and frontend packages. From this directory:

```sh
bash register.sh
bashy whiteboard install
bashy whiteboard status
bashy whiteboard start
```

`start` runs the paired console in the foreground. In another terminal, use
`bashy app pair --allow whiteboard --principal requester` to open the app. Set
`WHITEBOARD_CONSOLE_PORT` for a different console port; pass that port to `app pair`
as well. `bashy whiteboard stop` stops the console started by this command. The
whiteboard server starts on its first request.

The install creates a local `shape-whiteboard` branch from the exact fork pin. Its
adapter adds the server manifest, a `/whiteboard/` frontend base prefix, the built
frontend bundle, a small collab/storage service with standard owner-only bashy OTel,
and declared checks. The source is then imported into Bashy's managed checkout.
No app source is added to Bashy core.

## Data verbs (Sprint 632 A5)

The command also speaks to the running local whiteboard service, so scripts get
the same server-backed actions as the page. Every verb takes `--json`; writes
take `--dry-run`. Point `WHITEBOARD_URL` at the running principal instance's
numeric loopback URL with port (optional app proxy prefix); `--url` overrides
it. The service has no per-board credential: loopback reachability plus the
paired console is the access boundary, exactly as on the page.

```sh
whiteboard boards --json
whiteboard new my-board --note 'Kickoff' --json
whiteboard show my-board --json
whiteboard set my-board --note 'Decided: teal theme' --json
whiteboard lock my-board --json
whiteboard share my-board --json
whiteboard save --file ./scene.bin --json
whiteboard load 0123456789abcdef --output ./scene.bin --json
whiteboard upload rooms/my-board/logo --file ./logo.png --json
whiteboard download rooms/my-board/logo --output ./logo.png --json
whiteboard export my-board --scene 0123456789abcdef --output ./board.json
whiteboard rm my-board --json
```

| Page action | Verb or recorded reason |
|---|---|
| List boards | `whiteboard boards --json` |
| Create board | `whiteboard new [NAME] [--note TEXT]` (name omitted: the service mints the id) |
| Open / show board | `whiteboard show ID` (auto-initializes, exactly as opening the page does) |
| Add a board note | `whiteboard set ID --note TEXT` (appends, bumps the revision, as on the page) |
| Lock board | `whiteboard lock ID` (one-way close; the page has no unlock) |
| Remove board | `whiteboard rm ID` (missing board is a `not_found` denial, as on the page) |
| Share / collab room link | `whiteboard share ID` (read-only; prints the `#room=` link the page opens) |
| Save scene buffer | `whiteboard save --file PATH` (returns the scene id the page embeds in its link) |
| Load scene buffer | `whiteboard load SCENE-ID` (raw bytes, as the page fetches them) |
| Upload asset file | `whiteboard upload PATH --file LOCAL` |
| Download asset file | `whiteboard download PATH [--output LOCAL]` |
| Export board JSON | `whiteboard export ID [--scene SCENE-ID]` (board record, plus base64 scene bytes) |
| Export PNG / SVG | Page-only: the browser renders the canvas; no server API exists |
| Freehand drawing, shapes, text, move/resize | Page-only: client-side canvas state, ephemeral Socket.IO diffs; no server REST API |
| Clear canvas, zoom, pan, grid, theme | Page-only: client-side canvas or display preference; no server state |

# Shape a personal version

Use the paired principal's full `device:` identity with the CLI. A web user
can run the same actions from `/shape/whiteboard/`.

```sh
bashy shape wish whiteboard 'Make the canvas background darker' --user device:requester
bashy shape verify whiteboard CANDIDATE --user device:requester
bashy shape preview whiteboard CANDIDATE --user device:requester --console CONSOLE_ORIGIN
bashy shape approve whiteboard CANDIDATE --user device:requester
bashy shape history whiteboard --user device:requester
bashy shape undo whiteboard --user device:requester
```

For a deterministic candidate, make a copy of the managed source, edit its
frontend or configuration, and run `bashy shape wish whiteboard 'My change' --from COPY --user device:requester`.
The candidate's preview is private to that principal.

## Shapeable evidence

- **Registered:** `bashy app list --json` reports `whiteboard` with a
  `/shape/whiteboard/` launch path.
- **Checks:** `checks/whiteboard.bsh` runs during verification and fails if the
  pinned license, source marker, frontend bundle, server adapter, or no-phone-home
  invariants are missing.
- **Scope:** the console selects a revision and app data by paired principal.
- **Undo:** `bashy shape history` records approval and undo; another principal
  keeps the base page.
- **Interface parity:** the web Shape page and `bashy shape` use the same
  candidate lifecycle.

The source is the GitHub fork
[`qiangli/excalidraw-shape`](https://github.com/qiangli/excalidraw-shape) of
[`excalidraw/excalidraw`](https://github.com/excalidraw/excalidraw), pinned to
`a2ec2889babf7d2295469c6d90ebe77fae57df84` (tag `v0.18.1`). Its actual
[`LICENSE`](https://github.com/qiangli/excalidraw-shape/blob/a2ec2889babf7d2295469c6d90ebe77fae57df84/LICENSE)
is MIT (SPDX `MIT`). The upstream GitHub repository reported 135,622 stars
and a push on 2026-11-12 when checked through its repository API on 2026-11-13.
The pinned tree contains no `ee/`, `premium/`, or `enterprise/` directory.

Targeted command checks: `bash whiteboard_test.sh`. Targeted source tests:
`GOWORK=off CGO_ENABLED=0 go test ./server/...` from the installed source.
