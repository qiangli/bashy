---
id: 38c562a9bc13
kind: feature
title: 'bashy proxy http: pure-Go HTTP forward proxy with HTTPS CONNECT'
seq: 369
status: todo
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:16.967483Z
sprint: 343
sprint_id: ed2cbe05-a807-56d6-81dc-a9a04e7a11f5
sprint_title: 'Built-in tunnelling for air-gapped remotes: SOCKS4/4a/5 and HTTP/HTTPS proxy over the bashy peer channel'
---

Scope (3 pt): HTTP/1.1 forward proxy plus CONNECT tunnelling for HTTPS; optional basic auth; no TLS interception. Acceptance: red/green: curl-equivalent Go client through it for http:// and https:// targets.
