---
name: vlog
description: bashy dag front door for examples/vlog — build a static vlog, serve it from the bashy console, publish via Outpost or GitHub Pages
type: dag
default: build
vars:
  - SITE ?= sample
  - NAME ?= vlog
  - CONSOLE_PORT ?= 22749
  - REPO ?=
  - BRANCH ?= gh-pages
---

# vlog — task graph

A small video blog, authored and hosted on your own machine. `vlog.sh build`
turns `SITE/posts/*.md` (each with a video) into a static site; `serve`
registers it with the bashy console, which serves the directory itself at
/NAME/; `publish-outpost` lets a paired Outpost share that console path as a
link anyone can open; `publish-pages` pushes the same site to GitHub Pages.
Run it from anywhere by naming the folder: `bashy dag examples/vlog build
SITE=$PWD/my-vlog`.

## Tasks

### build
Generate SITE/public from SITE/posts.
Env: BASHY_HINTS=off
Effects: read, write

```bash
./vlog.sh build "$SITE"
```

### serve
Register SITE/public with the bashy console as app NAME (`app add --type
site`); the console serves it at http://127.0.0.1:CONSOLE_PORT/NAME/.
Re-running updates the same record. If the console is not running, serve
says how to start it (`bashy app serve`); it never starts one itself.
Requires: build
Env: BASHY_HINTS=off
Effects: read, write, exec, net

```bash
CONSOLE_PORT="$CONSOLE_PORT" ./vlog.sh serve "$SITE" "$NAME"
```

### publish-outpost
Make app NAME public (`bashy app set NAME --auth public`: the Outpost link
reaches the console with no login), then register this host's Outpost app
NAME -> http://127.0.0.1:CONSOLE_PORT/NAME/. Anyone with the link can read
the site; undo with `outpost apps rm NAME` and `bashy app set NAME --auth
system`.
Requires: serve
Effects: exec, net

```bash
CONSOLE_PORT="$CONSOLE_PORT" ./vlog.sh publish-outpost "$NAME"
```

### publish-pages
Push SITE/public to REPO's BRANCH for GitHub Pages: `REPO=owner/name`.
Requires: build
Env: BASHY_HINTS=off
Effects: read, write, net, exec

```bash
[ -n "$REPO" ] || { echo "set REPO=owner/name" >&2; exit 2; }
./vlog.sh publish-pages "$SITE" "$REPO" "$BRANCH"
```
