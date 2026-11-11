---
id: 5c1def056c71
kind: bug
title: 'Windows: bashy.exe does not start when launched from a Task Scheduler process (hangs before any output; outpost apps/service monitor degraded)'
seq: 3223
status: todo
priority: p1
labels:
    - windows
created: 2026-11-11T10:39:59.380413Z
---

Found 2026-11-11 on the Windows test host (bashy rc.23 at AppData\Local\outpost, also the .local\bin copy) during the S605 tray smoke. A GUI probe started via 'schtasks /run' (interactive, /IT) ran 'bashy --version' and 'bashy app serve': no output and no listener within 15-60 s, with every creation-flag variant (none, HideWindow, CREATE_NO_WINDOW, CREATE_NEW_CONSOLE); CREATE_BREAKAWAY_FROM_JOB is refused (Access is denied) inside that task's job. The same commands over ssh return immediately. outpost's supervisor (itself a Task Scheduler S4U task) shows apps/meet/files/... 'degraded: service monitor unreachable' (the bashy service monitor never comes up), consistent with the same cause. PowerShell scripts launched the same way also hung, so the trigger may be console-host/job related rather than bashy-specific. NOT root-caused: next steps: capture a stack (procdump/handle) of a hung bashy.exe, check stdin/console-handle waits and job-object assignment in bashy's procguard, and compare with an Explorer-launched parent.
