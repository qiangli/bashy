---
id: 8794110d6c74
kind: bug
title: Correct SJIS locale generation to remain fail-closed
status: triaged
stage: test
priority: p0
reporter: qiangli
created: 2026-07-13T18:20:26.191102Z
---

Corrective review of bashy weave run 21 commit 594234d. Import that commit from /Users/qiangli/.bashy/weave/bashy-6497d06f/workspaces/issue-21, then replace localedef --force because it exits 1 on the expected ASCII warning under workflow set -e. Container evidence proves localedef --no-warnings=ascii -i ja_JP -f SHIFT_JIS ja_JP.SJIS exits zero, locale -a includes ja_JP.sjis, and LC_ALL=ja_JP.SJIS locale charmap returns SHIFT_JIS. Keep the change CI-only, validate workflow YAML, commit the corrected series.
