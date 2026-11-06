---
id: 5bd1cd63cebf
kind: bug
title: Restore fixture terminal foreground ownership in Bash53 runner
seq: 3096
status: done
priority: p1
created: 2026-11-05T11:10:00.10735Z
weave: 20
assignee: s562-tty-runner
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
closed: 2026-11-05T11:55:34.984172Z
closed_by: codex-gpt6-sol
---

The authoritative Bash53 runner allocates each guarded fixture a separate process group while Makefile routes headless runs through a controlling PTY. Native GNU read2.sub stops in background terminal read; this invalidates the baseline. Implement bounded foreground handoff and restoration on success, timeout and cancellation, preserving group containment, idle stdin and no-TTY behavior. Preserve original fixture bytes, expected outputs and deadlines. Include focused native read plus ownership/restoration/cancellation/no-TTY controls. Parent executes remote gates; no broad local suites. Delivery requires DCO and Sprint/Story/Story-ID trailers.

Execution contract: use original Sprint 562 / Story-ID 5bd1cd63cebf only. Base is a4f5e7bb1385a7c2eafe69d3a716f49b1738c937; verify ancestry before editing. Do not create or reassign another sprint/story. If the isolated clone lacks this newly created card, read the canonical card or the coordinator-provided non-todo handoff; never duplicate its todo-store entry. Preserve any accidental unrelated story as unaccepted.

REMOTE ONLY: do not run any local builds, go tests, make test-bash, bashy gate, or acceptance suites. Parent executes every build and test on the authorized remote host. Prepare concrete resolved remote commands and source/provider/input hashes; a local script alone is not delivery. Ask the coordinator for private route details, which must not enter public files.

Required controls: native GNU unchanged read fixture; exact foreground process-group ownership; restoration after success, timeout and cancellation; no-TTY behavior unchanged; reject unrelated terminal ownership and avoid unrelated process signals; preserve parent-death/group cleanup. Review the design before broad changes. Native terminal-stop diagnosis does not yet explain the product read2 loop stall; keep those observations distinct. Preserve original fixture bytes, expected outputs and original deadlines. Delivery includes DCO plus original Sprint/Story/Story-ID trailers. Do not publish canonical changes.
