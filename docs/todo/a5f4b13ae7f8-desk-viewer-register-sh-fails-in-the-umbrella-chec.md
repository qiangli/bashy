---
id: a5f4b13ae7f8
kind: task
title: desk viewer register.sh fails in the umbrella checkout without GOWORK=off
seq: 3191
status: todo
created: 2026-11-08T02:13:17.291374Z
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
---

examples/desk/viewer/register.sh runs 'go run ./cmd/atlas' in ../kit without GOWORK=off; inside the umbrella the root go.work wins and it fails: 'main module (github.com/qiangli/bashy) does not contain package .../examples/desk/kit/cmd/atlas'. GOWORK=off ./register.sh works. Add GOWORK=off to that go run line (the build line already has it).
