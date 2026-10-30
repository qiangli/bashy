---
id: 6c7e91a55969
kind: feature
title: 'S537.D2 desk CLI/API conformance: standard subverbs, common options, schema_version, HTTP = CLI, notepad imports via lens'
seq: 2965
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T18:44:08.475158Z
weave: 365
assignee: codex-gpt6-sol
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T19:18:04.593039Z
closed_by: s537-manager
---

Operator (verbatim): "we have run a few sessions of ensuring the bashy command line flags/options/args are consistent, the same for ui rendering too ... the api should be consistent like command lines cross all the tools". Authority: docs/common-options.md (S446).
Gaps on bashy main: desk verbs new/get/put vs standard subverbs list/show/add/set/rm/edit; no schema_version in desk JSON; only --json of the common options evidenced; notepad bundles its own pdf/html converters, duplicating bashy lens.
Fix list:
1. Rename to standard subverbs, no aliases (alpha): new->add, get->show, put->set; domain verbs only where none exists (open, share, close, export, append, wait).
2. All common options (--json --fields --no-input --explain --dry-run --help=mcp); schema_version bashy-<tool>-v1 on every JSON output; exit 2 for unsupported.
3. HTTP API = CLI 1:1 (same verbs, fields, --fields projection, envelope, error shape); atlas generated from that one schema.
4. notepad imports via bashy lens; missing directions become lens rows, not notepad code.
5. contracttest conformance check for all of the above.
Applies to every desk tool (notepad, form/replyform/questionnaire, chart, whiteboard/brainstorm desk verbs) and the viewer brief. Lands before G1 docs.
Acceptance: contracttest red on a planted non-standard verb / missing schema_version / missing common option; green on all tools; focused per-tool tests green.
