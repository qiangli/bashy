---
id: 8a9f0dede7c6
kind: task
title: 'Hosted installers 404: dhnt.io/install.sh and install.ps1 -> ai.dhnt.io 404'
seq: 3192
status: todo
created: 2026-11-08T02:32:02.7944Z
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
---

Since ~2026-11-08 02:20 UTC: https://dhnt.io/install.sh and /install.ps1 return 301 to https://ai.dhnt.io/install.{sh,ps1}, which return 404 (portal root is 200). Both worked ~01:50 UTC (codex cold-start). README's primary one-liner is broken; muse's cold start fell back to the GitHub release tarball. Restore the routes on the hosted front door (public wire behaviour only).
