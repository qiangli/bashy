---
name: site
description: bashy dag front door for examples/site — simple company website and web presence
type: dag
default: build
vars:
  - SITE ?= sample
  - NAME ?= site
  - CONSOLE_PORT ?= 22749
  - REPO ?=
  - BRANCH ?= gh-pages
---

# site — task graph

Simple company website and presence, authored locally, served from your own machine,
and shared two ways: through the bashy console and Outpost (a link anyone can open),
or on GitHub Pages. Edit via Sveltia CMS at `/admin` and visual drag-and-drop editor.

Run it from anywhere by naming the folder:
`bashy dag examples/site build SITE=$PWD/my-site`

## Tasks

### new
Initialize a new company site project with Hugo layout, Sveltia CMS at /admin, and visual editor.
Effects: read, write

```bash
./site.bsh new "$SITE"
```

### build
Generate static HTML output in SITE/public from Markdown content and layouts.
Env: BASHY_HINTS=off
Effects: read, write

```bash
./site.bsh build "$SITE"
```

### serve
Register SITE/public with the bashy console as app NAME (`app add --type site`);
the console serves it at http://127.0.0.1:CONSOLE_PORT/NAME/.
Re-running updates the same record.
Requires: build
Env: BASHY_HINTS=off
Effects: read, write, exec, net

```bash
CONSOLE_PORT="$CONSOLE_PORT" ./site.bsh serve "$SITE" "$NAME"
```

### edit
Show URL for the Sveltia CMS and visual page editor on loopback.
Effects: read

```bash
CONSOLE_PORT="$CONSOLE_PORT" ./site.bsh edit "$NAME"
```

### publish-outpost
Make app NAME public (`bashy app set NAME --auth public`), then register this
host's Outpost app NAME -> http://127.0.0.1:CONSOLE_PORT/NAME/.
Requires: serve
Effects: exec, net

```bash
CONSOLE_PORT="$CONSOLE_PORT" ./site.bsh publish-outpost "$NAME"
```

### publish-pages
Push SITE/public to REPO's BRANCH for GitHub Pages: `REPO=owner/name`.
Requires: build
Env: BASHY_HINTS=off
Effects: read, write, net, exec

```bash
[ -n "$REPO" ] || { echo "set REPO=owner/name" >&2; exit 2; }
./site.bsh publish-pages "$SITE" "$REPO" "$BRANCH"
```

### shape-register
Register the site as a shapeable app (`bashy app add NAME --type shaped`).
Effects: read, write, exec

```bash
./site.bsh shape-register "$SITE" "$NAME"
```

### test
Run targeted unit tests for the command and shaping lifecycle.
Effects: read, write, exec

```bash
GOWORK=off go test -v ./...
```
