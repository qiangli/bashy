# desk kit — shared contract, UI kit and contract test

K0 deliverable for Sprint 537 (`bashy desk`). One implementation of the
parts every desk tool shares, extracted from `examples/whiteboard` and
`examples/brainstorm`, consumed by each tool via a relative `replace`.
Stdlib-only, same tests on Linux/macOS/Windows. Not a yoke package.

## Placement (confirmed in K0)

`examples/desk/kit`, as recommended in the sprint plan. A yoke package
would couple every tool to the yoke release train and its dependency
closure; a shared example module keeps each tool a standalone Go module
(`require github.com/qiangli/bashy/examples/desk/kit vX.Y.Z`, published to
the local Go module proxy by `../register-modules.sh`), dependency-light
and forkable by Software Shaping per tool or per item template. Desk forms
stay separate from `yoke/pkg/ask`: secrets are refused here (see point 7).

## The common contract (extracted from both servers)

| # | Point | Whiteboard | Brainstorm | Desk rule |
|---|---|---|---|---|
| 1 | Two front doors | `.bsh` serve/status/backup/… + page | same shape | CLI `add\|open\|show\|set\|list\|share\|close` (`--json`) + loopback page |
| 2 | One API | `POST /api/action`, `GET /api/state`, `GET /api/export` | same routes | same routes; `help <tool> --format=mcp` atlas entry |
| 3 | Stable id + URL | board name + `?board=` link; facilitator key private | session + `?session=` link | item id + `?id=` loopback URL; `open` prints id/URL/status only, never content |
| 4 | Any surface | same page everywhere | same page everywhere | same page on Linux/macOS/Windows; linkable, panel-embeddable |
| 5 | Local-first | `$data/boards/*.json`, atomic save, backup/restore | `$data/sessions/*.json`, atomic save, backup/restore | `$BASHY_HOME/desk/<tool>/`; sharing/revoke reuse the board mechanism |
| 6 | Shapable | `.bsh` + Go server + `register.sh` + `dag.md` | same layout | same layout; version history + rollback via Software Shaping |
| 7 | No secrets | facilitator key never in state/export | same | secret-typed or secret-named fields refused (400); credentials stay with `bashy ask`/vault |
| 8 | Tested the same way | Go tests + `browser-test.mjs` + duality check | same | same, plus this shared contract test |

Shared mechanics both servers implement identically (now in `kit.go`):
48-hex tokens, `^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$` names, unknown names 404
(never auto-created on read), explicit create with 409 on duplicates,
facilitator/owner key in `X-Facilitator-Key` with constant-time compare and
revealed only in the create response, HttpOnly `SameSite=Strict`
participant cookies, `DisallowUnknownFields` + single-value strict decoding,
GET/POST only (405 otherwise), same-origin POST check with the
loopback-proxy exception, cross-site `Sec-Fetch-Site` refusal, `no-store` /
`nosniff` / `no-referrer` / self-only CSP with `frame-ancestors 'none'`,
`-data` / `-addr` / `-prefix` flags, temp-file + fsync + rename persistence,
copy-modify-save (a failed save is never acknowledged), `/healthz`.

Deliberate K0 deltas: new tools bind **loopback** (`kit.CheckLoopback`;
whiteboard/brainstorm keep their LAN default as the recorded exception, R1
retrofits them); CLI verbs are the desk seven, not serve/status/backup.

## Contents

- `kit.go` — tokens, names, JSON/Fail, security headers, POST guard, strict
  decoding, participant cookie, atomic write, data dir, stable URLs,
  contentless open view, statuses, secret refusal, prefix strip, loopback
  check.
- `revision.go` — `RevisionStore`, immutable per-item/part snapshots plus an
  `active`/`previous` pointer (`Save` with optional `base_rev`, `Undo`,
  `Restore`, `History`, `Read`, `ServeAction` / `ActionHandler`,
  `WriteRevisionConflict`).
- `ui.go` + `kit.css` + `kit.js` (embedded) — copy button with "Copied"
  state, status badges (`pending`, `needs-human`, `filled-by-agent`,
  `answered`, `failed`, `closed`), field/question components, item header,
  light/dark theme, keyboard-first focus — plus the family look (D1, below).
