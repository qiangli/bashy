---
id: 0bce89792a25
kind: bug
title: Restore pure strict POSIX sh entrypoint for licensed shell gate
seq: 334
status: done
priority: p0
labels:
    - posix
    - release
    - shell
created: 2026-09-24T00:24:38.235204Z
assignee: codex-s250
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-24T02:12:59.830382Z
closed_by: codex-s250
---

Regression introduced by Bashy 501aa5d (Sprint253 Story712): cmd/bash now sets BashDropinShMode=true, so the VSC Profile B harness invoking pure bin/bash as sh loses WithStrictPosix and sh_07 TP7 fails (492/493). Historical Sep21 493/493 on Bashy ca58e133/sh3c3233 used the same pure Bash wiring. Public four-line named script with readonly x and x=5 date then echo after: pre-501 pure cmd/bash-as-sh rc1/empty; post-501 rc0/after on identical sh175, while bash --posix remains rc0/after in both. Deliver a lean pure strict cmd/sh binary and build target bin/sh{,.real} for certification, retaining cmd/bash GNU behavior, six-platform archive semantics and no suite/limit change. Acceptance: focused public TP7 and GNU Bash controls; native build/launcher behavior; Bashy three-OS CI; serial GNU86; fresh approved VSC493/493 with unchanged external providers. No release tag until all gates pass.

Acceptance: Bashy 7416f0b three-OS test workflow 35941029862 passed; serial macOS GNU Bash 5.3 passed 86/86, log SHA-256 1aada13dd8db9a8f85353210bf625773d75dc456dd295c4024c0db2fc798302f. The approved strict shell DAG arm passed 493/493 with zero missing TPs, caps, runner failures, or blockers; sealed ledger SHA-256 6ac2daf9ec384566e295e78182780885f9f964ecd87794183fa6e2fb0fe37aad. Published v0.28.0 archive probes on macOS, Linux, and Windows confirmed strict sh exits on the public readonly-assignment case while GNU bash continues. Both release tags point at Bashy 7416f0b.
