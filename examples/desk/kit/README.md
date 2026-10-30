# desk kit — shared contract, UI kit and contract test

K0 deliverable for Sprint 537 (`bashy desk`). One implementation of the
parts every desk tool shares, extracted from `examples/whiteboard` and
`examples/brainstorm`, consumed by each tool via a relative `replace`.
Stdlib-only, same tests on Linux/macOS/Windows. Not a yoke package.

## Placement (confirmed in K0)

`examples/desk/kit`, as recommended in the sprint plan. A yoke package
would couple every tool to the yoke release train and its dependency
closure; a shared example module keeps each tool a standalone Go module
(`require desk-kit v0.0.0` + `replace desk-kit => ../kit`), dependency-light
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
- `ui.go` + `kit.css` + `kit.js` (embedded) — copy button with "Copied"
  state, status badges (`pending`, `needs-human`, `filled-by-agent`,
  `answered`, `failed`, `closed`), field/question components, item header,
  light/dark theme, keyboard-first focus — plus the family look (D1, below).
- `contracttest/conformance.go` — `StyleViolations(page, allow)` /
  `FragmentViolations(markup, allow)`: the kit-conformance half of the
  contract.
- `contracttest/` — `Check(cfg) []string` / `Run(t, cfg)` probing every
  row above over HTTP against `BaseURL` (which must itself be loopback).
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
renderer: a safe Markdown subset built with DOM APIs (headings, lists,
quotes, code, pipe tables, inline marks, links, ```` ```chart ```` fences).
It emits semantic tags only; `.desk-content` styles them. Server pages mark
text with `kit.Rendered(tag, mode, text)` / `data-desk-render`. Notepad
preview, form help, reply reasons, brainstorm cards and whiteboard notes go
through it.

**Conformance (contracttest).** The page must load `kit.css` and the
`.desk-appbar` shell; outside the kit, no hex/rgb/hsl colour and no
font-family in `<style>`, `style=""`, SVG presentation attributes or
inline-script colour/font assignments. Allowlisted user content: the kit
data palette (chart series) and `Config.AllowColors` (whiteboard pens). The
fixture's `-break hex-color|font-family|no-shell` variants prove it red;
every tool's contract test proves it green.

**Evidence.** `examples/desk/gallery/capture.mjs` screenshots every tool in
light and dark into `gallery/index.html` (one side-by-side page).

## Use in a tool

```bash
# in the tool server module:
go mod edit -require=desk-kit@v0.0.0 -replace=desk-kit=../kit
```

```go
import kit "desk-kit"
import "desk-kit/contracttest"
```

## Verify

```bash
cd examples/desk/kit && GOWORK=off go test ./...
```

No podman, no browser: unit + contract only. Tool repos add their own
`browser-test.mjs` and duality checks on top.
