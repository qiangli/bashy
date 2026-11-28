---
id: fa74df82ef8a
kind: feature
title: 'M3 Go-backed lifted surface: build Go, judge on a synthetic temp DB, enforce a declared candidate file set'
seq: 3334
status: todo
priority: p0
labels:
    - shapeware
created: 2026-11-28T06:50:10.608245Z
sprint: 742
sprint_id: 7dde34d4-f382-5aba-8ff5-071c5e3548b8
sprint_title: 'Shaping: bugs and issues become spec changes (issue-to-spec support)'
---

The lift template and candidate grader only cover a presentation module (classgo reports-weekly-summary Build rule 4 forbids Go, templates, schema). Add the variant to the lift runbook and spec format: dag build runs go build; the contract starts the reference on a fresh temp SQLite with synthetic data and a loopback port; the surface declares the files a candidate may change (metadata or contract.bsh) and candidate.bsh enforces exactly that set; effect cap covers loopback serve and temp-dir writes only. First users: classgo Check-in lift and the 741 narrow lifts. Verification: one classgo Go-backed surface lints, certs 3/3 and kills every mutant.
