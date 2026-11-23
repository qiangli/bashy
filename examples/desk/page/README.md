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
  `here hands` (no ref), `embed` (optional participant URL, a same-origin
  path or an HTTP(S) URL).
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

## Poll and form blocks (native over the form app)

`page block add poll PAGE --options a,b,c [--title Q] [--votes N]` and
`page block add form PAGE --def FILE|-` (JSON; `[--title T]`) create the
item in the form app over its API (`PAGE_FORM_URL`, default
`http://127.0.0.1:18107`; start it with `form serve`) and append the
` ```block poll:FORM id=BLOCK ` fence. Common options: `--block NAME`,
`--anonymous`, `--results-visible`, `--closed`. Form's owner key stays in
the page store; it never appears in show, state, the page or export.

- **Identity is the respondent.** The page holds one opaque form respondent
  token per page participant (its cookie jar towards form), so each identity
  on either door is one respondent: a resubmit or a re-vote after a refresh
  replaces, never adds. `--votes N` is the budget of distinct options per
  identity. A rename or merge moves the response with the identity; a kicked
  participant is no longer counted. Only page participants are counted.
- **Organizer controls**: `page block open|close|show-results|hide-results
  PAGE BLOCK` (page-side gates, logged; form's own close is final and is
  not used) and `page block results PAGE BLOCK [--json]`: `option -> votes
  (handles)` and `handle -> answer`. `--anonymous` drops handles from the
  results view, the organizer face and the export.
- **Faces**: the owner key gets the results view; an enrolled participant
  gets the ballot or form with their own answer, and counts (never handles)
  when results are visible. `GET /api/block?id=&block=` returns one block's
  HTML for the caller; every answer and control publishes the block's live
  frame, and the page re-fetches only that block.
- **Record**: `export --to md|html` writes each block's tally with handles
  and the per-handle answers.

File fields are not offered on a page (`--def` with a `file` field is
refused); YAML definitions go through `form` itself.

## Content and timer blocks

- `book:<id>/pN` renders that viewer book page inline; `note:<id>` renders
  that notepad note inline (`VIEWER_ADDR`, `NOTEPAD_ADDR` locate the apps).
  An unreachable app or missing item shows a short notice, never an error page.
- `embed:<same-origin path or URL>` frames a desk-kit app that opted into
  same-origin framing. Engines that refuse framing (`/whiteboard`, `/table`,
  `/klatch`, `/demo-queue`, also under `/apps/`) and any block with
  `mode=link` (or `link_only` face settings) render a link-out card instead.
- `timer:<name> duration=5m` is a countdown. Only the organizer face has
  Start, Stop and Reset; every face sees it tick over the live stream. Give it
  `"project": true` face settings to show the bare clock on `?view=display`.
  The export records its state.

## Later Sprint 696 stories

Presence and hands, export with block states inline, PDF.
Until a kind's renderer lands its block renders as a labelled placeholder.

## Organizer and display views

`page add` returns a private organizer link alongside the regular join link.
Open that organizer link to see available faces side by side or in tabs.
**View as participant** uses a separate preview identity. Add
`?view=display` to the join link to open the projector page. Only the
configured display faces appear there. Preview answers stay in server
memory and never become form responses. The organizer can open or close
native forms and polls and control results visibility from the Organizer tab.

Put only the participant URL in a Markdown block, for example
`embed:/brainstorm/?session=ideation id=ideas`. A block with only organizer
and display faces can use `embed id=demos`. Import the corresponding private
settings using `page add -f agenda.md --faces faces.json` or
`page set ID --faces faces.json`:

```json
{
  "ideas": {"organizer_url": "/brainstorm/?session=ideation#facilitator=EXAMPLE"},
  "demos": {
    "organizer_url": "http://demo.example/control#ORGANIZER_EXAMPLE",
    "display_url": "http://demo.example/projector#DISPLAY_EXAMPLE",
    "link_only": true
  }
}
```

A settings entry replaces the settings for that block. Use `null` to clear
an entry. Settings stay in the page store. Keep the import file private.
To project a poll tally, give its block id a settings entry with
`"project": true`.
Serve Brainstorm under the same origin as Page. The organizer iframe needs
browser support for isolated iframe storage. Its separate-tab link works
without that support. Demo Queue refuses framing, so use `link_only` for
its organizer and projector links.

## Verify

```bash
cd examples/desk/page && GOWORK=off go test ./...
```

Desk contract (with the live-layer and LAN-guard rows) green, red on each
`-break` variant; block parsing; CLI round trip; audience gate; organizer-only
writes and export; no participant address in show, state, page or export. The poll/form tests build and
run the real form app (`../form/server`, never imported) on a loopback port:
a three-vote ballot from four identities on both doors, one refreshing and
reclaiming, counted once; open/close/visibility; anonymous; per-handle form
answers through rename and kick; form unreachable.
