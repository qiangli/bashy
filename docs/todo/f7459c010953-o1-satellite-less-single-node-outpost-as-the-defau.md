---
id: f7459c010953
kind: feature
title: O1 Satellite-less single-node outpost as the default install
seq: 3058
status: todo
priority: p3
labels:
    - outpost
created: 2026-11-04T06:51:49.525232Z
---

A clean install runs outpost single-node with no satellite and no cloudbox account; local apps and Bashy services work; pairing is optional and later. Related Sprint 563 J1. KISS/MVP: reuse existing Bashy tool pin, build and service commands; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

Deferred 2026-11-07 by the operator (not urgent). Unlinked from Sprint 565. Inspection findings (2026-11-07): an unpaired daemon boots and app CRUD works (TestE2E_StatusUnpaired, TestE2E_AppCRUD); the main and admin listeners bind loopback only (127.0.0.1:0 and 127.0.0.1:17777); the matrix tunnel still dials the default 127.0.0.1 when unpaired; no LAN path to apps was found.
