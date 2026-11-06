---
id: 7da98b722aca
kind: bug
title: Recognize declared Go fence command exports in Bashy static checks
seq: 3095
status: done
priority: p0
created: 2026-11-05T10:33:40.13233Z
weave: 19
assignee: s562-go-checker
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
closed: 2026-11-05T11:24:08.391744Z
closed_by: codex-gpt6-sol
---

Exact final Go corpus build-only context, http-server and tcp-server invoke g.Main as a shell command. Runtime supports that explicit command boundary, but Bashy check --bashsharp wrongly classifies g.Main as a missing PATH executable. Base a4f5e7bb1385a7c2eafe69d3a716f49b1738c937. Narrow parsed-symbol admission for declared exported Go fence callable commands; reject missing namespace/function, unexported and noncallable members, and arbitrary dotted commands. Preserve ordinary PATH checking, typed calls and original fixtures. Do not execute user Go programs during checking. Tests: three unchanged static positives plus explicit negatives and ordinary PATH controls. Parent independently runs exact CLI gate before integration. No canonical publication.
