---
id: d038cf56a420
kind: feature
title: S537.K0 desk contract + UI kit + shared contract test, extracted from whiteboard/brainstorm
seq: 2931
status: todo
priority: p1
labels:
    - desk
created: 2026-10-30T02:45:42.409713Z
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
---

8-point desk contract (two front doors: agent CLI/JSON API + human web page; one CLI/JSON API registered in atlas; stable id + URL reopen; any surface; local-first loopback; shapable by Software Shaping; no secrets (desk forms stay separate from yoke/pkg/ask); same tests on every OS).

UI kit: copy button + "copied" state, status badges, field/question components, item header.

Do:
1. Read examples/whiteboard and examples/brainstorm servers; extract the common contract.
2. Create shared module examples/desk/kit (Go helpers, embedded CSS/JS, and a contracttest package) consumed by each tool via a relative `replace`. Keep it standalone and dependency-light; not a yoke package. Confirm or justify a different placement in the delivery note.
3. Contract test green against a small fixture tool (examples/desk/kit/internal/fixture or similar). Red/green: show it fails on a fixture that breaks one contract point.
4. Verify that a tool registered via `bashy commands add` gets an atlas entry, and that an example app can be shaped (Software Shaping). Any gap = an in-sprint bug: fix it minimally in bashy, or report it precisely in the submit note.
Hosting: the bashy app launcher hosts tools (not yoke/pkg/broker). New tools bind loopback; whiteboard/brainstorm keep LAN (do not change them in K0; R1 retrofits them).
Tests: focused `go test` only on this host; no podman.
Spec (read-only, umbrella): $HOME/projects/poc/dhnt/docs/sprint-537-bashy-desk-plan.md
