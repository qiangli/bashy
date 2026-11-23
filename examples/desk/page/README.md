# page — one fixed, live page for a group

Sprint 696 page skeleton (design of record: the umbrella's
`docs/page-live-group-page.md`). A page is one desk item: Markdown written
by the organizer plus ` ```block ` fences, an audience, the participants and
a revision counter. Each block is a live view over an item another desk app
owns (a form poll, a viewer book page, a notepad note, ...) or a page-local
widget (timer, presence, hands). The engines are not merged: composition is
over HTTP at run time; this module imports only the desk kit.

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
  joined_at, last_seen}`, filled by the join story. `show`, `state`, the page
  and `export` carry handle and auth only; addresses never leave the store.
- **Revisions**: every acknowledged save bumps `revision` and publishes a
  `{item, block, revision}` frame on `GET /api/events?id=` (`kit.Events`).
  Sprint 684 G3 revisions replace the counter when they land.

## Verbs

`page add [-f FILE|-] [--title T] [--audience host|lan|satellite] [--id ID]`,
`open ID` (id, URL, status only), `show ID`, `set ID [-f FILE|-] [--title T]
[--audience A]`, `list`, `share ID` (single-use invite), `close ID`,
`export ID --to md|html [-o FILE]`, `serve [--lan]` (default
127.0.0.1:18113), `publish-outpost` / `unpublish-outpost` (kit
`outpost.bsh`, `--require-login`). Every verb takes `--json`, `--fields`,
`--explain`, `--dry-run`, `--help=mcp`. `set`, `close`, `share` and
`export` need the organizer's owner key: `--key`, `PAGE_KEY`, or the local
store (`$BASHY_HOME/desk/page/items/<id>.json`).

## Not yet (later Sprint 696 stories)

Join codes and handles, native and embed block renderers, roles and views
(organizer face), presence and hands, export with block states inline, PDF.
Until a kind's renderer lands its block renders as a labelled placeholder.

## Verify

```bash
cd examples/desk/page && GOWORK=off go test ./...
```

Desk contract (with the live-layer and LAN-guard rows) green, red on each
`-break` variant; block parsing; CLI round trip; audience gate; organizer-only
writes and export; no participant address in show, state, page or export.
