---
name: vlog
description: bashy dag front door for examples/vlog — build a static vlog, serve it locally, publish via Outpost or GitHub Pages
type: dag
default: build
vars:
  - SITE ?= sample
  - ADDR ?= 127.0.0.1:18090
  - NAME ?= vlog
  - PORT ?= 18090
  - REPO ?=
  - BRANCH ?= gh-pages
---

# vlog — task graph

A small video blog, authored and hosted on your own machine. `vlog.sh build`
turns `SITE/posts/*.md` (each with a video) into a static site; `serve` runs a
loopback-only server; `publish-outpost` lets a paired Outpost share it as a
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

### server
Build the loopback web server for this machine (Go, standard library only).
Effects: read, write, exec, net
Generates: bin/vlog-server

```bash
./vlog.sh server
```

### server-windows
Cross-build the server for a Windows host (bin/vlog-server-windows-amd64.exe).
Effects: read, write, exec, net

```bash
./vlog.sh server windows amd64
```

### serve
Serve SITE/public on ADDR (foreground; Ctrl-C stops).
Requires: build, server
Env: BASHY_HINTS=off
Effects: read, net, exec

```bash
./vlog.sh serve "$SITE" "$ADDR"
```

### publish-outpost
Register the loopback site with this host's Outpost as app NAME: a public
link anyone can open (add --require-login in vlog.sh to restrict it).
Effects: net, exec

```bash
./vlog.sh publish-outpost "$NAME" "$PORT"
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
