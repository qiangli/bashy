# Phase 3 — the gate broker (lined up)

Unify Phases 0–2 into the **gate broker**: when a weave worker blocks on an
interactive gate mid-run, classify the gate *by type* off its PTY stream and
**auto-route** it — trust → keystroke (`weave say`), browser-OAuth →
`bashy browser login <url>` (Phase 1), device-code/api-key/unresolvable →
escalate to the human through the foreman session (Phase 2). Layer 2 of
`docs/conductor-steering-surface.md`. Turns today's reactive one-off `say` into a
systematic broker. **Not a new verb** — it lives inside `weave`/`foreman`.

## What exists (surveyed) — reuse, don't reinvent

- **Phase 0 classifier** (`pkg/weave/weave_tools.go`): `authGateSignatures`
  (case-insensitive markers of an auth/trust gate) + `classifyReadyOutput` (pure,
  table-tested) + `ReadyOK/NeedsAuth/Stale`. This is the *pre-dispatch* readiness
  check. Phase 3 adds the *live, typed* sibling.
- **weave live stream + injection**: `weaveReadThrottleLogTail(logPath)` reads a
  running worker's PTY tail (`logs/issue-N.log`); the per-issue control socket
  `weaveCtlSockPath`/`CtlSock` + `newWeaveSayCmd` inject keystrokes into the live
  PTY. `runWeaveWait` (`weave_impl.go:4453`) is the monitor loop that hosts the
  broker.
- **Tool profiles** (`ToolProfile`): `TrustClear` (e.g. `"say:1"`), `SupportsSay`,
  `AuthHint`. Phase 3 reads these to pick the keystroke route.
- **Phase 1** `bashy browser login <url>` (`cmds/browser/browser.go:133`,
  `--success-url`/`--dry-run`, completion detector) — the browser-OAuth route.
- **Phase 2** `bashy foreman tell/status` + weave blocker comments
  (`weave_comments.go`) — the human-escalation channel.

## Design — a classifier + a router, wired into the monitor loop

Two pure, testable cores + one wiring point (mirrors Phase 0's split):

1. **Typed gate classifier** — `classifyGate(tail string) GateVerdict` where
   `GateVerdict{Kind, URL, Signature}` and `Kind ∈ {none, trust, browser_oauth,
   device_code, api_key, human}`. Extends `authGateSignatures` with:
   - **trust** — "do you trust", "trust the contents", a numbered
     yes/continue prompt → the existing `TrustClear` keystroke.
   - **browser_oauth** — an auth/login marker *plus an extractable* `https://…`
     URL (regex for `oauth`/`authorize`/`login`/callback URLs) → browser login.
   - **device_code** — "enter the code", "device", a short user-code + a
     verification URL → surface code+URL to the human (browser can't type it).
   - **api_key** — "no api key", "api key not set" → escalate (human sets a key).
   - **human** — an auth/login gate with no URL and no known keystroke → escalate.
   - **none** — no gate (working, or a normal question the conductor answers).
   Pure + table-tested, like `classifyReadyOutput`. This is the heart of the phase.

2. **Gate router** — `routeGate(verdict, w routeDeps) (action, error)` over an
   injected `routeDeps` seam (say-fn, browserLogin-fn, escalate-fn) so it is
   unit-tested with stubs (no live tool/PTY/Chrome):
   - `trust` → call the say-fn with the profile's `TrustClear` payload.
   - `browser_oauth` → call the browserLogin-fn with `verdict.URL`; on success,
     optionally say a continue key; on failure, fall through to escalate.
   - `device_code`/`api_key`/`human` → call the escalate-fn (foreman `tell` +
     a weave `blocker` comment) with an actionable message (code/URL/hint).
   Every route is **recorded** (what gate, what action, outcome) — measured
   numbers are the only truth; a broker action is a lead until the worker
   un-blocks.

3. **Wire into `weave wait --broker`** (opt-in flag; also honored by a foreman
   worker-supervise): the monitor loop tails the log; when a gate signature
   appears **and the worker is idle** (no new output for a debounce window — a
   *live* block, not stale scrollback, per the conductor rule), classify → route
   **once per distinct gate** (dedupe by signature+URL so it doesn't re-fire in a
   loop), log the action, and keep waiting. A cap (N auto-routes per worker) then
   escalates to the human — no silent infinite clearing.

## Sequenced steps (each a commit)

1. **`pkg/weave/gate_broker.go` — classifier.** `GateKind`, `GateVerdict`,
   `classifyGate` (+ URL extractor). Table test covering every kind incl. real
   agy/codex/claude/gh/gcloud gate strings + false-positives (a normal question,
   working output). No wiring yet.
2. **Router + deps seam.** `routeGate` over an injected `routeDeps`; the three
   route impls behind function fields. Unit-test each route with stubs + the
   cap/dedupe logic.
3. **Wire the live monitor.** `weave wait --broker`: tail + idle-debounce +
   classify + route once-per-gate + record. Gate: an integration test driving a
   synthetic log through the loop with stubbed deps asserts the right route fires
   once and the cap escalates.
4. **Real route bindings.** Bind say-fn → the ctl-socket injection, browserLogin
   → `bashy browser login` (invoke the registered tool), escalate → foreman
   `tell` + weave blocker comment. Gate: `weave wait --broker` on a real idle
   worker clears a trust prompt end-to-end (skip-if-no-fleet for the live arm).
5. **Conductor skill update.** Replace the reactive-`say` guidance in
   `bashy/skills/conductor/SKILL.md` with the broker: it classifies + routes
   automatically; the conductor only handles what the broker escalates. (Mine.)

## Acceptance / gate

`CGO_ENABLED=0 go build ./pkg/weave/... ./cmds/... ./tool/...`; `go test
./pkg/weave/...` green (classifier + router + monitor tests, all stubbed — no
live tool/Chrome/PTY needed); `weave wait --broker` clears a trust prompt on a
real idle worker (live arm, skip-if-no-fleet); the cap escalates instead of
looping; `make test-bash` stays 86/86; the conductor skill carries the broker.
Deferred: the browser *extension* transport, multi-host broker, a learned
classifier.

## Execution (conductor's call)

Steps 1–4 are **one cohesive subsystem sharing weave's monitor internals** →
sequential (codex), each commit gated; step 5 (skill prose) I do in parallel.
Same split that worked for Phases 1–2. Dogfood the Phase-0 `weave fleet --auth`
pre-flight + the explicit-headless launch discipline; I gate each commit
independently (classifier false-positive table is the sharp edge) and converge.
