---
name: memo
description: Desk-contract fixture tool (minimal memo server proving the shared kit)
type: dag
default: serve
---

# Memo fixture

Minimal desk tool used by K0 to prove `examples/desk/kit` and the shared
contract test. Real tools copy this shape.

Run from this directory with `bashy dag serve`.

## Tasks

### serve
Build and start the loopback-only memo server (default 127.0.0.1:18097).
Effects: read, write, exec, net

```bash
"$BASHY" ./memo.sh serve
```

### test
Run the shared contract test green plus the red-on-break proof.
Effects: read, write, exec

```bash
"$BASHY" ./memo.sh test
```

### register
Project the fixture into an ordinary command record for the atlas check.
Effects: read, write, exec

```bash
"$BASHY" ./register.sh
```
