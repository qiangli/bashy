---
id: f9e0a8bd660a
kind: bug
title: command -v / type cannot see bashy's in-process applets, so every 'has=jq' requires probe fails on native Windows
seq: 3234
status: todo
priority: p2
labels:
    - windows
    - media
created: 2026-11-13T21:18:15.13554Z
---

Found in Sprint 634's live run on the Windows test host: oss_doctor (examples/oss-rods/oss-preflight.bsh) probes each 'has=NAME' with command -v; bashy runs jq in-process (printf '{}' | jq . works) but command -v jq and type -t jq return 1 when no jq binary is on PATH, so translate-ollama's doctor reported has=jq missing and the door refused the rod (exit 126). Dev-box repro: PATH=/bin bashy -c 'command -v jq; type -t jq' -> 1, 1. Sprint 634 dropped has=jq from translate-ollama only; song-suno, image-chatgpt, animate-kling, song-/image-/animate-replicate still declare has=jq and will fail the same way on a host without a jq binary. Fix where the cause lives: either command -v/type report applets (Classic semantics question) or the requires:/oss_doctor probes skip tools bashy ships (define/commands show knows them). Root cause recorded; no fix in this sprint.
