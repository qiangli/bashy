---
id: ce94afa2d401
kind: bug
title: 'POSIX: declaration utility reached through ''command'' (command export / command command readonly A=$a) field-splits and globs the assignment operand'
seq: 361
status: todo
priority: p1
labels:
    - posix
created: 2026-09-30T13:40:16.495614Z
---

Found 2026-09-30 by Sprint 110 preflight (yash -p corpus ad34d458, tests backported from yash-rs 2026-09-26: declutil-p.tst 'command command export' line 58, 'command command readonly' line 66) on the frozen linux/amd64 candidate (SUT digest 0844a95e, bashy 2db80ed / sh 831b6b2d). These are the only 2 BASHY-SPECIFIC failures (bash OK, bashy FAIL) of 1833 cases. Repro (installed bashy on dragon): bashy --posix -c 'a="1  *  2"; command export B=$a; printf "[%s]\n" "$B"' -> 'export: `a.patch': not a valid identifier' ... ; GNU bash 5.3 --posix prints [1  *  2]. POSIX 2024 treats export/readonly as declaration utilities also when invoked via command. Fix belongs in sh (declaration-utility detection must see through 'command' wrappers).
