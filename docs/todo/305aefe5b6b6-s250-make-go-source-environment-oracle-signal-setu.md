---
id: 305aefe5b6b6
kind: bug
title: S250 make Go source environment oracle signal setup independent of shell OPTIND
seq: 329
status: todo
priority: p0
labels:
    - linux
    - ci
created: 2026-09-23T20:09:33.235802Z
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

Ubuntu Bashy CI run 35908988886 fails TestGoSourceExecutableEnvironment/explicit before native oracle starts: /bin/sh rejects caller-supplied OPTIND=caller-optind with Illegal number. Preserve the exact explicit caller environment and Go example source; replace the test-only /bin/sh SIGTERM-ignore wrapper with a tiny compiled native wrapper that sets SIG_IGN and execs target without parsing or rewriting environment. Keep original 60s run limit, compare raw outputs, and verify full focused executable differential on Ubuntu and local control.
