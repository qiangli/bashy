---
id: 51c7c587d8b8
kind: task
title: install.sh/install.ps1 default to --service, docs say dormant
seq: 3186
status: todo
created: 2026-11-08T01:46:28.149701Z
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
---

dhnt.io/install.sh sets service=1 and passes --service to bashy self install; self install --help and docs/setup.md say the default install leaves outpost dormant. Pick one: default --no-service (matches self install) or document the service start. Found by cold-start agent reading install.sh.
