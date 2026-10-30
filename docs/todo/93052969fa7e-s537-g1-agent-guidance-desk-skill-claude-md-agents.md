---
id: 93052969fa7e
kind: doc
title: 'S537.G1 agent guidance: desk skill + CLAUDE.md/AGENTS.md line + bashy app launcher tiles for all desk tools'
seq: 2934
status: done
priority: p2
labels:
    - desk
created: 2026-10-30T02:45:43.291995Z
weave: 375
assignee: agy-gemini3.8-flash
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T20:00:14.480887Z
closed_by: s537-manager
---

Agent guidance for the bashy desk suite. Rule for every agent: more than one question, a form, or a document for the human -> use a desk tool and re-surface by id (`bashy <tool> open <id>` prints id/URL/status only) instead of re-emitting content into chat.

Deliver in bashy:
1) A managed skill `skills/bashy/commands/desk.md` (follow the frontmatter/format of the sibling files there, e.g. inbox.md, transcribe.md): when to use which tool — notepad (show/collect a document; convert/export pdf/png/html/txt), form (ad-hoc form from JSON/YAML; `form reply` replyform from a browser extract + guard refusals; `form ask` numbered questionnaire with append rounds and `form wait`), chart (quick stats from JSON/CSV/NDJSON/OTLP-JSON with premade OTel widgets; not a dashboard), whiteboard/brainstorm (team sessions). Show the minimal command per case, the token-saving reopen-by-id rule, secrets never via desk (use `bashy ask`), loopback-only + `share` for others. Read each tool's README under examples/desk/<tool>/ and examples/{whiteboard,brainstorm}/ for exact verbs; do not invent flags — run each tool's --help to confirm. Keep it short (<120 lines).
2) It must be discoverable: `bashy skill show desk` works and the managed-skills test passes (skills/managed_skills_test.go; go test ./skills/...).
3) Launcher tiles: each desk tool's register.sh registers its tile with `bashy app add` under a common "desk" group if the app record supports a group/category field (check `bashy app add --help`); if not, keep names desk-prefixed-free and just ensure every tool's register.sh adds its tile. Do not change tool behaviour.
Red/green: a focused test (or the existing managed-skills test extended) that fails before desk.md exists and passes after. Do NOT edit the umbrella CLAUDE.md (the manager adds that line). Commit only skills/ and examples/desk/*/register.sh; gofmt clean; finish with `bashy sprint submit 537 93052969fa7e --as <your binding> -m "<evidence>"`. Dragon: focused go test only.

SCOPE UPDATE (manager, 13:05 PDT). Base: bashy main cbb73844 or later. Document what is ON MAIN, in bashy's existing conventions (skills, atlas/help entries, `bashy app` launcher tiles) — no new top-level concepts:
- Verbs are D2's standard subverbs: add/show/set/list/rm (+ domain verbs open/share/close/export/append/wait); every common option; JSON envelope schema_version bashy-<tool>-v1; error {code,message}; HTTP API = CLI 1:1.
- Tools: notepad (imports via bashy lens), form (ad-hoc, replyform, questionnaire, F4 multi-respondent: respondents one|many, poll preset `form add --poll ... --options ...`), chart, whiteboard, brainstorm, and viewer (V1, run 370 in flight — document from its brief; the manager re-checks the viewer section against V1's merged code).
- Rule of thumb: human edits text -> notepad; human reads a report/recap/document -> viewer, with a 2-4 line chat summary + the viewer link; agent needs answers -> form; many respondents/poll -> form respondents=many.
- Launcher tiles for every desk tool (form has none yet), same icons/accents as the kit (D1).
Red/green: the skill/help text names only verbs the CLIs accept (a test that runs each documented verb with --help=mcp or --dry-run and fails on an unknown verb).

=== REWORK (manager, 13:25 PDT) — THIS RUN STARTS HERE ===
Prior work preserved at refs/salvage/abandoned-374 (commit 1e6e35ea: skills/bashy/commands/desk.md, managed_skills_test.go, form launcher tile). Step 1: `git cherry-pick 1e6e35ea` onto origin/main (now has V1 viewer dcc748e4).
Fix: desk.md shows flags the CLIs do not have. Every example command must use the real flags from each tool's `--help=mcp` / cli.go on main. Known wrong: `-f draft.md`, `-f report.md`, `-f appendix.md`, `-o doc.pdf` (notepad uses --body/--title/--to/--format; viewer uses --title/--body or --template + --data, plus --sprint for the status template); check form (--def?), chart (--input/--widget), whiteboard/brainstorm add arguments too. Viewer section: describe the template+JSON path (templates status, plan-summary, recap, close-out) — agents never write HTML.
Test: extend TestDeskSkillDocumentedVerbsValid (or add one) so it also fails on any documented flag the tool's flag set does not define (red on a planted `-f`), green on the fixed doc.
Gate: go test ./skills/... and the desk modules you touch.
