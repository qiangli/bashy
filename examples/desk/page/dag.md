---
name: page
description: Desk page tool (one fixed, live page for a group, composed of the desk apps)
type: dag
default: serve
---

# Page

Desk page: Markdown plus ```block fences, each a live view over an item
another desk app owns; one URL, one audience, one record. The agent adds,
sets, shares and exports; the group opens the page.

Run from this directory with `bashy dag serve`, or use `bashy awd PATH -- bashy dag serve`.

## Tasks

### serve
Build and start the loopback-only page server (default 127.0.0.1:18113).
Effects: read, write, exec, net

```bash
"$BASHY" ./page.sh serve
```

### test
Run the desk contract test and unit tests.
Effects: read, write, exec

```bash
"$BASHY" ./page.sh test
```

### register
Project the page into an ordinary command record, skill and launcher tile.
Effects: read, write, exec

```bash
"$BASHY" ./register.sh
```

### publish-outpost
Put the page app behind the satellite door, login required.
Effects: read, write, exec, net

```bash
"$BASHY" ./page.sh publish-outpost
```

### show
Read the registered command and skill face.
Effects: read, write, exec, destroy, net

```bash
"$BASHY" commands show page
"$BASHY" skill show page
```
