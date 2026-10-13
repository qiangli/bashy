---
id: fdf2b03216ce
kind: bug
title: Prove Windows UDP no-egress by observed datagram delivery under WFP
seq: 1931
status: done
priority: p1
created: 2026-10-13T18:31:22.7022Z
assignee: s436-codex-manager
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T18:40:06.187568Z
closed_by: s436-codex-manager
---

Hosted run 38445979881 Windows native gate stops before runner: nonblocking UDP sendto returned 7 even though WFP ALE filters are installed, so synchronous WSAEACCES is an invalid proof. Preserve fail-closed dynamic WFP and TCP controls; add local receiver controls that prove permitted UDP delivery on configured loopback port and denied UDP delivery on wrong IPv4 port/IPv6, while TCP nonloopback control verifies shared address condition. Rerun hosted five-target gate. See Microsoft Winsock sendto documentation: success does not prove delivery.
