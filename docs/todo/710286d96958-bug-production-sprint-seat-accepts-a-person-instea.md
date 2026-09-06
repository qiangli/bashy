---
id: 710286d96958
kind: task
title: 'Bug: production sprint seat accepts a person instead of a registered agent'
seq: 237
status: assigned
priority: p0
created: 2026-09-06T03:27:39.01883Z
assignee: codex-gpt5.6-sol
sprint: 127
---

CONFIRMED AND FIXED. The production sprint-owner validator used the generic principal resolver, so a registered person could occupy an execution seat. Coreutils commit 67b59474 now requires fleet.KindAgent consistently for validation, canonicalization, and reachability; focused regressions refuse unknown, placeholder, and registered-person owners while accepting a registered offline agent. Umbrella commit 47c9e64 replaces the obsolete person-manager/watch path with the two supported modes, adds the secure external-manager watcher/session-claim → authored delegation → exact worker receipt sequence for #227, retains human steering via ping/MB/Meet to a resident agent, and reconciles the #215 identity design: humans remain principals/authors/counterparts but production sprint execution seats are agent-only. Evidence: go test ./pkg/weave; go test ./cmds/... ./tool ./git ./pkg/...; coreutils scripts/crossvet.sh; bash -n script/e2e-sprint-modes.sh; freshly built Bashy ran the hermetic synergy gate 23/23 PASS.
