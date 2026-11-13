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

## Data verbs (Sprint 632 A2)

Data commands use Python 3.9+ (standard library only) and the pinned app's
Connect JSON API, the same service used by the page. Set `NOTES_URL` to the
**running instance for the selected principal**, using its numeric loopback
address, port and optional proxy prefix. Set `NOTES_TOKEN` to that principal's
existing app access token. These are not a Bashy pairing token or a new CLI
identity. The command does not log in, mint, refresh or store tokens. An expired
token gets the app's unauthenticated result; renew it through the existing app.
A paired console also has its own device authentication: use the selected app
instance's loopback backend URL, not an unauthenticated console URL.

```sh
bashy notes list --page-size 20 --json
bashy notes show memos/ID --json
bashy notes add --content 'Meeting notes' --tags work --visibility private --dry-run --json
bashy notes set ID --content 'Revised notes' --tags work/project --json
bashy notes set ID --visibility protected --pinned true --json
bashy notes archive ID --dry-run --json
bashy notes unarchive ID --json
bashy notes rm ID --dry-run --json
bashy notes search 'meeting' --filter '"work" in tags' --json
bashy notes export ID --format markdown
bashy notes export ID --format json
```

All data verbs take `--json`; writes take `--dry-run`. ID also accepts
`memos/ID`. `list` and `search` return one API page; continue with
`--page-token` from `nextPageToken`, preserving filters and ordering. Use
`--state archived` for the archive, `--order-by 'update_time desc'` for order,
and `--space spaces/ID` on add/set for placement. An empty `--space ''`
removes placement. The server decides membership and visibility rules.

Tags are output-only in the API, extracted from Markdown. Edit/remove tags by
replacing `--content`, just as in the page editor. `--tags a,b` is a convenience
that appends `#a #b` to that explicit content; it never strips hashtags from
existing text or guesses how to rewrite code blocks. Hence `set --tags`
requires the full replacement `--content`.

`--json` preserves the app response object without wrapping it: a Memo for
show/add/set/archive/unarchive/export, `{memos,nextPageToken}` for list/search,
and `{}` for rm. App errors (including non-owner `permission_denied` and
inaccessible private-note `not_found`) pass through unchanged, with exit 1.
Local errors use the same `{code,message}` shape; invalid arguments exit 2,
credential/transport failures exit 1. `--json` takes precedence over Markdown
export: it returns the original Memo JSON, whose `content` holds the Markdown.
Export is one memo's content/JSON, not the settings page's full backup ZIP.

Dry-run produces `{dryRun:true,method:"POST",url,body}` without contacting the
app or reading a token. It previews the request, not an authorization decision.
The app remains the authority for ownership, administrative access, validation,
and deletion of comment descendants. Credentials never appear in a manifest.
Only numeric loopback endpoints are allowed; environment proxies and redirects
are disabled so a local token cannot be forwarded to another destination.

Run `bash examples/notes/notes_test.sh` from the Bashy repository. It includes
13 isolated client tests and Bash# dispatch checks, requiring no fork or server.
The fork and the no-phone-home gate are unchanged. The current source pin's
static scan returns 10 explained hits: five indirect OTel module references
(no exporter setup in application Go source), three opt-in upstream installer
version-resolution lines (our installer uses an exact Git pin), one demo-only
seed text containing the word telemetry, and one migrator comment. Demo seed
content includes external sponsor images and is not used by the normal app;
this is not evidence for a demo-mode offline guarantee. Runtime API/denial and
outbound-traffic proof remain open: the claimed Linux gate was reachable but
had no running notes instance or prepared notes image. No app was run on the
dev box, and the gate claim was released.
