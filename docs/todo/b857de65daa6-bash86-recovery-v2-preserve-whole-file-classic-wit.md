---
id: b857de65daa6
kind: task
title: 'Bash86 recovery v2: preserve whole-file Classic without breaking live -c'
seq: 197
status: done
priority: p0
created: 2026-09-03T12:59:30.7745Z
weave: 12
assignee: qiangli
sprint: 98
closed: 2026-09-03T14:11:07.192346Z
---

Start from accepted bashy main 548aad8 and accepted sh ef4ef743; do not use rejected run10 commit. Restore exact hermetic GNU Bash 5.3 suite to 86/86 for OFF and a top-level-only ON harness while preserving mandated refusal of inherited BASHY_BASHPP under explicit --posix. Fix persistent arith,array,new-exp,posixexp and previously recovered 21. Preserve -c,file,stdin,forced-interactive,regular-readline live set -o/+o transitions. No bytes.Contains/source-text sniff, fixture weakening, skip, cap, hidden failure swap, or unpublished sibling pin. Add production-mechanism tests, full cli/race, exact trailers; commit and submit.
