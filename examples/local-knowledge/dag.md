---
name: local-knowledge
description: build, test, and run the local-knowledge kb-ui service
type: dag
default: test
vars:
  ADDR ?= 127.0.0.1:6674
  ROOTS ?= .
  BASHY_BIN ?= bashy
---

# Local Knowledge UI — Task Graph

Run this graph with `bashy dag -f examples/local-knowledge/dag.md TARGET`.

## Tasks

### build
Build the kb-ui server binary.
Effects: read, write, exec, net
Generates: bin/kb-ui

```bash
mkdir -p bin
go build -o bin/kb-ui ./examples/local-knowledge/cmd/kb-ui
```

### test
Run the unit and HTTP boundary tests for kb-ui.
Effects: read, write, exec, net

```bash
go test -v ./examples/local-knowledge/cmd/kb-ui/...
```

### smoke
Build and smoke-test the kb-ui server healthz endpoint.
Requires: build
Effects: read, write, exec, net

```bash
bin/kb-ui --addr 127.0.0.1:16674 --roots . &
PID=$!
trap "kill $PID 2>/dev/null || true" EXIT

# Wait for server to bind
for i in $(seq 1 30); do
  if curl -sf http://127.0.0.1:16674/healthz >/dev/null; then
    echo "kb-ui healthy on 127.0.0.1:16674"
    break
  fi
  sleep 0.2
done

# Verify healthz response
RES=$(curl -s http://127.0.0.1:16674/healthz)
case "$RES" in
  *'{"ok":true}'*) echo "smoke: healthz passed" ;;
  *) echo "smoke: unexpected healthz response: $RES" >&2; exit 1 ;;
esac

# Verify roots endpoint
RES=$(curl -s "http://127.0.0.1:16674/api/roots")
case "$RES" in
  *'"roots"'*) echo "smoke: roots API passed" ;;
  *) echo "smoke: unexpected roots response: $RES" >&2; exit 1 ;;
esac

kill $PID 2>/dev/null || true
trap - EXIT
```

### run
Launch the kb-ui local knowledge server.
Requires: build
Effects: read, write, exec, net

```bash
bin/kb-ui --addr "$ADDR" --roots "$ROOTS" --bashy "$BASHY_BIN"
```
