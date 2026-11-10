---
id: 1b05b83d9c33
kind: bug
title: test-bash 79/86 in the container lane on the Windows test host WSL
seq: 3206
status: done
priority: p3
created: 2026-11-10T03:48:03.1064Z
closed: 2026-11-10T18:08:50.718656Z
---

Pre-existing on that venue: scripts/test-self-container.sh BASHY_TEST_TARGET=test-bash gives the same 7 failures on bashy 51137ad0 and 95057720 (execscript, glob-test, intl, new-exp, printf, redir, test). Likely venue (locale/fixture) differences; the release gate venue is 86/86. Not repaired in S602.

2026-11-10 resolution (manager): NOT a product defect and not "locale/fixture" alone. The 79/86 came from running `make test-bash` through scripts/test-self-container.sh as ROOT in a stock image without hexdump/locales; the 7 (execscript, glob-test, intl, new-exp, printf, redir, test) are environmental and disjoint from the historical dialect-on 79+7 (comsub, comsub2, errors, func, trap, type, varenv). The authoritative gate on the same host and bashy a9958c91: `make test-bash-container` 86/86 and `make test-bash-container-bashpp` 86/86. Guard added: `make test-bash` refuses to run as root (BASH53_ALLOW_ROOT=1 overrides) and points to the sealed gate.
