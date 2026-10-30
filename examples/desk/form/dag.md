---
name: form
description: Ad-hoc loopback forms an agent defines and a human fills
type: dag
default: serve
---

# Form (F1) — agent-defined forms, human-filled on the loopback page

Run from this directory with `bashy dag serve`, or use `bashy awd PATH -- bashy dag serve`.

## Tasks

### serve
Build and start the loopback-only form server (default 127.0.0.1:18107).
Ctrl-C stops it; saved forms remain.
Effects: read, write, exec, net

```bash
"$BASHY" --bashsharp ./form.bsh serve
```

### demo
Define the contact example, print its loopback URL, and wait for answers.
Fill the page, then watch the agent door return the answers as JSON.
Effects: read, write, exec, net

```bash
"$BASHY" --bashsharp ./form.bsh new --def ./examples/contact.json --title "Contact preferences"
"$BASHY" --bashsharp ./form.bsh open contact
```

### test
Run the shared desk contract test plus the focused form red/green tests.
Effects: read, write, exec

```bash
"$BASHY" --bashsharp ./form.bsh test
```

### register
Project the one Bash# file into an ordinary command record and executable skill.
Effects: read, write, exec

```bash
"$BASHY" ./register.sh
```

### show
Read the registered executable body and skill face.
Effects: read

```bash
"$BASHY" commands show form
"$BASHY" skill show form
```

### launcher
Offer the running server as a tile in the `bashy app` launcher (run while
the server from `serve` is up; replace PORT with its loopback port).
Effects: read, write, exec

```bash
bashy app add form --port PORT --label "Desk form"
```
