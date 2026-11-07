---
id: d69e3577abbf
kind: feature
title: D1 kb reads a read-only shared ring from the bashy release source pin
seq: 3178
status: done
priority: p1
labels:
    - kb
created: 2026-11-07T22:07:52.440681Z
assignee: claude-opus5.5-s588
sprint: 588
sprint_id: 2ab4b70e-c274-5779-9e55-8259c38a4204
sprint_title: 'External defaults: kb pages and Bash# skill/scripts from the release source pin'
closed: 2026-11-07T22:23:47.041124Z
closed_by: claude-opus5.5-s588
---

yoke pkg/kb: when ${BASHY_HOME:-$HOME/.bashy}/src/bashy/kb/defaults exists (the release source pin fetched by bashy src fetch bashy), search/recall/context also read it as a read-only shared ring (assetring.RingShared semantics) after repo/host/agent; a page with the same slug in a writable ring wins; add/update/supersede never write there. Red/green tests with a temp BASHY_HOME. KISS/MVP: smallest change, reuse existing mechanisms (assetring shared ring, bashy src pin, bashy commands add). Public content uses generic names and variables for host/user/home; no operator details. Delivery commits carry Sprint, Story and Story-ID trailers.
