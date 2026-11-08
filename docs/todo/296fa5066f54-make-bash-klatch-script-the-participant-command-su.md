---
id: 296fa5066f54
kind: feature
title: Make Bash# Klatch script the participant command surface
seq: 3197
status: done
priority: p1
created: 2026-11-08T17:43:16.904557Z
weave: 54
assignee: claude-sonnet5.5
sprint: 597
sprint_id: ce9248d4-2579-5c97-857e-de17d182395f
sprint_title: Klatch participant CLI MVP
closed: 2026-11-08T18:55:18.739073Z
closed_by: codex-gpt6-sol-klatch
---

Extend bashy/examples/klatch/klatch.bsh as PRIMARY participant command surface for software shaping. Put command routing and composable workflows in Bash#, delegating only protocol/crypto/files to the small Klatch Node helper. Preserve setup/provision/status; update registration, skill metadata, help and docs with shaping entrypoint. Expose auth/MFA/seal, contacts invite/accept, conversations and members, messages, attachments, foreground watch. JSON/stdin/file/session contract, no secrets/text in argv; assistant text [Assistant: NAME]. Node explicit MVP dependency. Coordinate exact helper contract with client owner. ALL builds/tests on the claimed shared test host through the established outpost SSH route; development host editing/coordination only.
