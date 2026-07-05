# Architecture: the conductor's steering surface — foreman + browser + gate-broker

## The real problem (agy is the symptom, not the disease)

A conductor drives a fleet of third-party agentic CLIs. Every one of them puts up
an **interactive gate** before it will work, and bashy today only knows how to
clear *one* kind:

| Gate | Example | Handled today? |
|---|---|---|
| Trust / welcome prompt | codex "do you trust this directory?" | ✅ reactively (`weave say <N> "1"` + `TrustClear` profile) |
| Terminal auth (API key, device code) | paste a key / confirm a code | ⚠️ ad-hoc keystroke injection only |
| **Browser OAuth sign-in** | **agy "you are currently not signed in"** | ❌ **not at all — this is why agy stalled 15 min** |
| Ongoing steering mid-run | "actually, do X first" | ⚠️ `weave say` / `weave attach`, no first-class session |

Two more facts make it worse:
- **`bashy weave fleet` checks PATH + `--version`, never auth.** A tool on PATH but
  unauthenticated (agy) reports **"available"**, so the conductor dispatches it
  straight into a stall. Availability ≠ readiness.
- **`bashy chat` is one-shot** ("resolve a role, build one instruction, run it
  unattended"). There is no persistent, steerable session a human *or* a
  conductor-agent can drive incrementally.

So the fix is not "make agy work" — it's **give the conductor a complete surface
for clearing any tool's interactive gate**. Three layers, all pure-Go,
cross-platform, self-contained; all pieces already exist somewhere (ycode) and
need to be brought together under bashy.

## Layer 1 — `bashy foreman`: the steerable session (the cockpit)

Elevate one-shot `bashy chat` into a **persistent, resumable, steerable** session.
Name it **`foreman`** (discoverable; differentiates from a chatbot; aligns with
ycode's existing Boss→Foreman→Worker model). `chat` survives as `foreman --once`.

Two co-equal drive modes over the *same* session:
- **Human mode** — a readline REPL (reuse bashy's `internal/cli/interactive.go` +
  the `sh/interactive` stack). A person steers live.
- **Agent mode** — a control channel (generalize weave's per-issue `ctl/*.sock` +
  `say`). A conductor-agent steers programmatically: `bashy foreman tell "<msg>"`,
  `status`, `pause`, `resume`, `stop` (mirrors `ycode foreman …`).

The session **embeds a `bashy dag`** (the literate-markdown task graph bashy
already runs): `bashy foreman run plan.md` = "execute this dag as a steerable
session, holding context + backlog, delegating units to `weave` workers, pausing
for me when you're stuck." **Merge the ycode `ycode-foreman` skill into
`bashy/skills/conductor`** so there is exactly one playbook and one home (the
conductor skill already moved to bashy).

Net: the conductor stops being "me typing ad-hoc `weave` commands" and becomes a
**first-class, resumable object** with a control surface — the thing that then
*coordinates* Layers 2 and 3.

## Layer 2 — the gate broker: auth-readiness + gate clearing (the direct agy fix)

Extend the fleet/launch tool profiles (today: `HeadlessArgs` + `TrustClear`) with
two fields:
- **`AuthCheck`** — a cheap, cached "am I logged in?" probe per tool (e.g. `agy
  whoami`, `codex login status`, an env/keychain check).
- **`AuthFlow`** — how to authenticate (browser OAuth → Layer 3; terminal device
  code → keystroke/paste; API key → prompt the human).

Then:
- **`bashy weave fleet --auth`** (and a `foreman doctor` pre-flight) reports each
  tool as `ready` / `needs-login` / `cooling-down` — not just `available`. The
  conductor **fails fast on `needs-login`** (surface "agy needs sign-in") instead
  of dispatching into a 15-minute stall. *This one change alone eliminates the
  agy failure mode*, before any big feature ships.
- A **gate classifier** on the PTY stream routes an in-flight gate by type: trust
  → keystroke (existing); browser-OAuth → Layer 3; unresolvable → escalate to the
  human through the foreman session. Turns today's reactive one-off `say` into a
  systematic broker.

## Layer 3 — `bashy browser`: the browser surface (the auth unblocker)

Port ycode's browser subsystem into coreutils/bashy — `pkg/browser` (transport-
shaped dispatcher), `mcpservers/browsermcp`, the shell builtin, and the ~16
`browser_*` verbs (navigate/click/type/eval/extract/cookies_get/storage_get/
screenshot/keyboard/tabs/…). Keep **both transports** so it's dependency-free:
- **CDP → the user's real Chrome** (`--remote-debugging-port`) — reuses their
  existing logins; ideal for OAuth. A small pure-Go CDP/websocket client (or
  ycode's) — nothing vendored, no headless binary required.
- **The live browser extension** (the "ycode browser ext") — moved in so it runs
  under `bashy browser` with zero ycode dependency, giving access to the user's
  live session/cookies where CDP can't reach.

The unblocking primitive: **`bashy browser login <oauth-url>`** — open the URL in
the user's session, detect completion (redirect / token), extract the token or
cookie; if there's no session, surface "open this and approve" to the human via
the foreman. This is the **known, uniform entry** for every browser-gated
third-party tool — the pain the user named ("steering a third-party tool has been
a pain to any agent acting as conductor").

## How the three layers fix agy (and the general case)

1. **Foreman pre-flight** runs the **gate broker's `AuthCheck`** → agy reports
   `needs-login` → conductor does **not** dispatch (no stall); it surfaces the gap.
2. **`bashy browser login <agy-oauth-url>`** completes the OAuth — automatically
   if the user's Chrome already has the Google session, else it hands the URL to
   the human through the foreman.
3. Authed, the **foreman** dispatches agy as a normal weave worker.

Same path unblocks any tool that gates on browser login, terminal auth, or a
trust prompt — one surface, not per-tool heroics.

## Recommended build order (each independently shippable)

- **Phase 0 — auth-readiness (days, huge ROI, do first):** add `AuthCheck` to the
  tool profiles + `bashy weave fleet --auth` + conductor fail-fast. Kills the
  agy-stall class immediately, no new subsystem. *Also: keep agy off the default
  headless fleet until this lands (per the RETRO memory).*
- **Phase 1 — `bashy browser`:** port ycode's browser (CDP transport first,
  extension second) + `browser login`. Pure-Go, cross-platform, self-contained.
- **Phase 2 — `bashy foreman`:** elevate `chat` → steerable session (human REPL +
  agent control channel), embed `bashy dag`, merge the ycode foreman skill into
  `bashy/skills/conductor`.
- **Phase 3 — the gate broker:** unify — foreman detects a worker's gate on the
  PTY stream and routes it to browser / keystroke / human automatically.

## Naming & surface discipline

- `bashy foreman` = the session (primary); `bashy chat` = `foreman --once` alias.
- `bashy browser` = the browser surface (new top-level verb).
- The gate broker is **not** a new verb — it lives inside `weave`/`foreman`
  (profiles + pre-flight), keeping the surface minimal.
- One playbook: `bashy/skills/conductor` absorbs `ycode-foreman`.

## Why this is the right shape (veteran-conductor view)

The recurring conductor tax is not "which model is smartest" — it's **the minutes
lost at every interactive gate and the silent stalls they cause**. Fixing model
selection is marginal; fixing the *gate surface* is structural. Phase 0 pays for
itself on the next fleet run; Phases 1–3 turn "steering third-party tools is a
pain" into "there is one known entry, and the conductor clears the gate or asks
me once." That is the highest-leverage investment in the conductor stack right
now — higher than any per-tool tuning.
