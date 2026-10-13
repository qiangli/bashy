---
id: b2ef2f0b0835
kind: task
title: Repair isolated service gate fixtures after signed Tuwunel and seed defaults changes
seq: 1906
status: assigned
priority: p1
created: 2026-10-13T09:18:16.188561Z
assignee: s434-bashy-pins
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
---

Remote exact Bashy2073b73c full gate fails TestCheckServiceBuildRefusesUnsignedToolPin and TestServiceCLIRefusesUnsignedBuild because tests assume now-signed tuwunel is unsigned. TestServiceMonitorSeedInternalAndCustomShareOneMechanism fails disabled-seed assertion; apps port22749 log is incidental until isolated. Test-only repair: use explicit unsigned synthetic fixture and isolated service defaults, preserve real production refusal checks. Base2073b73c, do not change production policy/source or weaken assertions/skip tests. Focused three tests only on Dragon; full candidate gate remote later. Deadline09:22 preserve partial if not done. Commit Story/Sprint trailers, no main/push.
