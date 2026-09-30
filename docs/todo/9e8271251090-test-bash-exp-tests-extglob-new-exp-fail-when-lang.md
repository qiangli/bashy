---
id: 9e8271251090
kind: bug
title: 'test-bash: exp-tests, extglob, new-exp fail when LANG is unset (pass with LANG=C.UTF-8) - check bashy vs GNU bash in the C/POSIX locale'
seq: 357
status: todo
priority: p1
labels:
    - posix
created: 2026-09-30T11:47:40.608648Z
---

Found 2026-09-30 by the Sprint 110 conductor on the linux/amd64 runner (bashy 4b1cb4b, sh 4324d9ba). make test-bash under a systemd unit with no LANG: 83/86, fails exp-tests (want 'hello' got '[hello'), extglob (want 'a,b' got 'a , b'), new-exp (want 'argv[1] = <yyy>' got '<yyyyyy>'); with LANG=C.UTF-8 all three pass. CI always sets a UTF-8 locale, so this was invisible. Next: run the same fixture lines under GNU bash 5.3 with LANG unset; if GNU bash matches the .right files there, bashy diverges in the C/POSIX locale, which is the locale the VSC/TET run uses.
