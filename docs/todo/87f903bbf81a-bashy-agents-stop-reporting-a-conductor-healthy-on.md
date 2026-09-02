---
id: 87f903bbf81a
kind: task
title: 'bashy agents: stop reporting a conductor healthy on a heartbeat nothing stands behind'
seq: 20
status: done
created: 2026-09-02T19:51:50.790694Z
sprint: 105
closed: 2026-09-02T19:51:54.664127Z
---

`bashy agents` graded a conductor row on the lease heartbeat alone and reported healthy with nothing running. Consult the process the lease names (dead attached watch => stale), release the seat on every watch-detach path, and grade a heartbeat past the sprint's own cutoff as overrun rather than healthy — reported, not hidden, since a conductor legitimately running long is exactly the row the operator needs. Gate: go vet, go test ./..., make test, Windows cross-build.
