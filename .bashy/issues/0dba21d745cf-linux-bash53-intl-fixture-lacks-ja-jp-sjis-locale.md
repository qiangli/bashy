---
id: 0dba21d745cf
kind: bug
title: Linux bash53 intl fixture lacks ja_JP.SJIS locale
status: closed
stage: test
priority: p0
reporter: qiangli
created: 2026-07-13T18:09:37.252563Z
weave: 21
closed: 2026-07-13T18:35:47.371776Z
resolution: fixed
closed_by: qiangli
---

Live GitHub Actions run 29267773644 on bashy c76cbfb: intl fails at line 24 because ubuntu does not have ja_JP.SJIS; expected Passed all 1770 Unicode tests. This is distinct from the closed fr_FR.ISO8859-1 issue. Reproduce locale generation in an Ubuntu container, make the workflow generate a selectable ja_JP.SJIS-compatible locale, and avoid shell-engine changes. Required evidence: container locale probe and the focused Linux intl fixture. Commit the CI-only fix.

## Resolution

Corrected implementation merged via weave run 22 as 5af647b; locale generation uses localedef --no-warnings=ascii and verifies the SJIS selector/charmap fail-closed.
