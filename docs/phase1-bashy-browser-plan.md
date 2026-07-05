# Phase 1 — `bashy browser` (lined up)

Port ycode's browser subsystem into coreutils so `bashy browser` is a
first-class, self-contained verb — the "known entry" that unblocks browser-gated
third-party tools (the conductor-steering surface, Layer 3 of
`docs/conductor-steering-surface.md`). Phase 0 (`weave fleet --auth`) already
*detects* a browser-login gate; Phase 1 lets the conductor *clear* it.

## What exists in ycode (surveyed)

- `pkg/browser/wire` — the wire protocol (`Action`/`Result`), **stdlib-only**.
- `internal/runtime/browser/client.go` — a transport-shaped `Client` interface
  (`Execute(action) → result`); backend injected via context.
- `internal/runtime/mcpservers/{probe,solo,live}` — the three engines:
  - **probe** — CDP-attach (via `chromedp`) to a Chrome the user started with
    `--remote-debugging-port`. Reuses the user's real session/cookies. **Best for
    OAuth login.** Zero bundled browser.
  - **solo** — `chromedp` launches a fresh isolated Chrome discovered on the
    system (`google-chrome`/`chromium`/…). Full headless automation.
  - **live** — a Chrome *extension* talking to the user's live tab.
- `internal/runtime/mcpservers/browsermcp/mcpserver.go` — 21 `browser_*` verbs
  (navigate, click, type, eval, extract, cookies_get, storage_get, screenshot,
  keyboard_press, scroll, tabs, wait_for_selector, back, clipboard_*, perf_*,
  capabilities …).
- `internal/shell/builtins/browser.go` — the `browser` shell builtin (the
  `bashy browser` precedent): "open, fetch, find; probe mode if reachable, HTTP
  fallback for fetch."

## Dependency & license verdict

- **`github.com/chromedp/chromedp` + `cdproto` + `sysutil` are all MIT** →
  permissive + **pure-Go (no cgo)** → allowed as a compiled-in coreutils dep per
  `docs/licensing-supply-chain-policy.md`. Record in `THIRD_PARTY_LICENSES`.
- **No bundled Chrome.** probe mode uses the user's *already-running* browser
  (zero install, and it carries their logins — exactly what OAuth needs). solo
  mode uses a Chrome/Chromium found on `$PATH`. A downloaded headless-shell
  (download-exec, not vendored) is a later convenience, not required for Phase 1.
- Adds `chromedp` to the **default (lean) `cmd/bashy`** build — it's pure-Go and
  cross-compiles CGO-free, so it stays in the core worker (not an ext build-tag).
  Size impact: cdproto is large; if `bin/bashy` grows too much, gate the engine
  behind `-tags bashy_browser` and fall through to a PATH `chromium`/HTTP-fetch
  like the current builtin already does.

## Surface

- `bashy browser <action> [args] [--json]` — the 21 verbs, driven by the mode
  configured (probe > solo > http-fetch fallback), reusing ycode's `wire.Action`
  contract verbatim so agents that already know `browser_*` transfer directly.
- **`bashy browser login <url>`** — the killer flow (Phase 1's reason to exist):
  1. probe-attach to the user's Chrome (or solo-launch if none, or instruct the
     user to start Chrome with `--remote-debugging-port=9222`);
  2. open `<url>` in a tab;
  3. poll for completion — a redirect to a configured success/callback URL, a
     token rendered in the page, or a cookie set on the domain;
  4. extract and print the token/cookie (`--json`), or, if it can't auto-detect,
     surface "approve in the open tab, then press enter" to the human.
  This is what turns "agy needs a browser sign-in" from a dead end into
  `bashy browser login <agy-oauth-url>`.
- `bashy browser status` — which mode is live, is a probe Chrome reachable.

## Why SEQUENTIAL, not a fleet

This is **one cohesive subsystem sharing heavy code** (the `Client` interface,
the three mode services, the verb registry, `go.mod`). Per the conductor
scheduling rule, parallel agents on shared source *collide irreconcilably at
merge*. So Phase 1 runs as a **single sequential story** — one strong tool
(codex) grinding + resuming, or the conductor doing it directly — decomposed
into commit-sized steps below, each independently gate-able.

## Sequenced steps (each a commit)

1. **Vendor the protocol + client.** Copy `pkg/browser/wire` + a `Client`
   interface into `coreutils/pkg/browser`; add `chromedp`/`cdproto` to go.mod +
   `THIRD_PARTY_LICENSES`. Gate: `CGO_ENABLED=0 go build ./pkg/browser/...`.
2. **Port the probe engine.** CDP-attach to `--remote-debugging-port`; implement
   `Execute(action)` for the core verbs (navigate/eval/extract/cookies_get/
   screenshot/click/type/wait_for_selector). Gate: unit tests with a stub CDP
   target; a live test behind `t.Skip` when no Chrome.
3. **`bashy browser` verb.** Register the shell builtin in coreutils + wire the
   21 actions; `--json`; probe>solo>http-fetch fallback. Gate: `bashy browser
   status` + `bashy browser fetch <url>` (HTTP fallback, no browser needed).
4. **`bashy browser login <url>`.** The OAuth flow on top of step 2/3. Gate: a
   unit test of the completion-detector (redirect/token/cookie matchers) against
   synthetic navigation events; a `--dry-run` that prints what it would poll.
5. **solo mode.** `chromedp`-launch a discovered system Chrome for headless runs
   (the current builtin's chrome-path list). Gate: skip-if-no-chrome live test.
6. **Wire into the conductor.** Phase 0's `weave fleet --auth` `needs-login`
   result gets a one-line hint: "clear with `bashy browser login <url>`." (Full
   auto-routing is Phase 3's gate-broker; this is just the pointer.)

## Acceptance / gate (the whole phase)

`CGO_ENABLED=0 go build ./...` cross-compiles clean (pure-Go); `go test
./pkg/browser/... ./cmds/... ` green; `bashy browser fetch`/`status` work with no
browser present; `bashy browser navigate`+`extract` work against a probe Chrome;
`make test-bash` stays 86/86; `THIRD_PARTY_LICENSES` updated. Deferred to later:
the `live` extension mode, a downloaded headless-shell, `bashy foreman` (Phase 2).

## Execution options (conductor's call)

- **A — one sequential agent (codex), supervised:** file the single story with
  the steps above as the body + the gate; `weave start … -- codex exec …`;
  steer + resume between steps. Matches the shared-code=sequential rule.
- **B — conductor does it directly:** the port is mechanical (copy + adapt +
  wire); I can grind the six steps with tight gates. Fastest for a cohesive port
  where merge-collision risk is the main reason to avoid a fleet anyway.

Recommendation: **B for steps 1–3** (the mechanical vendor+wire, where I keep the
`go.mod`/license/registry coherent), then **A (codex) for steps 4–5** (the login
detector + solo mode are self-contained feature work a single agent does well),
with me gating each commit. Kick off on your word.
