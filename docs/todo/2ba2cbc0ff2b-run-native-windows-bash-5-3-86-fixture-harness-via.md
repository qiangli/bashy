---
id: 2ba2cbc0ff2b
kind: test
title: Run native Windows Bash 5.3 86-fixture harness via passwordless Outpost SSH
seq: 319
status: done
priority: p1
labels:
    - windows
created: 2026-09-23T07:04:03.131632Z
assignee: codex-gpt-5.5
sprint: 258
sprint_id: 1de8a8da-0946-5daa-9bd7-68555d410da2
sprint_title: Verify Bash 5.3 fixtures on a second Windows host
closed: 2026-09-23T07:32:10.712296Z
closed_by: codex-gpt-5.5
---

Passwordless Outpost SSH access already works in BatchMode. Use final candidate Bashy b192f5f, sh f9bfd142, coreutils 84369e33, yoke 0868b481 with the pinned GNU Bash 5.3 corpus. Record exact listed/runnable/pass/fail/time/skip counts and locale provider setup.

Windows build 10.0.19045.6466, native amd64: all 86 fixtures listed and runnable. The verified GNU Bash 5.3 corpus was fetched by `bash53fixtures`, preserving executable modes. Seven locales were provisioned through a prepared host locale provider; the harness used its provider interface. Final full run: 85 passed, 1 failed (`printf`), 0 skipped, 0 timed out. The `printf` mismatch is line 280 of `printf.right`: expected `16:09:15`, got `13:09:15` under the fixture's POSIX `TZ` setting. An earlier full pass failed `redir`; an isolated rerun passed, and the final full run passed it. Logs and host-specific setup remain outside the repository.

Follow-up in Sprint #257 story `2b03805c1605`: a general POSIX `TZ` resolver and embedded IANA timezone data fixed the explicit `TZ` lookup. The patched final candidate passed all 86 fixtures on this Windows build with 0 failures, skips, or timeouts. The complete evidence and testee hash are in that story.
