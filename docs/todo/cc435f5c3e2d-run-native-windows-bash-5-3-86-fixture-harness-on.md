---
id: cc435f5c3e2d
kind: test
title: Run native Windows Bash 5.3 86-fixture harness on final Sprint 253 pins
seq: 318
status: done
priority: p1
labels:
    - windows
created: 2026-09-23T06:52:28.134839Z
assignee: codex-gpt-5.5
sprint: 257
sprint_id: 14e6cca7-6d3d-5712-b474-70aa075ad503
sprint_title: Verify final Sprint 253 Bash 5.3 candidate on Windows
closed: 2026-09-23T07:32:10.753665Z
closed_by: codex-gpt-5.5
---

Inspect host prerequisites and use final candidate Bashy b192f5f, sh f9bfd142, coreutils 84369e33, yoke 0868b481. Run the same pinned GNU Bash 5.3 fixture corpus through tools/bash53suite on native Windows; record listed/runnable/pass/fail/time/skip and setup details.

Windows build 10.0.26200.9457, native amd64: all 86 fixtures listed and runnable. The verified GNU Bash 5.3 corpus was fetched by `bash53fixtures`, preserving executable modes. Seven locales were provisioned through a prepared host locale provider; the harness used its provider interface. Final full run: 85 passed, 1 failed (`printf`), 0 skipped, 0 timed out. The `printf` mismatch is line 280 of `printf.right`: expected `16:09:15`, got `13:09:15` under the fixture's POSIX `TZ` setting. An earlier full pass timed out on `read` at the 60-second per-fixture limit; an isolated rerun passed in 16.647 seconds, and the final full run passed it. Logs and host-specific setup remain outside the repository.

Follow-up in Sprint #257 story `2b03805c1605`: a general POSIX `TZ` resolver and embedded IANA timezone data fixed the explicit `TZ` lookup. The patched final candidate passed all 86 fixtures on this Windows build with 0 failures, skips, or timeouts. The complete evidence and testee hash are in that story.
