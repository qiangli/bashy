---
id: 941f35b6524f
kind: bug
title: 'exec PROGRAM does not replace the shell process: the program runs as a child and survives a signal sent to the shell'
seq: 3325
status: todo
priority: p1
labels:
    - exec
created: 2026-11-26T16:55:07.142851Z
---

Found 2026-11-26 in umbrella Sprint 731 (story b390ac728272) while defining how a shapeware service is stopped.

Reproduction (Linux gate lane, bashy built at e95b09a18): a script with the one line 'exec /usr/bin/python3 -c "import time;time.sleep(303)"'. Run it in the background with the system sh: the process with the shell's pid IS python3, it has no children, and TERM to that pid ends it. Run it with bashy (plain, and with --bashsharp): the process with that pid is still bashy, python3 is its child, and after TERM to that pid the shell exits while python3 keeps running, re-parented. With a coreutils program (sleep) nothing is visible because it runs in-process.

Expected (Bash 5.3): exec replaces the shell with the program (same pid), so a supervisor that signals the pid it started stops the program.

Effect seen: bashy examples/desk/form 'form serve' ends with 'exec "$bin" serve'; a TERM to the launcher ended the launcher with status 143 and left the form server answering. Any service started through a bashy script with exec cannot be stopped by signalling the pid that was started (systemd, a container runtime, a test harness). Sprint 731 worked around it in the spec, not in the shell: the form contract defines 'form stop' and leaves signals an implementation freedom.

Not fixed here: the shell interpreter is on the frozen baseline. Needs its own repair with a regression test (pid identity after exec; TERM reaches the program), on every OS (on Windows there is no process replacement; forwarding signals and the exit status is the equivalent).
