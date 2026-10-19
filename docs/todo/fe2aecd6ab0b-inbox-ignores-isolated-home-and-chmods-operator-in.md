---
id: fe2aecd6ab0b
kind: bug
title: Inbox ignores isolated HOME and chmods operator inbox directory
seq: 1939
status: done
priority: p2
labels:
    - inbox
created: 2026-10-18T21:13:49.837141Z
closed: 2026-10-19T03:24:53.937632Z
---

Repro from Sprint 446 S10 real-binary zero-model canary (agent-bench run #8, commit 5dfeaa6): launch the current Bashy binary with HOME and BASHY_HOME both pointing to a fresh temp arm directory, under a macOS sandbox-exec profile that denies file-write* within the operator's real HOME. Invoke `bashy inbox --as <test-agent> --peek`. The sandbox reports `deny file-write-mode $HOME/.bashy/inbox`: inbox attempts to chmod the real operator inbox directory despite the isolated HOME/BASHY_HOME. The DAG benchmark sets BASHY_CHAT_INBOX=off and does not invoke inbox on its live arm path, so this is a separate Bashy product bug. Fix the inbox path selection to respect the isolated state root and add a regression test that fails on a real-home chmod attempt. Do not change Sprint 446 S10 implementation to hide this failure.
