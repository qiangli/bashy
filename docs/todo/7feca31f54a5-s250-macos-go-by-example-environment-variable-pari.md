---
id: 7feca31f54a5
kind: bug
title: S250 macOS Go by Example environment variable parity
seq: 328
status: done
priority: p0
labels:
    - macos
    - gosource
created: 2026-09-23T16:22:31.086703Z
weave: 22
assignee: codex-s250
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-23T17:20:41.743213Z
closed_by: codex-s250
---

Full authenticated novidesign Go by Example gate, candidate 4f2d2d7a, executed 255/255 but interpreted examples/environment-variables/environment-variables.go mismatched oracle/compiled. Its os.Environ output includes BASHY_HARD_IGNORE only in interpreted mode; raw evidence at ~/s250-8aaa6171-evidence/podman-recovered-529e0ac/go-by-example-s250/evidence.fail. Diagnose startup environment snapshot versus harness injection, preserve original source/comparison, fix and rerun exact row/full gate.
