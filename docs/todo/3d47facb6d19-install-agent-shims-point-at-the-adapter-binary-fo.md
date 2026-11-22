---
id: 3d47facb6d19
kind: bug
title: 'install-agent shims point at the adapter binary (foreman) instead of bashy when dispatched through an adapter: bash/sh/zsh shims exec the wrong program'
seq: 3300
status: assigned
priority: p0
labels:
    - bashy
    - adapter
created: 2026-11-22T13:50:33.902213Z
assignee: codex-gpt6-astra
sprint: 692
---

Seen 2026-11-22 on the dev box: 'bashy sprint instruct 692' started a managed manager session (foreman serve); at that moment ~/.bashy/shims/{bash,sh,zsh} were rewritten to 'exec <cache>/bashy/adapters/yoke/foreman "$@"'. Every '#!/usr/bin/env bash' script on the box (umbrella pre-commit scrub gate among them) then ran under foreman and failed ('bashy chat: unknown command <script>'). Cause (code read, not yet reproduced in a test): internal/agentos/installagent.go, 'shell := *shellPath; if shell == "" { exe, _ := os.Executable(); shell = exe }' - when install-agent runs inside an adapter process (foreman, or any BASHY_ADAPTER_INPROC path) os.Executable() is the adapter binary, and shimInstaller writes it into the shims via chat.WriteShellShim. Fix direction: resolve the shell from the bashy launcher identity (BASHY_SELF / the resolved bashy path used by runCommandAdapter), never os.Executable() when running as an adapter; refuse to write a shim whose target is under the adapter cache; unit test: install-agent agy with BASHY_SELF set and os.Executable() pointing elsewhere writes shims that exec BASHY_SELF. Owner codex, same run as 42f79daa/3dd8b4ec (adapter first-use set). Venue repair already done by hand on the dev box (shims rewritten to the installed bashy). TEST RULE (operator 2026-11-22): build + smallest targeted unit tests only, then submit; the one full run is the rc.40 candidate gate 458628c0.
