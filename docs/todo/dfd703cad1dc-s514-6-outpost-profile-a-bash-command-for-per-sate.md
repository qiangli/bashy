---
id: dfd703cad1dc
kind: feature
title: 'S514.6 outpost-profile: a Bash# command for per-satellite outpost profiles (add/use/list) around register --out and outpost restart'
seq: 2727
status: todo
priority: p0
labels:
    - outpost
    - pairing
created: 2026-10-27T02:29:36.888865Z
sprint: 514
sprint_id: fadd1a46-0368-50f1-a65d-4ed147b17629
sprint_title: 'invitation email + scoped satellites (KISS/MVP: scope + expiry on the one-time pairing code; no outpost change)'
---

Spec: docs/sprint-514-scoped-pairing-multihoming.md (umbrella), plan step 5. No outpost code change. A Bash# file (skill + script) registered with bashy commands: 'outpost-profile add <name> --code <code> --server <url>' runs outpost register --out ~/.config/matrix/profiles/<name>/agent.json; 'outpost-profile use <name>' copies the profile into the active ~/.config/matrix/agent.json (backing up the current one into its own profile) and runs outpost restart; 'outpost-profile list' shows profiles and which is active. Works on macOS, Linux and Windows paths. Red/green: two fake profiles, use swaps the active file and calls restart; list marks the active one.

Constraints (s514-manager): NO podman/docker/containers on the dev box. Never run `outpost register` or `outpost restart` against the live outpost on the dev box from tests: use a temp HOME/XDG_CONFIG_HOME and a fake outpost binary in tests. No hostnames in code/docs. Gate: focused tests for the command + `go build ./...`.
