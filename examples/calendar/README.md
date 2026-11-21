# Desk Calendar (`examples/calendar`)

Self-hosted shared calendar MVP built on `desk-kit` (`docs/calendar-decision.md` §3 & §5).
Humans browse and edit events on a day/week loopback agenda page; agents drive the
exact same server through `bashy calendar` verbs with `bashy-calendar-v1` JSON envelopes.

## Design Boundaries

- **Self-hosted source of truth**: atomic JSON items under `$BASHY_HOME/desk/calendar/items/<id>.json` (`0700` dir, `kit.AtomicWrite`).
- **Explicit IANA timezones**: every event stores `start` (RFC 3339), `end` (RFC 3339, `end >= start`), and `tz` validated with `time.LoadLocation` (embedded pure-Go `time/tzdata`). Event times are never derived from `created`/`updated`.
- **Zero execution side effects**: creating, updating, or importing an event never runs commands or launches agents. Wire reminders explicitly via `bashy schedule` when needed:
  ```bash
  bashy schedule add --at "$(bashy calendar show evt-1 --json | jq -r .start)" -- bashy inbox human send "Sprint review starting"
  ```
- **Deferred / out of scope**: recurrence (`RRULE`), RSVP, built-in reminder daemons, CalDAV serving, background two-way sync, and month-grid views.

## Quick Start & Consumer Shaping Registration

```bash
# Register command, skill, and shaped console app (--type shaped):
./examples/calendar/register.sh

# Confirm shaped app registration:
bashy app show calendar --json
```

When opened in the console, `bashy calendar` client verbs automatically resolve the
live console-managed instance via `bashy app show calendar --runtime --json`
(`kit.DiscoverAppRuntime("calendar")`) so printed links open through the console mount
with Genie controls. For standalone testing without the console, run `calendar serve`
and pass `-addr` / `-data` (or `CALENDAR_ADDR` and `CALENDAR_DATA`).

## Event Entity (`items/<id>.json`)

```json
{
  "id": "sprint-616-demo",
  "title": "Sprint 616 Demo",
  "start": "2026-11-21T10:00:00-08:00",
  "end": "2026-11-21T10:45:00-08:00",
  "tz": "America/Los_Angeles",
  "location": "Room 4B",
  "body": "# Agenda\n\n- Demo Desk calendar\n- Review shaping hooks",
  "refs": ["sprint:616", "kb:enable-an-app-for-consumer-shaping", "https://example.com/notes"],
  "audience": ["alice", "bob"],
  "status": "pending",
  "source": {"provider": "local", "uid": "sprint-616-demo"},
  "owner": "<48-hex capability>",
  "members": {"<participant-token>": true},
  "invites": {},
  "created": "2026-11-21T09:00:00Z",
  "updated": "2026-11-21T09:00:00Z"
}
```

## Web-to-CLI Parity Matrix

Every browser action maps 1-to-1 to a CLI verb returning the `bashy-calendar-v1` envelope under `--json`:

| Web Action | CLI Verb | Notes |
|---|---|---|
| Create event | `calendar add [ID] --title T --start S --end E --tz TZ [--body MD] [--ref R]... [--audience P]... [--dry-run] [--json]` | Returns `id`, `url`, `status`, `ownerKey` (never `body`). |
| View agenda / switch TZ / filter | `calendar list [--from T] [--to T] [--tz TZ] [--audience P] [--status S] [--json]` | Groups/sorts chronologically and projects `display_start`, `display_end`, `date` in viewer `tz` (never `body`). |
| View event detail & refs | `calendar show ID [--tz TZ] [--json]` | The one read that returns Markdown `body` and `refs`. |
| Re-surface link | `calendar open ID [--json]` | Prints `id`, `url`, `status` only. |
| Edit event / revoke member | `calendar set ID [--title ...] [--start ...] [--revoke MEMBER] [--dry-run] [--json]` | Refused with 409 after `close`. |
| Cancel event | `calendar cancel ID [--dry-run] [--json]` | Marks `status: "cancelled"`, retains item. |
| Close event | `calendar close ID [--dry-run] [--json]` | Marks `status: "closed"`. |
| Share single-use invite / join | `calendar share ID [--json]`, `calendar join ID INVITE` | Non-members and revoked members get **HTTP 404** on protected state/action/export. |
| Export ICS / JSON / Google | `calendar export [ID\|--all] --to ics\|json\|google [-o FILE\|-] [--calendar CAL_ID] [--dry-run] [--json]` | Google export composes `BASHY_GOOGLE_CMD` (`calendar-create` / `calendar-update`). |
| Import ICS / Google | `calendar import [-f FILE\|-] [--from ics\|google] [--calendar CAL_ID] [--dry-run] [--json]` | Deduplicates/updates in-place via `source.{provider,uid}`. |
| Git-dir save / load | `calendar save [--dir DIR] [--dry-run] [--json]`, `calendar load [DIR] [--dry-run] [--json]` | Writes `events.json` + `<id>.ics` (default `docs/calendar/`); never invokes `git`. |
| Standalone server | `calendar serve [-addr 127.0.0.1:18099] [-data DIR]` | Loopback-only HTTP server. |

Common options supported across verbs: `--json`, `--fields`, `--no-input`, `--explain`, `--dry-run`, `--help=mcp`.

## Shaping Hooks

Consumers can reshape their own isolated instance via `bashy shape calendar "…"` or the console Genie button:

1. `templates/agenda.html` — Agenda page layout, filters, and client behavior (loaded live from `<data>/templates/agenda.html` or `<source>/templates/agenda.html` with `//go:embed` fallback; styled strictly with `kit.css` `--desk-*` tokens).
2. `formats/event.schema.json` — Event field constraints, max lengths, and enums validated before persistence (loaded from disk with `//go:embed` fallback).
3. `calendar.bsh` — Bash# skill-script entrypoint and embedded skill documentation.
4. `shaped.json` — Console server manifest and starter consumer wishes (`genie.examples`).
5. `contract.bsh` & `checks/go.bsh` — Verifier effect cap and clean-clone Go build + test gate.