- `contracttest/conformance.go` — `StyleViolations(page, allow)` /
  `FragmentViolations(markup, allow)`: the kit-conformance half of the
  contract.
- `contracttest/` — `Check(cfg) []string` / `Run(t, cfg)` probing every
  row above over HTTP against `BaseURL` (which must itself be loopback).
  `Config.ViewOnly` is the read-only variant (viewer): `set` must be
  refused (400 / CLI exit 2) before and after close.
- `internal/fixture/` — minimal `memo` tool proving the contract: green on
  the clean server, red on each `-break` variant, plus a CLI/HTTP duality
  check (`open` hides content on both doors).

## One look: tokens, shell, renderer (D1)

Desk tools are parts of bashy and use its conventions; where the kit
differed from bashy, the kit changed. `kit.css` derives its tokens from the
launcher design system (`yoke/pkg/webconsole/artifact/app.css`): font stack
and type scale (`--desk-font`, `--desk-font-mono`, `--desk-text-*`), 4px
spacing (`--desk-space-1..6`), radius, shadow, zinc neutral surfaces and the
launcher blue primary, light and dark. Tool CSS uses only these variables.

**Family accents.** One accent + icon per tool, harmonised with the
launcher primary; `kit.Tools` is the registry and the icon matches the
launcher tile. `<html data-desk-tool="NAME">` selects the accent.

| Tool | Icon | Accent (light / dark) |
|---|---|---|
| notepad | 📝 | blue `#2563eb` / `#60a5fa` |
| viewer | 👁 | indigo `#4338ca` / `#818cf8` |
| whiteboard | 🎨 | teal `#0f766e` / `#2dd4bf` |
| brainstorm | 💡 | amber `#b45309` / `#fbbf24` |
| form | 📋 | emerald `#047857` / `#34d399` |
| chart | 📊 | violet `#6d28d9` / `#a78bfa` |

**Data palette.** `--desk-data-1..8` (`kit.DataPalette`) colours chart
series and ```` ```chart ```` embeds; `.desk-series-N` classes apply it.

**App shell.** `kit.Page(tool, title, body)` + `kit.AppBar(tool, home,
itemHeader, extra)`: the same top bar in the same place — icon + name, item
id, status badge, share/copy, theme toggle. Shared states
(`kit.State("empty"|"error"|"loading", …)`), toasts (`deskToast(msg,
kind)`), the sticky `[data-desk-alert]` (`data-kind="error|ok"`), and the
same shortcuts everywhere: ⌘/Ctrl+S `[data-desk-shortcut=save]`,
⌘/Ctrl+Enter `[data-desk-shortcut=submit]`, `t` theme, `y` copy link, `?`
help. The theme choice is remembered (`localStorage desk-theme`) and
applies before first paint.

**Renderer.** `deskRender(el, text, {inline})` in `kit.js` is the one text
renderer: a thin client for the tool's `POST api/render` endpoint
(`kit.ServeRender`), which runs the shared server renderer
(`desk-kit/render`: goldmark GFM + file-type embeds, bluemonday-sanitized).
There is no client-side Markdown subset; server pages mark text with
`kit.Rendered(tag, mode, text)` / `data-desk-render` and the endpoint swaps
in sanitized HTML. It emits semantic classes only (`desk-content`,
`desk-table`, `desk-code`, `desk-toc`, `desk-embed-*`); kit.css styles them.
Notepad preview, form help, reply reasons, brainstorm cards and whiteboard
notes go through it. ` ```chart ` fences reuse the chart tool's renderer
(`desk-chart/fence`, wired as `render.ChartSVG` by tools that embed charts).

**Conformance (contracttest).** The page must load `kit.css` and the
`.desk-appbar` shell; outside the kit, no hex/rgb/hsl colour and no
font-family in `<style>`, `style=""`, SVG presentation attributes or
inline-script colour/font assignments. Allowlisted user content: the kit
data palette (chart series) and `Config.AllowColors` (whiteboard pens). The
fixture's `-break hex-color|font-family|no-shell` variants prove it red;
every tool's contract test proves it green.

