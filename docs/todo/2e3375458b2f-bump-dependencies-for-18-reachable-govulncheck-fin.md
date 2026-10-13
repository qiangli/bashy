---
id: 2e3375458b2f
kind: security
title: Bump dependencies for 18 reachable govulncheck findings
seq: 1905
status: todo
priority: p2
labels:
    - release
    - security
created: 2026-10-13T04:20:52.167528Z
---

Go 1.27.2 has 0 reachable standard-library findings. Reachable third-party findings: GO-2026-5062, GO-2026-5066, GO-2026-5970, GO-2026-6061, GO-2026-6213, GO-2026-6214, GO-2026-6303, GO-2026-6348, GO-2026-6354, GO-2026-6355, GO-2026-6505, GO-2026-6508, GO-2026-6603, GO-2026-6610, GO-2026-6611, GO-2026-6612, GO-2026-6615, GO-2026-6617. Bump producers, refresh pins, and rerun make govulncheck until reachable=0.
