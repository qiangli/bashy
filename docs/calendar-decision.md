# Shared calendar: reuse-or-build decision (Sprint 614, S8)

**Status:** decided 2026-11-10 (discovery spike, docs only; no product code).
**Decision:** build a tiny `examples/calendar` Desk tool from existing Desk
primitives. No third-party calendar product is adopted. The calendar is
self-hosted and the source of truth; cloud calendars connect only through
explicit import/export (ICS baseline, Google through `examples/google`).

## 1. What already exists (inventory)

| Surface | What it is today | Calendar fit |
|---|---|---|
| `bashy schedule` | cron/interval/at **command automation** (`add/list/run/tick/daemon/enable/disable`, `--json`, effects exec+persist) | Fires commands; no event entity, no agenda, no audience. Stays separate. It is the only **reminder** route, and only when a user wires it explicitly. |
| `examples/google` (`google.bsh`, gws-backed) | `calendar-list/get/create/update/delete/freebusy` + `agenda` against a Google account | Provider-coupled, needs an external account and BYOP credentials, Google holds the data. An **interop route**, not the shared calendar. |
| `bashy meet` | agent meeting rooms, minutes, polls | Not dated scheduling. |
| `bashy tz` | IANA zone `list/now/convert/info`, `--json` | Timezone primitive to reuse in the wrapper. |
| `examples/tutor` + `bashy sprint` | a **Session** is a sprint card with timeboxed steps (`bashy sprint show N --json`) | The entity a calendar event references; no date view of its own. |
| `examples/desk` kit + notepad/viewer/form/chart | stdlib Go loopback server + browser page + one `.bsh` skill-script; stable id + URL; owner key, single-use invite → members, 404 for non-members; atomic JSON items under `$BASHY_HOME/desk/<tool>/`; `register.sh` does `commands add` + `skill add` + `app add --type shaped`; shared contract test; `viewer add --session S` session books | The app skeleton. `kit/internal/fixture` (`memo`) is a ~900-line complete tool proving the contract. |
| `bashy person` / `whois` / `member`, launcher auth tier, `app pair` | human principals and device/operator access | Identity to record on events; enforcement reuses the Desk membership mechanism. |
| `bashy kb` (repo ring `docs/kb/`, `kb:slug` refs), `examples/klatch` (`conversations-*`) | materials and chat entities | Reference targets. |
| Memos | **not present** in this repo (no usememos reference anywhere). Upstream calendar view groups notes by create/update timestamp, in React components coupled to Memos contexts | Not an event calendar; extraction is S7's Notes concern, not a scheduling route. |
| ICS / RFC 5545 | nothing in bashy, coreutils or yoke parses or writes VEVENT | Missing primitive (small). |

Conclusion: no shared human calendar exists anywhere in the suite today.

## 2. Shortlist evaluated

Pins are the default-branch heads on 2026-11-10. Builds were not run; the
verdicts rest on each repo's `go.mod`, tree and README.

| Route | Pin / license | Build + runtime dependencies | Web for humans | Agent CLI parity | Audience | Effort (integrate + maintain) |
|---|---|---|---|---|---|---|
| mgnsk/calendar | `7b16907` · MIT | Go 1.26, 18 direct deps (bun ORM + SQL migrations, scs sessions, gomponents/htmx, golang-ical, gofeed), Tailwind via npm, Docker + Caddy in Makefile and compose | Yes (events, tags, feeds, own users/invites/login) | **None**: HTML form handlers only, no JSON API, no CLI | Its own user table, not bashy identity | High: wrap every HTML form, map two identities, carry a DB + npm toolchain; 1 star, single maintainer |
| DouglasdeMoura/chroncal | `5a999ca` · MIT | Go 1.26, 26 direct deps (bubbletea v2 TUI, modernc sqlite, go-webdav, rrule, keyring, cobra/viper) | **No** web UI; sharing needs a CalDAV **server** (deferred by the story) | Good CLI, TUI | CalDAV server's | High: still needs a web UI and a server; wrong shape |
| revelaction/ical-git | `7e5f2e8` (2024-10-28) · MIT | Go 1.23, go-git, golang-ical, rrule, beeep, telegram bot | No UI | No CRUD; it is a **reminder daemon** over ICS files in a git repo | None | Overlaps `bashy schedule`; nothing to reuse for create/list/show/edit |
| Memos calendar view | upstream `web/src/components/CalendarView` | React + Memos contexts, memo APIs, routing | Yes, but of memo timestamps | Memos API only | Memos users | Falsifies timestamps to encode start/end; extraction costs more than a page |
| **`examples/calendar` from Desk** | this repo; `desk-kit` stdlib + optionally arran4/golang-ical (Apache-2.0) | `go build` with `CGO_ENABLED=0`, same as notepad; no DB, no npm, no container | Yes: the Desk page (agenda list), same look and shell as notepad/viewer | By construction: every page action is an `/api/action` verb and a `.bsh` verb with `--json` | Desk owner/invite/members + `bashy person` ids | **Low**: copy the `memo` fixture, add fields and an agenda page, ~1 story |

Picking a product to get a web page and then writing a CLI over its HTML is
more work than the whole MVP; picking a CLI product still leaves the web page
to build. The Desk route needs the fewest new lines and no new identity,
storage or toolchain.

## 3. The chosen route: `examples/calendar`

**Shape.** `examples/calendar/{calendar.bsh, main.go, cli.go, register.sh,
shaped.json, contract.bsh, dag.md, templates/, formats/}`, module `calendar`
with `replace desk-kit => ../desk/kit`, accent/icon row added to `kit.Tools`.
`calendar.bsh` is the one Bash# skill-script (`~~~skill as guide` fence) and
the shaping entrypoint; it builds and execs the Go server exactly as
`notepad.bsh` does. Placement is example level (experimental); graduation to
optional/core follows the rules in the kb page on extending bashy, never
automatically.