**Evidence.** `examples/desk/gallery/capture.mjs` screenshots every tool in
light and dark into `gallery/index.html` (one side-by-side page).

## Outpost publishing (`outpost.bsh`)

The one `publish-outpost` / `unpublish-outpost` verb pair every catalog app
(notes, form, clips, site, board, whiteboard, table; vlog too) sources:

```bash
source "$here/../desk/kit/outpost.bsh"
publish-outpost|unpublish-outpost) kit_outpost "$1" notes notes "$port" false "${@:2}" ;;
```

`bashy <app> publish-outpost [NAME] [--public|--private] [--icon URL] [--json]
[--dry-run]` upserts `outpost apps add NAME --url
http://127.0.0.1:PORT/NAME/ --require-login`, idempotent on re-run; the
icon defaults to the app's fleet icon (emoji or SVG path) as a data URL.
Apps are "just for me": login required by default; `--public` opts in and
also sets `bashy app set NAME --auth public` (site and vlog default to
public, `--private` opts out). It prints the outpost URL the owner opens
(`<cloudbox>/matrix/h/<host>/app/NAME/`). `unpublish-outpost` runs `outpost
apps rm NAME` and returns a public console record to `--auth system`.
`--dry-run` writes nothing and prints the commands; `--json` prints one
object (errors `{code,message}`, exit 2 usage, 1 otherwise). Test:
`bash examples/desk/kit/outpost_test.sh` (stubbed outpost and bashy).

## Live layer, embedding and the LAN escape hatch (Sprint 696)

Three additive pieces for the `page` app (`docs/page-live-group-page.md`);
no existing tool changes behaviour until it opts in.

**`kit.Events`: one SSE stream per item.** A tool mounts
`GET /api/events?id=ITEM`: it validates the id and access as for
`/api/state` (400 bad id, 404 unknown item), then calls
`events.Serve(w, r, id, item.Revision)`, and calls
`events.Publish(id, block, revision)` after every acknowledged save. Frames
are `{"item","block","revision"}` and never carry content (block `""` means
the whole item); the browser re-fetches only the block that changed.
Revisions, never merges. Pattern from meet (`yoke/pkg/meet/events_sse.go`)
and demo-queue: every frame has `id: <epoch>-<seq>`, so an `EventSource`
reconnects with `Last-Event-ID` and receives exactly the frames it missed;
an id the history can no longer continue (unknown, another server run,
fallen behind `Keep` frames) is answered with one whole-item frame marked
`"resync": true`. A fresh stream opens with one whole-item frame at the
current revision. Keepalive comments every 15 s; write deadlines are
per-frame, so a quiet stream outlives the server's `WriteTimeout`;
`MaxStreams` (256) refuses more with 503 + `Retry-After`. `ContractHandler`
streams an event-stream answer on `GET …/api/events` and still envelopes
that route's refusals.

**Embeddable CSP.** `kit.SecurityOptions{Embeddable: true}.Apply(w)` (or
`.Wrap(h)`) sends the same headers with `frame-ancestors 'self'` instead of
`'none'`: same-origin framing only, for engines a page embeds as a block
under the same console origin. `ApplySecurityHeaders` is unchanged.

