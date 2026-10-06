---
id: 2e82097d8031
kind: feature
title: 'bashy proxy socks: pure-Go SOCKS5 server'
seq: 368
status: done
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:15.32509Z
weave: 5
assignee: claude-fable5.1
sprint: 343
sprint_id: ed2cbe05-a807-56d6-81dc-a9a04e7a11f5
sprint_title: 'Built-in tunnelling for air-gapped remotes: SOCKS4/4a/5 and HTTP/HTTPS proxy over the bashy peer channel'
closed: 2026-10-06T02:17:24.188952Z
closed_by: claude-fable5.1
---

Owner 2026-09-30; rescoped SOCKS5-only by owner 2026-10-05. Scope (3 pt): SOCKS5 CONNECT with socks5h-style remote DNS (domain-name address type, resolved at the proxy); auth none and username/password; listen-address flag; structured logging. No SOCKS4/4a, no UDP ASSOCIATE, no BIND in v1. Acceptance: red/green tests with a pure-Go SOCKS5 client (golang.org/x/net/proxy) for no-auth, user/pass and a remote-DNS target.

Rationale: bashy owns both proxy ends; SOCKS5 supplies remote DNS and all intended clients speak SOCKS5. No SOCKS4/4a handler or 0990/socks5 vetting is needed. Implementation may use a permissive server or the Go standard library; verify cancellation, listener control and licenses, preserving notices.