**Entity.** One item = one event, id `^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`,
file `items/<id>.json`:

```
{ "id", "title", "start", "end", "tz", "location", "body" (Markdown),
  "refs": ["kb:<slug>", "sprint:<N>", "viewer:session-<S>", "klatch:<conversation-id>", "https://…"],
  "audience": ["<person-id>", …], "status": "pending|closed|cancelled",
  "source": {"provider": "ics|google|local", "uid": "<external UID>"},
  "owner", "members", "invites", "created", "updated" }
```

`start`/`end` are RFC 3339 **with** the explicit IANA `tz`; the server
rejects an event whose zone `time.LoadLocation` cannot load. Display converts
to the viewer's zone (`--tz` / `TZ`, default host zone); the wrapper reuses
`bashy tz convert` for ad-hoc conversions. Event time is never derived from
item create/update time.

**Minimum scope (MVP).**

- Verbs: `add`, `show`, `set`, `list` (the agenda: `--from/--to/--tz/--json`),
  `cancel` (status `cancelled`, kept), `open`, `share`, `close`, `serve`,
  `export`, `import`, `save`, `load`. All print the `bashy-calendar-v1` envelope
  with `--json`; `open` prints id/URL/status only.
- Page: one agenda (day/week list grouped by date in the viewer's zone) with an
  event detail panel showing refs as links through the kit renderer. A month
  grid is **not** in scope.
- Audience: kit owner key (`--key`), `share` mints a single-use invite,
  `join` makes the browser a member; non-members and revoked members get 404
  on state/action/export, as in notepad. `audience` names `bashy person` ids
  for display and filtering. Hosted under the launcher via `app add`, the
  launcher auth tier applies unchanged.
- Durable Git-managed definitions: the live store is the Desk data dir
  (atomic write). `calendar save [--dir DIR]` writes one `.ics` per event plus
  `events.json` into a user-chosen Git directory (default `docs/calendar/` of
  the current repo, the same repo-ring idea as `bashy kb`); `calendar load DIR`
  reads them back. Committing and pushing is the user's explicit `git` step;
  the tool never runs git.
- Import/export: ICS (RFC 5545 VEVENT subset: UID, DTSTART/DTEND with TZID,
  SUMMARY, DESCRIPTION, LOCATION, STATUS, URL for refs); `import` and `export`
  take `--dry-run` and `--json`, run the same audience checks, and record
  `source.{provider,uid}` so a re-import updates instead of duplicating.
  Google: `import --from google` composes `bashy google calendar-list` and
  `export --to google` composes `bashy google calendar-create/update` per
  event, recording the returned event id as `source.uid`. No background sync,
  no two-way sync, no CalDAV.
- Never launches agents: creating an event has no exec effect. A reminder is
  a separate, explicit `bashy schedule add --at "$(bashy calendar show ID --json | jq -r .start)" …`.

**CLI parity matrix (every web action has a verb).**

| Web action | CLI |
|---|---|
| create / edit / cancel event | `calendar add|set|cancel ID … --json` |
| agenda, change zone, filter by date | `calendar list --from --to --tz --json` |
| event detail, follow a ref | `calendar show ID --json` (refs are stable strings) |
| share link, join, revoke | `calendar share ID`, `join` (page), `set ID --revoke MEMBER` |
| export page / .ics | `calendar export ID|--all --to ics|json [-o FILE]` |
| admin: serve, data dir, register, retire | `calendar serve`, `CALENDAR_DATA`, `register.sh`, `bashy app retire calendar` |

**Shaping hooks (named).** `templates/agenda.html` (page layout),
`formats/event.schema.json` (fields and validation), `calendar.bsh` verbs
(composition, including which `examples/google` verbs the Google route calls),
`shaped.json` (server args). `bashy shape calendar "…"` edits these; Go is
shaped only for a missing primitive. No classroom, roster or provider flow is
hard-coded.

**Minimum dependencies.** `desk-kit` (stdlib). ICS: either a hand-written
VEVENT subset reader/writer (~150 lines, no recurrence) or
`github.com/arran4/golang-ical` (Apache-2.0, permissive); the follow-up
decides by test size. No DB, no npm, no cgo, no container.

## 4. Gaps and deferrals

Recurrence and per-occurrence exceptions (would add rrule-go, MIT), RSVP and
invitations, reminders inside the tool, CalDAV serving, external sync, month
grid, drag-and-drop, Memos UI extraction. Time zone display across DST edges
and all-day events need tests. Google export needs an onboarded `examples/google`
account; without it the verb fails with an actionable `CLI_NOT_FOUND`-style error.

## 5. Draft follow-up story (not filed)

> **Build `examples/calendar` (Desk tool) MVP.** Start from
> `examples/desk/kit/internal/fixture` and `examples/desk/notepad`. Deliver
> `calendar.bsh` + Go server with the entity, verbs, agenda page, audience,
> `save/load`, ICS import/export with `--dry-run/--json` and idempotent
> `source`, and Google import/export composing `examples/google`. Register
> through `register.sh` (`commands add`, `skill add`, `app add --type shaped`);
> add a `kit.Tools` row; pass the desk contract test, Go tests and a
> `browser-test.mjs` round-trip; add a `dag.md` `roundtrip` target. Document
> the shaping hooks (`templates/`, `formats/`, `calendar.bsh`) and the CLI
> parity matrix in the README. Acceptance: every page action has a verb with
> structured output and the same denied-access result; creating an event
> executes nothing; no recurrence, RSVP, reminders, CalDAV or sync. Placement
> stays `examples/`; not added to `bashy-defaults`.
