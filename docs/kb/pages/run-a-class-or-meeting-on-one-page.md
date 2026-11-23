---
id: 01a208e6-f84e-7b13-a339-b30aca956333
seq: 31
form: page
type: runbook
title: Run a class or meeting on one page
description: 'Run a class, meeting, workshop or retrospective on one live page: start from page templates, open both doors (outpost + LAN) with a join code, change the page in seconds with data verbs or in minutes by shaping a new block kind, then export the session record. Use when facilitating a group with the page desk app.'
tags:
    - page
    - meeting
    - class
    - facilitation
    - runbook
status: candidate
evidence: examples/desk/page TestTemplatesLoad; command signatures checked against examples/desk/page/cli.go and bashy shape wish --help
source:
    tool: claude-opus5.5
    episode: ep-4bfa1f5918573cd0
    parent_episode: ep-da7202d5258836a1
created: "2026-11-23T14:54:44Z"
---

Use this when a teacher, facilitator or meeting lead wants one live page for
the whole group: agenda, presence, speaking queue, timers, polls, forms and
boards in one place, reachable from the room and from outside, ending in a
record you can commit. Design of record: the umbrella's
`docs/page-live-group-page.md`; tool reference: `examples/desk/page/README.md`.

## 1. Start from a template

`examples/desk/page/templates/` ships `agenda.md`, `meeting.md` and `retro.md`:
phases with timeboxes, a `here` roster, a `hands` queue and one
`timer:<phase>` block per phase. They parse and load as shipped:

```bash
page add -f templates/meeting.md --title "Weekly sync" --audience lan --code blue-fox
```

`--audience host` (default) and `satellite` serve only this host and
signed-in outpost visitors; `lan` also admits peers on the local network.

## 2. Open both doors on one fixed port

```bash
page publish-outpost --port 8090   # satellite door: signed-in visitors
page serve --lan --port 8090       # LAN door: same listener
page open PAGE                     # join links, QR codes, the bare code
```

Publish before serving so both doors reach the same listener. Participants
type the short join code at the root or scan the QR code. A satellite
visitor joins under the app-specific pseudonym of their sign-in; an
anonymous LAN visitor is bound as `handle@address` and reclaims the same
handle from the same address after losing the cookie.

`page add` also returns the private organizer link: the organizer view shows
every face side by side or in tabs, **View as participant** previews under a
separate identity (`?view=participant`), and `?view=display` is the
projector page. Moderate with `page participant PAGE list`, `rename HANDLE
NAME`, `merge HANDLE INTO`, `kick HANDLE`; take the floor with
`page hands next|clear|list PAGE`.

## 3. Two speeds of change

**Seconds — data verbs, no build.** The facilitator, or their agent,
changes the running page with verbs:

```bash
page block add poll PAGE --options "A,B,Defer" --title "Which way?"
page block close PAGE poll-1 && page block show-results PAGE poll-1
page block add form PAGE --def signup.json --title "Sign up"
page block add board PAGE --session ideas --title "Ideas"
page set PAGE -f agenda.md          # add note:, book:, embed: or timer: blocks
page hands next PAGE
```

Everyone's view updates over the live stream. Timers start, stop and reset
from the organizer face.

**Minutes — shape a new block kind or layout.** When no verb or block kind
does what the group needs (a vote as a bar chart, results hidden until the
vote closes, one row per participant for a sign-off, a block listing open
pull requests), wish for it through shaping:

```bash
bashy shape wish page 'Show a poll as a bar chart on the projector' --user PRINCIPAL --agent TOOL:MODEL
bashy shape verify page CANDIDATE --user PRINCIPAL
bashy shape preview page CANDIDATE --user PRINCIPAL
bashy shape approve page CANDIDATE --user PRINCIPAL
```

or use Genie in the app console. A new kind is one registry entry and one
renderer; verify and preview before approving. Do not shape mid-session what
a data verb already does.

## 4. Close and export the record

```bash
page close PAGE
page export PAGE --to md -o records/2026-session.md   # or --to html | pdf
```

The export inlines every block's final state (tallies with handles,
per-handle answers unless anonymous, timer state, roster) and lists
participants as `handle (auth)` only. It never carries addresses,
fingerprints, cookies, respondent tokens or facilitator keys, so the
Markdown is safe to commit next to the code it discussed.

## Guardrails

- One fixed `--port` for both doors; a second listener splits the group.
- Keep the faces import (`--faces faces.json`) and the organizer link
  private: they carry facilitator keys.
- The default outpost Unix tunnel omits the client address; anonymous
  joins through it need that address, signed-in participants join by
  principal.
