---
name: chart
description: Quick offline SVG statistics on the desk contract
type: dag
default: serve
---

# Chart

Run from this directory. Source, templates and tests are shapable files.

## Tasks

### serve
Build and serve on 127.0.0.1:18098.
Effects: read, write, exec, net

```bash
"$BASHY" --bashsharp ./chart.bsh serve
```

### gate
Focused unit tests, per-widget SVG goldens, OTLP fixture and desk contract.
Effects: read, write, exec, net

```bash
GOWORK=off go test -count=1 -race .
```

### register
Register the command atlas entry and launcher tile.
Effects: read, write, exec

```bash
"$BASHY" ./register.sh
```

### browser
Run only on the designated browser test host with Playwright installed.
Effects: read, write, exec, net

```bash
node browser-test.mjs
```
