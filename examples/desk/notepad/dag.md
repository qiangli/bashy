---
name: notepad
description: Desk notepad tool (view any content as text, edit Markdown in loopback page)
type: dag
default: serve
---

# Notepad

Desk notepad: view any content as text (txt, md, html, json, PDF->text),
edit Markdown in the loopback page, agent reads/puts content, lists and shares.

Run from this directory with `bashy dag serve`, or use `bashy awd PATH -- bashy dag serve`.

## Tasks

### serve
Build and start the loopback-only notepad server (default 127.0.0.1:18095).
Effects: read, write, exec, net

```bash
"$BASHY" ./notepad.sh serve
```

### test
Run the desk contract test and unit tests.
Effects: read, write, exec

```bash
"$BASHY" ./notepad.sh test
```

### register
Project the notepad into an ordinary command record, skill and launcher tile.
Effects: read, write, exec

```bash
"$BASHY" ./register.sh
```

### show
Read the registered command and skill face.
Effects: read, write, exec, destroy, net

```bash
"$BASHY" commands show notepad
"$BASHY" skill show notepad
```
