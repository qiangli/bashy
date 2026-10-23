---
id: 44ee7d2a5b10
kind: feature
title: 'M1b (bashy) mount bashy src + class axis: verb wiring, featured rename, commands --view class'
seq: 1941
status: done
priority: p0
created: 2026-10-23T16:17:55.367113Z
weave: 272
assignee: codex-gpt6-sol
sprint: 458
sprint_id: 3b16e60f-dce3-5dd1-8ed1-4384110d0741
sprint_title: 'bashy is all you need, MVP: invariant core + every other command as editable Bash# skill/script'
closed: 2026-10-23T17:17:53.002538Z
resolution: fixed
closed_by: codex-gpt6-sol-l
---

Split from M1 (yoke story 0e66bb40; umbrella integration b9b3580c), because a weave run covers one repo only. Starts after M1's yoke commit is merged and published. Repo: bashy only. Public repo: no hostnames, user ids or cloudbox internals.

Deliver:
1. `go get github.com/qiangli/yoke@bdede1a9` (merged M1, pushed to yoke main), then make `go build ./...` green.
2. Mount `bashy src` (show, fetch, check) as a bashy verb: agentos dispatch case, verb list, atlas entry, commands description and cobra schema. It is a core verb with experimental stability until M3.
3. `bashy commands --view class` and the class field in `bashy commands <cmd>` and `--json`.
4. Rename the presentation list coreRows "1.0.0 core" to "featured" in bashy/internal/agentos/commands_sections.go, including help text. Version the JSON key `core` -> `featured` if a consumer reads it.

Acceptance: `bashy src --help` works from a freshly built binary; `bashy commands --view class` lists 14 core verbs plus builtins, coreutils and posix-providers as core; focused tests for agentos commands, sections and cobra schema; `go build ./...`. Spec: umbrella docs/bashy-invariant-core.md.
