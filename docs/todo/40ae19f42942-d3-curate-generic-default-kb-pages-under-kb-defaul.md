---
id: 40ae19f42942
kind: doc
title: D3 Curate generic default kb pages under kb/defaults in the bashy repo
seq: 3180
status: done
priority: p1
labels:
    - kb
created: 2026-11-07T22:07:52.754509Z
assignee: claude-opus5.5-s588
sprint: 588
sprint_id: 2ab4b70e-c274-5779-9e55-8259c38a4204
sprint_title: 'External defaults: kb pages and Bash# skill/scripts from the release source pin'
closed: 2026-11-07T22:23:47.705827Z
closed_by: claude-opus5.5-s588
---

Copy the generic public runbooks/facts (outpost setup, multi-satellite outpost-profile, sharing model, Bash# example registration, Windows remote ops, one podman machine, cloud access, AWS satellite provisioning; the DigitalOcean satellite page only if marked unverified) into kb/defaults as a kb store (pages + index), separate from the repo's working docs/kb. Scrub check: no hosts, users, teams, secret names, IPs, endpoints; variables for host/user/home. KISS/MVP: smallest change, reuse existing mechanisms (assetring shared ring, bashy src pin, bashy commands add). Public content uses generic names and variables for host/user/home; no operator details. Delivery commits carry Sprint, Story and Story-ID trailers.
