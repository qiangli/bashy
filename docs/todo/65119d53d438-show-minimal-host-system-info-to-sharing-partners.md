---
id: 65119d53d438
kind: feature
title: Show minimal host system info to sharing partners for operations and debugging
seq: 3176
status: todo
priority: p2
labels:
    - satellite
    - outpost
created: 2026-11-07T21:22:50.523725Z
---

Operator requirement 2026-11-07 (from Sprint 587): sharing is a trusted relationship, so a sharing partner should see a host's non-sensitive operational info: hostname and alias, online status and last seen, names of shared apps (all visible today), plus CPU model/count, memory, disk total/free, GPU kind/model/count/VRAM, OS/platform/arch, Bashy and outpost versions, and IP. Outpost already reports everything except IP to the host owner. Change: (1) hosted satellite/cloudbox (private repository): include the host's system info and versions in the shared-with-me view; (2) outpost: IP reporting was added and then reverted on operator approval on 2026-11-04 ('Neighborhood display is sufficient'), so re-decide which address (LAN, tunnel or public) is wanted before re-adding. Never include secrets, tokens, file paths, environment, process lists or usernames.

Operator decision 2026-11-07 on IP: show the LAN address for now, but collect ALL addresses (LAN, tunnel and public) and make them available to cloudbox and the satellites.
