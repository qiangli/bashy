---
id: a555c551303e
kind: requirement
title: H1 Define the shaping workflow for CLI and service apps
seq: 3055
status: todo
priority: p1
labels:
    - shaping
created: 2026-11-04T06:51:46.948421Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

Define the states start, test, use, release, status and submit, and the failure behavior, on existing weave, self build/install, service and tool pin commands. Define the identity record schema (source commit, branch, build artifact hash, test verdict, host, actor and represented user). Define the isolation and per-host rules. Two app kinds with worked examples: CLI = bashy itself (side-slot binary, release preserved, PATH unchanged); service = Klatch, from the Sprint 563 F14 manual shaping (host git, two podman builds, image swap, secret copy; commits 469031ce, 0fe385ad). Acceptance: a reviewed workflow and command contract that H2-H5 implement without inventing a framework.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. KISS/MVP: compose existing weave (workspace/branch), self build/install, service and tool pin commands; no new framework or store; --json; same commands and authorization for humans, agents and agent-assisted humans. Shaping always in a weave workspace (never a shared checkout); swaps affect only this host/user; a failing shaped build is never used; no satellite needed. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