**`--lan`: the loopback guard's escape hatch.** `kit.CheckBind(addr, lan,
logw)` is `CheckLoopback` unless the explicit override is given
(`kit.LANFlag(fs)` registers `--lan`); then any well-formed address binds and
the override is logged on start (stderr when `logw` is nil), naming the flag
and the address. An override that cannot be logged refuses to start.

**Contract rows (opt-in, `contracttest.Config`).** `Events: true` — the
stream's status codes, headers, fresh/live/resume/resync frames and that no
frame carries content. `Embeddable: true` — `frame-ancestors` is exactly
`'self'` (otherwise exactly `'none'`, as before; `FramingViolation` is the
pure rule). `BindGuard` — the tool's start-up guard: loopback accepted,
`0.0.0.0`, `:port`, a LAN address and `[::]` refused without `--lan`,
accepted and logged with it, malformed refused. The memo fixture runs all
three green (`memo serve -lan`, `-embeddable`) and red on
`-break events-no-publish|events-no-resume|events-leak-content|embed-unasked|frame-any|lan-open|lan-unlogged`.

## Artifact revisions (Sprint 684)

`kit.RevisionStore` (`revision.go`, shaped like `yoke/pkg/shape/slot/slot.go`)
applies bashy's software-shaping revision model directly to shared desk items
with the same words, pointer mechanics, and JSON as `bashy shape
history|undo|restore`:

| | Shaping a tool (`bashy shape`) | An artifact (`desk/kit`) |
|---|---|---|
| a revision | immutable retained version | immutable snapshot of the authored content |
| current | `active` pointer, with `previous` | same |
| save | draft saved against `base_rev` | `set --base-rev R`; a different active revision refuses with both |
| back one | `shape undo APP` | `APP undo ITEM` |
| to any | `shape restore APP REV` | `APP restore ITEM REV` |
| list | `shape history APP` | `APP history ITEM` |
| JSON | `revision`, `created`, `active`, `previous`, `base_rev` | same fields, plus `author` (and optional opaque `kind` and `assistant`) |

Action and CLI JSON contract (served identically on page and CLI through
`POST /api/action` and `RevisionStore.ServeAction`):

| Verb / Action | Request fields | Response JSON (under `ContractHandler` envelope) | Refusals |
|---|---|---|---|
| `set` (via `Save` / `SaveRevision`) | `id`, optional `part`, optional `base_rev` | tool's `set` response (`Save` returns `{id, revision, author, kind?, assistant?, created, active, previous}`) | stale `base_rev`: `409` (`conflict`) with `base_rev` and `active` (`WriteRevisionConflict`); no `base_rev` is a recorded overwrite |
| `history` | `{"action":"history","id":ITEM}` | `{id, active, previous, versions:[{revision, author, kind?, assistant?, created}, …]}` (newest-first) | `400` invalid id/part |
| `undo` | `{"action":"undo","id":ITEM}` | `{id, revision, author, kind?, assistant?, created, active, previous}` (swaps `active` and `previous`) | `409` (`conflict`) when `previous` is empty |
| `restore` | `{"action":"restore","id":ITEM,"revision":REV}` | `{id, revision, author, kind?, assistant?, created, active, previous}` (selects any retained `REV`) | `400` missing `revision`; `404` (`not_found`) unknown `REV` |

Boundaries: the app chooses which bytes enter a snapshot and decides who may
read or write; `RevisionStore` stores `author`, participant `kind` (`member`,
`signed-in`, `anonymous`, …) and `assistant` as opaque provenance metadata
without checking roles or identity, never owns or modifies the app's item file
(snapshots live beside it under `$data/revisions/<item>/<part>/` with
`state.json` as the single commit point; an unpublished `rN` orphan is
atomically replaced on the next save while listed versions stay immutable), and
implements no merge, no lock, and no roles.
Callers serialize concurrent mutations per item (desk apps already hold their
item mutex across `/api/action`).

## Use in a tool

The kit is a versioned module served by the local Go module proxy, never a
sibling path: a tool builds from its own directory alone (the console builds a
revision's tree with nothing beside it). `../register-modules.sh` publishes
the kit and chart versions it names (`bashy mod proxy publish`); a kit change
ships under a new version there, and the tools move to it.

```bash
# in the tool server module, after ../register-modules.sh:
export GOPROXY="file://${BASHY_GOMOD_PROXY:-$HOME/.bashy/gomod-proxy},https://proxy.golang.org,direct"
export GONOSUMDB=github.com/qiangli/bashy/examples
go get github.com/qiangli/bashy/examples/desk/kit@v0.2.0
```

```go
import kit "github.com/qiangli/bashy/examples/desk/kit"
import "github.com/qiangli/bashy/examples/desk/kit/contracttest"
```

## Verify

```bash
cd examples/desk/kit && GOWORK=off go test ./...
```

No podman, no browser: unit + contract only. Tool repos add their own
`browser-test.mjs` and duality checks on top.
