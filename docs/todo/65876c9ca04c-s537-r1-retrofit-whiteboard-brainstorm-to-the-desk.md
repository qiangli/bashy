---
id: 65876c9ca04c
kind: feature
title: S537.R1 retrofit whiteboard + brainstorm to the desk contract and UI kit (no user-visible behaviour change)
seq: 2933
status: todo
priority: p2
labels:
    - desk
created: 2026-10-30T02:45:42.993782Z
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
---

Retrofit bashy/examples/whiteboard and bashy/examples/brainstorm (Sprint 513 apps) to the desk contract and UI kit WITHOUT user-visible behaviour change. Add: desk CLI verbs mapped onto existing ops where they exist (keep existing serve/status/backup/stop/test verbs working), stable id + URL reopen (`open <id>` prints id/URL/status only), the kit's copy button/status badges/item header where they replace equivalent markup, the shared contract test (desk-kit/contracttest) run against each server, launcher tile registration (`bashy app add`) in register.sh. Recorded exception: they KEEP their LAN default bind (do not switch to loopback) — configure the contract test accordingly (kit README / contracttest config; if the contracttest cannot express the LAN exception, add a minimal opt-in field and say so in the submit note). Must stay green: each app's existing Go tests and duality check (verify-duality.py) on Dragon; browser-test.mjs runs later on a test host (do not run browsers here). Use require desk-kit + replace => ../desk/kit in each server module.

Common: desk contract + kit at examples/desk/kit (read examples/desk/kit/README.md); keep contracttest green for the tool; focused red/green Go tests; gofmt clean (pre-push fmtcheck rejects otherwise); commit only your story files (do not commit KB.md or docs/todo); end with `bashy sprint submit 537 <story-id> --as <your binding> -m "<evidence>"`. Dragon: focused `go test` only, no podman. Spec (read-only): $HOME/projects/poc/dhnt/docs/sprint-537-bashy-desk-plan.md
