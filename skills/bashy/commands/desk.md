---
name: desk
description: Agent guidance for the bashy desk suite (notepad, form, chart, whiteboard, brainstorm, viewer). Use desk tools for human-facing documents, forms, stats, and sessions, re-surfacing by id instead of chat dumps.
metadata:
  requires: "has=bashy"
---

# bashy desk — agent-human tool suite

Rule for every agent: more than one question, a form, or a document for the human -> use a desk tool and re-surface by id (`bashy <tool> open <id>` prints id, URL, and status only) instead of re-emitting content into chat.

## Rules of thumb

- **Human edits text/Markdown** -> `notepad` (show/collect docs; convert/export pdf/png/html/txt; lens imports).
- **Human reads a report/recap/document** -> `viewer` (view-only rendered book; 2–4 line chat summary + link).
- **Agent needs answers** -> `form` (ad-hoc form from JSON/YAML; replyform from browser extract; questionnaire with `wait`).
- **Many respondents / poll** -> `form` with `--respondents many` or poll preset (`--poll ... --options ...`).
- **Quick stats / OTel metrics** -> `chart` (ready-made SVG widgets from JSON/CSV/NDJSON/OTLP-JSON; not a dashboard).
- **Team deliberation / sessions** -> `whiteboard` (drawing & notes) or `brainstorm` (private writing, reveal, voting).

## Conventions & safety

- **Reopen-by-id**: `bashy <tool> open <id>` prints id, loopback URL, and status only — saves context tokens.
- **Secrets never via desk**: `kind: secret` or secret field names (`db_password`, keys) are refused. Use `bashy ask`.
- **Loopback-only & sharing**: Binds `127.0.0.1` by default. Mint invite links for others with `bashy <tool> share <id>`.
- **Standard verbs**: `add`, `show`, `set`, `list`, `open`, `share`, `close` (+ `export`, `append`, `wait`, `reply`, `ask`).
- **Common options**: `--json`, `--fields`, `--dry-run`, `--no-input`, `--explain`, `--help=mcp`.
- **JSON envelope**: `schema_version: bashy-<tool>-v1`, error `{code, message}`, HTTP API = CLI 1:1.

## Tools & minimal commands

### notepad — view, edit, and convert documents
Human edits text. Imports via `bashy lens`. Converts and exports to PDF, PNG, HTML, TXT.
```bash
bashy notepad add --title "Notes" --body "..."       # create note (or pipe via lens)
bashy notepad open <id>                             # reopen: prints id, URL, status only
bashy notepad show <id>                             # read note content
bashy notepad set <id> --body "..."                 # update note content
bashy notepad export <id> --to pdf                  # export: pdf, png, html, txt
bashy notepad list                                  # list saved notes
bashy notepad close <id>                            # freeze note against mutations
```

### viewer — view-only rendered reports & books
Human reads a report/recap/document. Agents provide structured JSON data for shipped templates (`status`, `plan-summary`, `recap`, `close-out`) or Markdown — agents never write HTML. View-only: `add`, `show`, `list`, `open`, `share`, `close`, `append`, `export` (no `set`).
```bash
# Book from template + JSON data (templates: status, plan-summary, recap, close-out)
bashy viewer add --sprint 537 --template status --data status.json # create status book (or --session S)
# Or create from Markdown body directly; in chat, post 2-4 line summary + link
bashy viewer add --title "Report" --body "..."       # initial page
bashy viewer append <id> --template recap --data recap.json # append next page/section
bashy viewer append <id> --title "Next" --body "..." # append Markdown page
bashy viewer open <id>                               # reopen: prints id, URL, status only
bashy viewer show <id> --page 1                      # read page content + metadata
bashy viewer export <id> --to pdf                    # export to PDF (or self-contained HTML)
bashy viewer list                                    # list saved viewer books
bashy viewer close <id>                              # freeze viewer book against appends
```

### form — forms, replyforms, questionnaires, polls
Agent defines in JSON/YAML; human fills on loopback page; agent reads answers as JSON.
```bash
# Ad-hoc form
bashy form add --def form.json --title "Intake"      # define form; returns id, URL, status, ownerKey
# Multi-respondent poll preset
bashy form add --poll "Choice?" --options a,b,c      # multi-respondent voting (or --respondents many)
# Replyform from browser extract + guard refusals
bashy form reply --extract ext.json --values v.json --refusals ref.json
# Questionnaire (persistent numbered questions Q1..Qn; append followup rounds)
bashy form ask --def questions.json --title "Q&A"    # assign Q1..Qn
bashy form ask <id> --key "$KEY" --def followup.json # append round (Qn+1..)
bashy form open <id>                                 # reopen: prints id, URL, status only
bashy form wait <id>                                 # block until answered, closed, or timeout
bashy form show <id> --json                          # read answers keyed by field/question id
bashy form close <id>                                # close form against further saves
```

### chart — quick stats & OTel metrics
Ready-made SVG widgets: `counter`, `gauge`, `updowncounter`, `histogram`, `exponentialhistogram`, `line`, `bar`, `table`, `stat`.
```bash
bashy chart add --input metrics.json --widget counter # render quick stat / metric chart
bashy chart open <id>                                 # reopen: prints id, URL, status only
bashy chart show <id>                                 # read saved chart data & SVG
bashy chart set <id> --input updated.json             # replace snapshot
bashy chart list                                      # list saved charts
bashy chart close <id>                                # freeze chart
```

### whiteboard & brainstorm — team sessions
LAN team sessions with participant browser cookies and facilitator controls.
```bash
bashy whiteboard add --id main                       # shared drawing canvas & pinned notes
bashy whiteboard open main                           # reopen: prints id, URL, status only
bashy whiteboard show main                           # inspect board state
bashy whiteboard close main                          # lock board against contributions
bashy brainstorm add --id main                       # private idea cards, reveal, voting
bashy brainstorm open main                           # reopen: prints id, URL, status only
bashy brainstorm show main                           # inspect brainstorm state
bashy brainstorm close main                          # close voting and finalize
```
