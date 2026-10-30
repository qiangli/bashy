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
| 1 | Two front doors | `.bsh` serve/status/backup/… + page | same shape | CLI `new\|open\|get\|put\|list\|share\|close` (`--json`) + loopback page |
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
  light/dark theme, keyboard-first focus.
- `contracttest/` — `Check(cfg) []string` / `Run(t, cfg)` probing every
  row above over HTTP against `BaseURL` (which must itself be loopback).
- `internal/fixture/` — minimal `memo` tool proving the contract: green on
  the clean server, red on each `-break` variant, plus a CLI/HTTP duality
  check (`open` hides content on both doors).

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
