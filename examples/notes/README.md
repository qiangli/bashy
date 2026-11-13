# Notes: a shapeable web app

This example wraps a pinned, permissively licensed Go notes app as a Bashy
custom command. It builds the web UI, imports a local adapter branch into a
shaped app, and lets the app console select a version and data directory for
each paired principal. It targets Linux and the web UI.

## Install and run

Requirements: `bashy`, Go 1.27.2, Git, Node 24 or later, and Corepack. The
install downloads source and frontend packages. From this directory:

```sh
bash register.sh
bashy notes install
bashy notes status
bashy notes start
```

`start` runs the paired console in the foreground. In another terminal, use
`bashy app pair --allow notes --principal requester` to open the app. Set
`NOTES_CONSOLE_PORT` for a different console port; pass that port to `app pair`
as well. `bashy notes stop` stops the console started by this command. A notes
server is built from the selected revision and starts on its first request.

The install creates a local `shape-notes` branch from the exact fork pin. Its
small adapter adds the server manifest, a version for source builds without Git
metadata, a `/notes/` frontend prefix, the built frontend, and declared checks.
The source is then imported into Bashy's managed checkout. No app source is
added to Bashy core.

## Shape a personal version

Use the paired principal's full `device:` identity with the CLI. A web user
can run the same actions from `/shape/notes/`.

```sh
bashy shape wish notes 'Make my notes page easier to scan' --user device:requester
bashy shape verify notes CANDIDATE --user device:requester
bashy shape preview notes CANDIDATE --user device:requester --console CONSOLE_ORIGIN
bashy shape approve notes CANDIDATE --user device:requester
bashy shape history notes --user device:requester
bashy shape undo notes --user device:requester
```

For a deterministic candidate, make a copy of the managed source, edit its
frontend and run `bashy shape wish notes 'My change' --from COPY --user
device:requester`. The candidate's preview is private to that principal.

## Shapeable evidence

- **Registered:** `bashy app list --json` reports `notes` with a
  `/shape/notes/` launch path.
- **Checks:** `checks/notes.bsh` runs during verification and fails if the
  pinned license, source marker, frontend bundle, or server adapter is missing.
- **Scope:** the console selects a revision and app data by paired principal.
- **Undo:** `bashy shape history` records approval and undo; another principal
  keeps the base page.
- **Interface parity:** the web Shape page and `bashy shape` use the same
  candidate lifecycle.

The source is the GitHub fork
[`qiangli/memos-shape`](https://github.com/qiangli/memos-shape) of
[`usememos/memos`](https://github.com/usememos/memos), pinned to
`d0c6e2a7afc6de17220f9b439bb077a5aa12f83f`. Its actual
[`LICENSE`](https://github.com/qiangli/memos-shape/blob/d0c6e2a7afc6de17220f9b439bb077a5aa12f83f/LICENSE)
is MIT (SPDX `MIT`). The upstream GitHub repository reported 64,437 stars
and a 2026-11-13 push when checked through its repository API on 2026-11-13.
The pinned tree contains no `ee/`, `premium/`, or `enterprise/` directory.

Targeted command checks: `bash notes_test.sh`. Targeted source tests:
`GOWORK=off CGO_ENABLED=0 go test ./cmd/memos ./server/frontend` from the
installed source.
