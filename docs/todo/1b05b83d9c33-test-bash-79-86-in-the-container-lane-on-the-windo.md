---
id: 1b05b83d9c33
kind: bug
title: test-bash 79/86 in the container lane on the Windows test host WSL
seq: 3206
status: todo
priority: p3
created: 2026-11-10T03:48:03.1064Z
---

Pre-existing on that venue: scripts/test-self-container.sh BASHY_TEST_TARGET=test-bash gives the same 7 failures on bashy 51137ad0 and 95057720 (execscript, glob-test, intl, new-exp, printf, redir, test). Likely venue (locale/fixture) differences; the release gate venue is 86/86. Not repaired in S602.
