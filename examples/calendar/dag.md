---
name: calendar
description: Desk shared calendar tool (explicit IANA timezones, agenda page, ICS + Google interop, Git save/load)
type: dag
default: serve
---

# Calendar

Self-hosted Desk shared calendar: human agenda page + full agent CLI parity,
explicit IANA timezones, Desk owner/invite/member audience (404 for non-members),
durable Git-directory `save`/`load`, and RFC 5545 ICS + Google Calendar interop.

Run from this directory with `bashy dag serve`, or use `bashy awd PATH -- bashy dag serve`.

## Tasks

### serve
Build and start the loopback-only calendar server (default 127.0.0.1:18099).
Effects: read, write, exec, net

```bash
"$BASHY" ./calendar.sh serve
```

### test
Run the desk contract test and unit tests.
Effects: read, write, exec

```bash
"$BASHY" ./calendar.sh test
```

### register
Project the calendar into an ordinary command record, skill and shaped launcher tile.
Effects: read, write, exec

```bash
"$BASHY" ./register.sh
```

### show
Read the registered command and skill face.
Effects: read, write, exec, destroy, net

```bash
"$BASHY" commands show calendar
"$BASHY" skill show calendar
```

### roundtrip
Run the real-browser Playwright end-to-end round-trip on an isolated gate-host lane (PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs).
Effects: read, write, exec, net

```bash
rt_dir=$(mktemp -d "${TMPDIR:-/tmp}/calendar-rt.XXXXXX")
trap 'rm -rf "$rt_dir"' EXIT
(GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$rt_dir/calendar" .)
CALENDAR_BIN="$rt_dir/calendar" node browser-test.mjs
```
