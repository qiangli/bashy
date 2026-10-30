---
id: cb59c69a1ec3
kind: feature
title: S537.S1b desk tools ship shaped.json (bashy side of S1) + item URLs behind the app shell
seq: 2961
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T19:50:39.014667Z
weave: 377
assignee: claude-opus5.5
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T20:09:39.649616Z
closed_by: s537-manager
---

bashy side of S537.S1 (yoke story 887a07a1; yoke part merged as yoke 849701cd "shaped apps can declare a server (shaped.json)"). Close condition (supervisor/operator): every desk tool, incl. viewer, ships shaped.json, and W5 proves one fork/rollback.
1. Start from the S1 worker branch: `git cherry-pick 28912766` (agent/weave-issue-736-notepad-shaped in the bashy repo: notepad shaped.json, README, relative fetch) onto origin/main. Resolve against D2 verbs / D1 shell / N3 renderer that landed since.
2. Ship shaped.json (same schema as notepad's, per yoke pkg/fleet/shaped + pkg/shape/appserver at 849701cd) for chart, form (server), viewer, whiteboard, brainstorm — each tool becomes shapable: the console builds + runs the principal's revision.
3. Known gap from S1: notepad item URLs print the internal loopback, not the /notepad/ path when served behind the app shell. Fix it for every tool (relative URLs / the shell's base path), red/green test.
4. Bump bashy's yoke pin to 849701cd (`bashy mod sync --only yoke` or go.mod edit), only go.mod/go.sum.
Red/green: a test per tool that its shaped.json parses with yoke's shaped loader and names a buildable server; gofmt + GOWORK=off go vet + go test in viewer, kit, notepad, form/server, chart, whiteboard/server, brainstorm/server; root `go build ./...` for the pin bump.
