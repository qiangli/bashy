---
id: e1b0b5ae8d15
kind: bug
title: 'Headless/automation Chrome launches hit the macOS keychain: every launch pops ''Chrome Safe Storage'' at the operator (dialog storm from tests)'
seq: 3274
status: assigned
priority: p1
created: 2026-11-20T23:38:23.28431Z
assignee: agy-gemini4-carbon
sprint: 682
sprint_id: 623586bb-742f-570e-87c9-9af0c101bac1
sprint_title: Shaping-enable the example apps for consumers (viewer, session, form, board/whiteboard/table/hello-world, notepad/chart)
---

2026-11-20: the conductor ran the desk example tests; each headless Chrome launch (PDF export) made macOS prompt for the Chrome Safe Storage key, repeatedly. Launch sites without keychain isolation: bashy/examples/desk/viewer/pdf.go, bashy/examples/desk/notepad/export.go, yoke/pkg/browser/solo/service.go, yoke/pkg/browser/probe/launch.go (check video.sh too). Fix: every automation/headless launch passes --use-mock-keychain (macOS) and --password-store=basic (Linux) and uses a throwaway --user-data-dir; never the operator's real profile. Policy (operator 2026-11-20): an automated path must never trigger an OS credential prompt; if one is genuinely needed it waits for a recorded human/approver decision once, never retries in a loop. Tests assert the flags on every launcher. Root cause first (confirm the prompt disappears with the flag on this macOS); verification of browser tests runs on the gate host, not the dev box.
