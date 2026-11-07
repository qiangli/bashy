---
id: a009204fb183
kind: spike
title: 'S1 Spike: run a second outpost instance per satellite next to the main one'
seq: 3163
status: todo
priority: p0
labels:
    - outpost
created: 2026-11-07T08:41:45.677628Z
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: Satellite-less single-node outpost by default; join and switch between satellites
---

Run a second outpost with its own XDG_CONFIG_HOME/XDG_CACHE_HOME and OUTPOST_ADMIN_ADDR next to the host's main outpost, first unpaired and then paired with a second satellite. Publish Klatch through it as an app that points at the same local backend with the same SSO secret. List every collision (LAN sshd 2222, seeded service units, discovery, mesh, ollama, upgrade puller, OS service) and whether an existing config toggle turns it off. Exit: a written list of findings with the config that avoids each collision, or the smallest outpost change needed (S2b). Runs from a scratch directory on the dev box; never touches the main outpost's config or service. KISS/MVP: reuse existing code and harnesses; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
