---
id: 2ff43d03b869
kind: bug
title: 'S539.F8 Explicit @sandbox() island: fix doubled bashy argv0, launch the running candidate, Windows exit 126 (known issue 6)'
seq: 2855
status: todo
priority: p0
labels:
    - release
created: 2026-10-30T15:51:31.849651Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Moved from S525 goal a5w (operator 2026-10-30). Known issue 6 for v1.0.0-rc.23 (operator-approved): an explicit @sandbox() island does not work; it fails closed and never runs unsandboxed. Found by S525.R23c on rc.23-dev 51271e35, elephant: (a) the anonymous pull of ghcr.io/qiangli/bashy-scratch:51271e35 PASSES (digest sha256:1add847f...). (b) WSL: HTTP 200 with the bashy help doc as the body, because the image ENTRYPOINT is /bashy and yoke pkg/shape/contain/oci.go BashyOCI.Argv appends [bashy -c BODY], so the container runs '/bashy bashy -c ...'. Repro: 'bashy oci run --rm IMG bashy -c "echo hi"' prints help; 'IMG -c "echo hi"' prints hi. (c) BashyOCI.Bin is empty, so the launcher is 'bashy' on PATH, not the running candidate (same class as S525.A9d). (d) Native Windows: HTTP 500 'unit probe failed (exit 126)' after app add + serve on a fresh BASHY_HOME. Raw receipts on elephant: WSL /root/s525-rc23/a5w-b, Windows C:/Users/liqia/s525-rc23/a5w-win-b2. Fix: don't prepend argv0 when the image entrypoint is bashy; launch the running binary; root-cause the Windows exit 126. Red/green tests on WSL and native Windows; then a bare @sandbox() unit returns its output on both OSes. Ships in rc.24; close the ledger entry then.
