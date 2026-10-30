---
name: viewer
description: Desk viewer tool (view-only books of agent reports, rendered by the kit renderer)
type: dag
default: serve
---

# Viewer

Desk viewer: a view-only reader. Agents add books and append pages from
Markdown or from a template + JSON data; humans read, page, compare and
export them in the loopback page.

Run from this directory with `bashy dag serve`, or use `bashy awd PATH -- bashy dag serve`.

## Tasks

### serve
Build and start the loopback-only viewer server (default 127.0.0.1:18096).
Effects: read, write, exec, net

```bash
"$BASHY" ./viewer.sh serve
```

### test
Run the view-only desk contract test, red/green tests and unit tests.
Effects: read, write, exec

```bash
"$BASHY" ./viewer.sh test
```

### browser-test
Real-browser check (needs Playwright: PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs).
Effects: read, write, exec, net

```bash
(GOWORK=off CGO_ENABLED=0 go build -trimpath -o "${TMPDIR:-/tmp}/viewer-bt" .)
VIEWER_BIN="${TMPDIR:-/tmp}/viewer-bt" node browser-test.mjs
```

### register
Project the viewer into an ordinary command record, skill and launcher tile.
Effects: read, write, exec

```bash
"$BASHY" ./register.sh
```

### show
Read the registered command and skill face.
Effects: read, write, exec, destroy, net

```bash
"$BASHY" commands show viewer
"$BASHY" skill show viewer
```
