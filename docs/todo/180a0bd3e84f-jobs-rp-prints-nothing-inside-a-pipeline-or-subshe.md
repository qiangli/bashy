---
id: 180a0bd3e84f
kind: bug
title: jobs -rp prints nothing inside a pipeline or $(...) subshell
seq: 3237
status: todo
priority: p1
labels:
    - shell
created: 2026-11-14T12:43:50.868015Z
---

Found 2026-11-14 by the cloudbox e2e harness (Sprint 630 S23): `while [ "$(jobs -rp | wc -l)" -ge $N ]; do sleep 1; done` never waits under bashy because the job table is not visible from a pipeline child or a command substitution, so every background job launched at once (26 e2e runs on a 4 GB host). GNU bash 5.3 lists the running jobs in both. Repro: bash -c 'sleep 3 & sleep 3 & jobs -rp | wc -l; echo $(jobs -rp | wc -l); jobs -rp; wait' -> bashy prints 0, 0, then two pids; GNU prints 2, 2, two pids. Top-level `jobs -rp > file` and `wait -n` work. Harness worked around it (cloudbox script/e2e-multisat.sh).
