---
id: 29b9661dab3d
kind: feature
title: 'Reverse proxy over the peer channel: give an air-gapped remote egress through the operator machine'
seq: 370
status: done
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:18.517599Z
weave: 7
assignee: claude-fable5.1
sprint: 343
sprint_id: ed2cbe05-a807-56d6-81dc-a9a04e7a11f5
sprint_title: 'Built-in tunnelling for air-gapped remotes: SOCKS4/4a/5 and HTTP/HTTPS proxy over the bashy peer channel'
closed: 2026-10-06T02:23:27.53095Z
closed_by: claude-fable5.1
---

Scope (3 pt): reverse egress for an air-gapped remote = SSH remote port forwarding (tcpip-forward) over the outpost in-process SSH channel (5a917b8b): the remote gets a localhost listener that forwards to the operator's 'bashy proxy socks|http'; remote commands run with ALL_PROXY/HTTPS_PROXY/HTTP_PROXY/NO_PROXY exported. Nothing listens beyond the remote's localhost. Acceptance: red/green; e2e: a remote with no default route fetches a URL and a go module through it.
