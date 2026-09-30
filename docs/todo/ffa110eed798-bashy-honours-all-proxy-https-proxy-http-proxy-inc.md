---
id: ffa110eed798
kind: feature
title: bashy honours ALL_PROXY/HTTPS_PROXY/HTTP_PROXY (incl. socks5://) for its own network use
seq: 371
status: todo
priority: p2
labels:
    - remote
created: 2026-09-30T17:19:20.111505Z
sprint: 343
sprint_id: ed2cbe05-a807-56d6-81dc-a9a04e7a11f5
sprint_title: 'Built-in tunnelling for air-gapped remotes: SOCKS4/4a/5 and HTTP/HTTPS proxy over the bashy peer channel'
---

Scope (3 pt): binmgr fetches, native git transport and go module download inside bashy use the standard proxy env vars, including socks5:// and socks5h:// URLs, and NO_PROXY. Acceptance: red/green against the #343 proxies.
