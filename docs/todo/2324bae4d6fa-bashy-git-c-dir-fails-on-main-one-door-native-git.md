---
id: 2324bae4d6fa
kind: bug
title: 'bashy git -C DIR fails on main: one-door native git refuses -C'
seq: 418
status: todo
priority: p0
labels:
    - git
created: 2026-10-09T04:16:45.826933Z
---

Found by the Sprint 321 conductor 2026-10-09 after env make install of bashy main 7be3316: 'bashy git -C DIR log -1' fails with "unknown shorthand flag: 'C' in -C"; the installed fe23b23 forwards it to real git and works; 'bashy git --external -C DIR ...' also works. Agent shells wrap git as a function calling 'bashy git', so every git -C in every agent breaks once main is installed. Introduced by the Sprint 252 git one-door (6c39ef39 and later). The canonical ~/.local/bin/bashy was rolled back to fe23b23. Fix: the one-door must accept -C (and other global options it does not implement natively) by delegating to real git, per the bashy-git-is-real-git rule; add a red test for git -C before the fix. Release blocker for the next install.
