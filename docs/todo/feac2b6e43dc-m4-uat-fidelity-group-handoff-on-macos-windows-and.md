---
id: feac2b6e43dc
kind: test
title: M4 UAT + fidelity + group handoff on macOS, Windows and Linux
seq: 3215
status: done
priority: p2
labels:
    - media
    - uat
created: 2026-11-11T02:40:05.207573Z
assignee: claude-opus5.5-s603
sprint: 603
sprint_id: c808e55d-6ce2-5525-8829-96499861c90e
sprint_title: 'Agent-assisted PDF and video: small verbs, a media skill-script and kb pages any agent can follow'
closed: 2026-11-11T11:29:36.063694Z
closed_by: claude-opus5.5-s603
---

Spec: dhnt-docs docs/sprint-603-master-execution-plan.md (M4). Supersedes dhnt-docs story 2f571ccf8fad (B5). Depends on M1-M3.

UAT on a macOS host, a Windows host and a Linux desktop (claim each host): a fresh agent session per OS, kb pages only, makes a 2-page PDF and a 60-second own-voice narrated video, viewed and shared. Fidelity: one sample terminal scene (colours, a progress line, a bashy command) recorded with `cast record` on all three OSes and with asciinema on macOS, rendered by agg with the pinned font, compared frame by frame. Group handoff: a project started on Windows, extended on macOS, built on Linux with no edits; rebuilding on each OS gives the same duration and segment order. Evidence in dhnt-docs docs/sprint-603-evidence.md.
