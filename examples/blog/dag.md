---
name: blog
description: bashy dag front door for examples/blog — a personal journal with rich media
type: dag
default: build
vars:
  - SITE ?= sample
  - NAME ?= blog
  - CONSOLE_PORT ?= 22749
  - REPO ?=
  - BRANCH ?= gh-pages
---

# blog — task graph

A personal blog or wiki authored in plain Markdown with the media the bashy
families produce: video and voice takes with captions, recorded terminal
scenes, diagrams, colored code and math. Builds into a static site locally,
serves from this machine, shares via Outpost or a Pages branch, and is a
shapeable app. Run from anywhere by naming the folder:
`bashy dag examples/blog build SITE=$PWD/my-journal`.

## Tasks

### new
Scaffold a journal from the sample and build it once.
Effects: read, write

```bash
./blog.bsh new "$SITE"
```

### post
Create a dated draft post. Extra args pass through (--series, --part).
Env: BASHY_HINTS=off
Effects: write

```bash
./blog.bsh post "First note" --dir "$SITE"
```

### build
Build SITE into SITE/public (Hugo CLI if installed, else the pure-Go generator).
Effects: read, write, exec

```bash
./blog.bsh build "$SITE"
```

### media
Attach a media file to a post (kind by extension: video/audio/cast/captions).
Args after `add` are DIR POST.md FILE.
Effects: read, write

```bash
./blog.bsh media "$SITE" content/posts/first-note.md add take.mp4
```

### captions
OPT-IN local transcription of a post's video/audio into a .vtt caption file
(needs the transcribe pipeline registered; nothing leaves the machine).
Effects: read, write, exec

```bash
./blog.bsh captions "$SITE" content/posts/first-note.md
```

### serve
Register SITE/public with the bashy console as app NAME.
Requires: build
Effects: read, write, exec

```bash
CONSOLE_PORT="$CONSOLE_PORT" ./blog.bsh serve "$SITE" "$NAME"
```

### edit
Show the admin CMS URL (bundled editor at /admin/).
Effects: read

```bash
CONSOLE_PORT="$CONSOLE_PORT" ./blog.bsh edit "$NAME"
```

### publish-outpost
Make app NAME public and register it with this host's Outpost.
Requires: serve
Effects: exec, net

```bash
CONSOLE_PORT="$CONSOLE_PORT" ./blog.bsh publish-outpost "$NAME"
```

### publish-pages
Push SITE/public to REPO's BRANCH for GitHub Pages.
Requires: build
Effects: read, write, net, exec

```bash
[ -n "$REPO" ] || { echo "set REPO=owner/name" >&2; exit 2; }
./blog.bsh publish-pages "$SITE" "$REPO" "$BRANCH"
```

### shape-register
Register the source as a shapeable app (bashy app add --type shaped).
Effects: read, write, exec

```bash
./blog.bsh shape-register "$SITE" "$NAME"
```

### shape-demo
The full shaping lifecycle on a hand-made series-index candidate:
wish -> candidate -> verify -> preview -> approve -> isolation -> undo.
Effects: read, write, exec

```bash
./blog.bsh shape-demo "$SITE" "$NAME"
```

### register
Register the command, the skill and the shaped app.
Effects: read, write, exec

```bash
BASHY_BIN="${BASHY_BIN:-bashy}" bash register.sh "$NAME"
```

### test
Run this example's targeted tests (module + script end to end).
Effects: read, write, exec

```bash
GOWORK=off go test -v ./...
```
