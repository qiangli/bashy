---
id: 1dc7db5ce501
kind: bug
title: K2 Outpost, podman and satellite support Klatch depends on
seq: 3125
status: todo
priority: p1
labels:
    - outpost
created: 2026-11-06T18:13:56.477716Z
---

From Sprint 563 UAT: F5 podman machine listed running but API refused after a Windows reboot until stop/start; F25 restart-policy always containers not restarted at machine start (J7 on puppy 11-06); F22 outpost ssh exec joins arguments without quoting (~, $VAR, backslashes, braces, parentheses mangled); F23 outpost ssh exec ends long-running commands silently with exit 0; F24 Bashy-managed rootful WSL podman machine does not forward container ports to Windows localhost; F16 Bashy on Windows intermittently reports external programs as command not found; satellite tickets T1 roles hardcoded admin, T2 ~60 s before a new app is reachable, T3 anonymous rate limit. Fix the Bashy/outpost items with red/green tests and rerun J7 on puppy; file the satellite tickets (cloudbox is proprietary, no shaping). Carried from Sprint 563 (docs/plan-s563-master-execution.md). KISS/MVP: reuse existing code and harnesses; smallest causal repair. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
