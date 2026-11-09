---
id: 8f65d384b73d
kind: test
title: S601.4 Hermetic loopback HTTP mock server E2E test suite (examples/google/test_e2e_loopback.py)
seq: 3202
status: todo
priority: p1
created: 2026-11-09T09:04:54.099082Z
sprint: 601
sprint_id: 3f93eee1-fb25-5828-a779-966c345ff576
sprint_title: Google Workspace custom command MVP (examples/google)
---

Create standalone hermetic loopback E2E test suite examples/google/test_e2e_loopback.py (do not modify google.bsh or other files). Spin up an in-process Python http.server.ThreadingHTTPServer on 127.0.0.1:0 simulating Google Workspace API endpoints and a fake gws shim that forwards requests to the local HTTP loopback server, verifying end-to-end HTTP payload delivery, status codes, partial-mutation handling in drive-share-mail, mail-to-task linkage, and agenda aggregation across Gmail, Calendar, Tasks, and Drive.
Verify with: python3 -m unittest examples/google/test_e2e_loopback.py -v
