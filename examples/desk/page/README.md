# page — one fixed, live page for a group

Design of record in the umbrella's
`docs/page-live-group-page.md`. A page is one desk item: Markdown written
by the organizer plus ` ```block ` fences, an audience, the participants and
a revision counter. Each block is a live view over an item another desk app
owns (a form poll, a viewer book page, a notepad note, ...) or a page-local
widget (timer, presence, hands). The engines are not merged: composition is
over HTTP at run time.

## Item

```markdown
# Sunday class

Agenda and links ...

```block poll:form-9f3a
```

~~~block
embed:/whiteboard/?room=r1 id=board
~~~
```

- **Block line**: `kind:ref [id=NAME] [key=value ...]`, as the fence's info
  string or the first line of its body. Kinds (`blocks.go`): `poll form book
  note board chart` (ref = the owning app's item), `timer` (optional ref),
  `here hands` (no ref), `embed` (a same-origin path or an http(s) URL).
  Unknown kinds, missing refs, `javascript:`/`//host` embeds, duplicate ids
  and unclosed fences are refused (400 / exit 2) before anything is stored.
  Default ids are `KIND-N`; the live stream addresses blocks by id.
- **Audience**: `host` (default) and `satellite` pages are served to this
  host only (the outpost tunnel terminates here, login required); a `lan`
  page is also served to LAN peers when the server runs with `--lan`.
- **Participants**: `{handle, principal, address, fingerprint, cookie, auth,
  joined_at, last_seen}`. The public record carries handle and auth. An
  anonymous participant reclaims the same handle from the same address after
  losing the cookie. Set `--handle-conflicts=suffix` in `shaped.json` under `server.args` to
  suffix a handle already bound to a different address.
- **Revisions**: every acknowledged save bumps `revision` and publishes a
  `{item, block, revision}` frame on `GET /api/events?id=` (`kit.Events`).
  Sprint 684 G3 revisions replace the counter when they land.

## Verbs

`page add [-f FILE|-] [--title T] [--audience host|lan|satellite] [--id ID] [--code CODE]`,
`open ID` (join links, QR codes and the bare code), `show ID`, `set ID [-f FILE|-] [--title T]
[--audience A]`, `list`, `share ID` (single-use invite), `close ID`,
`export ID --to md|html [-o FILE]`, `serve [--lan] [--port PORT]`,
and `publish-outpost [--port PORT]` / `unpublish-outpost`. Every verb takes `--json`, `--fields`,
`--explain`, `--dry-run`, `--help=mcp`. `set`, `close`, `share` and
`export` need the organizer's owner key: `--key`, `PAGE_KEY`, or the local
store (`$BASHY_HOME/desk/page/items/<id>.json`).

Use `page publish-outpost --port PORT` before `page serve --lan --port PORT`
so both doors reach the same fixed listener and it can load the app's
outpost secret. Create the page with `--audience lan` to admit LAN peers.
The browser root accepts a join code. The legacy `/?id=ID` link is also
supported. Signed-in outpost visitors use their app-specific pseudonym.

The organizer can run `page participant ID list`, `rename HANDLE NAME`,
`merge HANDLE INTO`, `kick HANDLE` or `--open` to admit duplicate handles.
The full participant list requires the owner key. All four changes are logged.

The default outpost Unix tunnel currently omits the forwarded client address
(outpost follow-up `a1333108286e`). Anonymous requests on a verified tunnel
need that address to join. Signed-in participants can enter by principal.

## Later Sprint 696 stories

Native and embed block renderers, roles and views
(organizer face), presence and hands, export with block states inline, PDF.
Until a kind's renderer lands its block renders as a labelled placeholder.

## Verify

```bash
cd examples/desk/page && GOWORK=off go test ./...
```

Desk contract (with the live-layer and LAN-guard rows) green, red on each
`-break` variant; block parsing; CLI round trip; audience gate; organizer-only
writes and export; no participant address in show, state, page or export.
