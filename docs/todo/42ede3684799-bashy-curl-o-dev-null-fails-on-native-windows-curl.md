---
id: 42ede3684799
kind: task
title: 'bashy curl -o /dev/null fails on native Windows (curl: (23) client returned ERROR on write)'
seq: 2835
status: done
priority: p1
created: 2026-10-29T17:58:01.949765Z
assignee: claude-sonnet5.5
sprint: 531
sprint_id: f7bc2570-ca8e-59b0-8163-c3c51ba23610
sprint_title: 'Genie core harness: drive vendor agent CLIs as tools'
closed: 2026-10-29T18:43:36.873519Z
closed_by: s531-manager
---

Found in D4 live 10-29 on elephant native Windows with the candidate bashy: 'bashy -c "curl -sS -o /dev/null -w %{http_code} URL"' prints 401, then 'curl: (23) client returned ERROR on write' with exit 23; -o NUL works. The POSIX /dev/null path must map to NUL on Windows for builtin curl output files (and audit other builtins that open output paths). Same-on-every-OS principle. Red/green unit test; focused go test only.
