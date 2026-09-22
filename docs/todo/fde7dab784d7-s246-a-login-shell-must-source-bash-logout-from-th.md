---
id: fde7dab784d7
kind: bug
title: 'S246: a login shell must source ~/.bash_logout from the shell''s own $HOME on Windows'
seq: 314
status: todo
priority: p1
labels:
    - windows
created: 2026-09-22T12:32:46.207849Z
sprint: 246
sprint_id: 4702a897-aa8e-54be-8f46-619a5c50904e
sprint_title: 'Windows fixture residuals: the POSIX permission model, the tty pair, a provisioned toolchain, and the tour markers after a release'
---

invocation.tests runs `HOME=$TDIR ${THIS_SH} --login -c 'logout'` and expects ~/.bash_logout under $TDIR to run. loadStartupFiles and runWithLoginLogout took their home from os.UserHomeDir, which reads %USERPROFILE% on Windows and ignores $HOME, so the file was never found. Fixed by shellHome() in internal/cli/main.go.
