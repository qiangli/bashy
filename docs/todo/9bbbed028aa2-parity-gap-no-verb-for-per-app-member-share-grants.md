---
id: 9bbbed028aa2
kind: feature
title: 'Parity gap: no verb for per-app member share grants'
seq: 3208
status: done
priority: p2
labels:
    - parity
created: 2026-11-10T14:02:07.576759Z
closed: 2026-11-10T14:46:41.349739Z
---

docs/ui-cli-parity.md Cloudbox Hosts page + satellite console 'share with members' rows are console-only. satellite-member publish stops at outpost apps add and hands the grant to the human. Need a verb (or outpost/satellite surface) to mint, list, and revoke per-app member share grants with --json and --dry-run.

Done 2026-11-10 (S606 S7, story d7933b7c): cloudbox 7dfc4c7 (Bearer host shares on standalone+satellite, host-own token), outpost 993631d (outpost apps share|unshare|shares), bashy satellite-member share.
