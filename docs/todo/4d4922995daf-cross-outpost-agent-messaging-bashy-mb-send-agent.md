---
id: 4d4922995daf
kind: feature
title: 'Cross-outpost agent messaging: bashy mb send agent@outpost routed over the outpost mesh'
seq: 3289
status: todo
priority: p3
created: 2026-11-21T15:46:32.094526Z
---

Operator 2026-11-21: file the cross-outpost mb todo (not planned yet). Today mb, inbox, chat steer and weave say are host-local; cross-host options are ad hoc (outpost ssh <host> -- bashy mb send ..., bashy meet serve for remote room clients, sprint stories traveling with git). Shape when picked up: an address form agent@outpost (and agent+N@outpost per the office-phone model in bc800d20), delivered over the existing outpost mesh (mesh-direct / tunnel) to the target host's board, with caller ID of the sending host+agent, owner-scoped authorization (same owner or an owner-granted membership), store-and-forward when the target outpost is offline (never drop), and bashy inbox showing remote senders. Related: owner-level company number todo 193d49d8.
