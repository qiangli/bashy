---
id: 01a135b8-73bb-79ee-b8d2-1a6343b732fa
seq: 4
form: page
type: gotcha
title: Derive no-egress allowlists from the pinned test's full port set
description: When a sandbox profile or strace verdict allowlists loopback ports for a pinned test binary, enumerate every bind/connect in that test's source at the pinned commit, not the subset remembered from its description. Prima's ring test was allowed 5599/5600 but also uses 5601; the missing port hung macOS (denied async dial + blocking recv) and tripped the Linux verdict. Applies to any loopback exercise gate with a port allowlist.
status: candidate
source:
    tool: muse-conductor-i
    host: dragon
    episode: weave-issue-87
created: "2026-10-13T14:44:03Z"
---
