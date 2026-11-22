---
id: 01a1fb78-52f1-7c11-98f1-1a8453994ea6
seq: 30
form: page
type: runbook
title: Enable an app for consumer shaping
description: 'Make any app (static public/ or its own loopback server) shapeable by its consumers: shaped.json with server + genie.examples, bashy app add --type shaped, CLI following the console runtime address, Genie injection check, member wish to undo, and the failures found doing it — WHEN an app owner wants members to reshape their own copy with bashy shape / the Genie button (keywords: shaping-enable, consumer shaping, shaped app, shaped.json, genie, app add --type shaped, runtime address, member wish)'
tags:
    - shaping
    - apps
    - genie
    - consumer
status: validated
evidence: Sprint 682 C1–C3 shipped example-app checks plus the 2026-11-21 native WSL Hello World walkthrough on Bashy 5c8be965. Production Genie candidate 02ad98b45385e8bc, trusted verifier, paired Chrome preview/approval/isolation/undo. Private evidence manifest SHA-256 f945a5851b962bd3de6340542e338e952edeadc5d9fd8075227161a62f29445b.
source:
    tool: qiangli
created: "2026-11-21T00:18:49Z"
updated: "2026-11-21T08:42:50Z"
---

## Who this is for

You own an app — a static page set under `public/`, or a small program that
serves itself on a loopback address — and you want its **consumers**
(members, not you the owner) to be able to reshape their own copy of it: a
member states a wish, an agent prepares an isolated candidate, a trusted
verifier checks it, the member previews and approves, and undo always
brings the baseline back. Other members never see the change. Owner-side
offer/adopt/release flows are a separate topic and are not covered here.

A **✓** marks a step exercised while enabling the Sprint 682 examples or
walking through the live Hello World lifecycle. The walkthrough used the
production Genie delegate, the trusted verifier, and two independently
paired Chrome sessions on the WSL gate.

## 1. Declare the app shapeable: `shaped.json` ✓

Put a `shaped.json` at the app's source root. For an app with its own
server, name the entry point **relative to the source/module root** and
take the console-assigned address and data directory from the manifest
placeholders — never a fixed port:

```json
{
  "server": {"main": "server", "args": ["serve", "-addr", "$ADDR", "-data", "$DATA"]},
  "genie": {
    "examples": [
      "Keep the action bar visible while I scroll.",
      "Show the overdue items first."
    ]
  }
}
```

- The build root and `server.main` must agree: the console builds from the
  source root, so a `go.mod` (or equivalent) buried in a subdirectory
  breaks the candidate build. Move the module root to the app root.
- `genie.examples` are the starter wishes your members see. Write two or
  three concrete, checkable wishes in the member's voice, not features in
  yours.
- Ship a `contract.bsh` and `checks/` that verify the app from a clean
  candidate clone. Verification must build from source; never rely on a
  prebuilt binary checked into the repo. ✓

For a static app there is no `server` block: the shaped files live under
`public/` and the same lifecycle applies (the hello-world example under
`examples/shaping/` is the reference). ✓

## 2. Register through the public shaped path ✓

```sh
bashy app add APP --type shaped --source /path/to/app
bashy app show APP --json   # confirm the record: type shaped, managed source
bashy app list              # confirm the registration is visible
```

Do not register the app as a fixed-port proxy instead: a proxy registration
misses the shaped request path, so the Genie controls are never injected
into the page and member shaping silently does not exist. ✓

## 3. Point the app's terminal CLI at the console's instance ✓

If the app ships an agent CLI, that CLI must talk to the **same** server
instance (and data directory) the console launched for the requesting
principal — not to a port it remembers, and never to a second server it
starts itself. A separately launched terminal does not inherit the
console subprocess environment, so resolve the live instance explicitly:

```sh
bashy app show APP --runtime --json
# -> the live loopback addr, the principal's data dir, and the console
#    app URL, read from the runtime record the console publishes after
#    the app becomes ready (open the app once first).
```

Resolution order that works: an explicit app-specific handoff
(environment/flag set by a trusted launcher) wins; otherwise the
`--runtime` record; otherwise **one actionable error** telling the user to
open the app in the console. Never fall back to a development port. Links
the CLI prints for a human must use the returned console app URL, so the
page opens through the console mount — with Genie — not the raw listener.
Keep the console data directory and any standalone-mode data directory
distinct; copying one mode's path into the other mixes two installations'
data. ✓

## 4. Check the Genie injection through the console mount ✓

Open the app **through its console mount** and confirm the Genie controls
are present on the page. Testing the app's own listener directly can pass
every functional check while the shaping entry point is missing, because
injection happens on the console path, not inside the app. ✓

## 5. The member lifecycle ✓

```sh
bashy shape status  APP --user PRINCIPAL --json
bashy shape wish    APP 'Keep the action bar visible while I scroll.' \
                    --user PRINCIPAL --agent TOOL:MODEL --json
bashy shape show    APP CANDIDATE --user PRINCIPAL --json
bashy shape diff    APP CANDIDATE --user PRINCIPAL
bashy shape verify  APP CANDIDATE --user PRINCIPAL
bashy shape preview APP CANDIDATE --user PRINCIPAL --console CONSOLE_ORIGIN
bashy shape approve APP CANDIDATE --user PRINCIPAL
bashy shape undo    APP --user PRINCIPAL
```

Rules of the road: a wish produces an isolated candidate; approve only
after `verify` passes (approval re-verifies); `preview` prints a console
URL to open in a browser paired as the same principal (the printed URL is
not evidence the page rendered); a second principal stays on the baseline
throughout, and `undo` restores the previous version. Pass the console's
origin explicitly when it uses a non-default port. In the Hello World
walkthrough, the member saw the candidate greeting in preview while their
active page still showed the baseline. Approval changed only that member's
active page, and undo restored it. The example-app matrix uses the same
lifecycle for its app-specific checks. ✓

## Common failures found while enabling the examples ✓

- **No Genie on the page.** Almost always the registration: proxy instead
  of `--type shaped`, or the check was done against the raw listener.
  Re-register via the public path (§2) and re-check through the mount (§4).
- **Candidate build fails immediately.** `server.main`/module root are not
  at the source root the console builds from (§1).
- **CLI writes "succeed" but the page shows nothing.** The CLI reached a
  different instance or data directory than the console's (fixed port, own
  server, standalone data dir). Resolve via §3; make the wrong-mode case
  an error, never a silent fallback.
- **`--runtime` record missing or stale.** The app is not running. Open it
  once in the console, or supply a trusted explicit handoff; the reader
  already says so — surface that error, do not invent an address.
- **Verify fails before any check runs.** Apps whose checks need
  network-installed bundles (cloned/built web assets) cannot verify until
  that install has run. Say so and skip loudly; never report such an app
  as covered.
- **A flag that does not apply to the chosen mode is silently ignored.**
  Treat mode-mismatched options as errors. Silent drops are how a CLI ends
  up serving the wrong instance while reporting success.

## Verify you are done

- ✓ `bashy app show APP --json` shows a shaped registration; the console
  page shows Genie; the app CLI (if any) reaches the console instance.
- ✓ One member wish went wish → verify → preview → approve → undo with a
  second member unaffected (the C4 e2e script automates exactly this).
- ✓ This page itself: `bashy kb search "enable an app for shaping"` finds it.
