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

The pin carries no phone-home of its own, so `web-prefix.patch` only relocates
the app: it points the scene-storage and collaboration endpoints at the local
`/whiteboard` service and sets the frontend base prefix. The third-party
telemetry, Firebase backend, analytics, remote fonts and Excalidraw+ paths the
upstream ships are removed in the fork itself — see
`../../../docs/whiteboard-fork-no-phone-home.md`.

## Shape a personal version

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
`1304220cc539d902dfce104bac6cd7c946d9d591` on branch `no-phone-home`, which is
three commits on top of tag `v0.18.1` (`a2ec2889babf7d2295469c6d90ebe77fae57df84`).
Its actual
[`LICENSE`](https://github.com/qiangli/excalidraw-shape/blob/1304220cc539d902dfce104bac6cd7c946d9d591/LICENSE)
is MIT (SPDX `MIT`), unchanged from upstream. The upstream GitHub repository
reported 135,622 stars and a push on 2026-11-12 when checked through its
repository API on 2026-11-13. The pinned tree contains no `ee/`, `premium/`,
or `enterprise/` directory.

Targeted command checks: `bash whiteboard_test.sh`. Targeted source tests:
`GOWORK=off CGO_ENABLED=0 go test ./server/...` from the installed source.
