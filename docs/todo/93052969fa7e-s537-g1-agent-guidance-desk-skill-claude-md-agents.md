---
id: 93052969fa7e
kind: doc
title: 'S537.G1 agent guidance: desk skill + CLAUDE.md/AGENTS.md line + bashy app launcher tiles for all desk tools'
seq: 2934
status: todo
priority: p2
labels:
    - desk
created: 2026-10-30T02:45:43.291995Z
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
---

Agent guidance for the bashy desk suite. Rule for every agent: more than one question, a form, or a document for the human -> use a desk tool and re-surface by id (`bashy <tool> open <id>` prints id/URL/status only) instead of re-emitting content into chat.

Deliver in bashy:
1) A managed skill `skills/bashy/commands/desk.md` (follow the frontmatter/format of the sibling files there, e.g. inbox.md, transcribe.md): when to use which tool — notepad (show/collect a document; convert/export pdf/png/html/txt), form (ad-hoc form from JSON/YAML; `form reply` replyform from a browser extract + guard refusals; `form ask` numbered questionnaire with append rounds and `form wait`), chart (quick stats from JSON/CSV/NDJSON/OTLP-JSON with premade OTel widgets; not a dashboard), whiteboard/brainstorm (team sessions). Show the minimal command per case, the token-saving reopen-by-id rule, secrets never via desk (use `bashy ask`), loopback-only + `share` for others. Read each tool's README under examples/desk/<tool>/ and examples/{whiteboard,brainstorm}/ for exact verbs; do not invent flags — run each tool's --help to confirm. Keep it short (<120 lines).
2) It must be discoverable: `bashy skill show desk` works and the managed-skills test passes (skills/managed_skills_test.go; go test ./skills/...).
3) Launcher tiles: each desk tool's register.sh registers its tile with `bashy app add` under a common "desk" group if the app record supports a group/category field (check `bashy app add --help`); if not, keep names desk-prefixed-free and just ensure every tool's register.sh adds its tile. Do not change tool behaviour.
Red/green: a focused test (or the existing managed-skills test extended) that fails before desk.md exists and passes after. Do NOT edit the umbrella CLAUDE.md (the manager adds that line). Commit only skills/ and examples/desk/*/register.sh; gofmt clean; finish with `bashy sprint submit 537 93052969fa7e --as <your binding> -m "<evidence>"`. Dragon: focused go test only.
