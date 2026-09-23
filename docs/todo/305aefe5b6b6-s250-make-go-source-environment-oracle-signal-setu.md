---
id: 305aefe5b6b6
kind: bug
title: S250 make Go source environment oracle signal setup independent of shell OPTIND
seq: 329
status: done
priority: p0
labels:
    - linux
    - ci
created: 2026-09-23T20:09:33.235802Z
assignee: codex-s250
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-23T20:52:57.984246Z
closed_by: codex-s250
---

Ubuntu Bashy CI run 35908988886 fails TestGoSourceExecutableEnvironment/explicit before native oracle starts: /bin/sh rejects caller-supplied OPTIND=caller-optind with Illegal number. Preserve the exact explicit caller environment and Go example source; replace the test-only /bin/sh SIGTERM-ignore wrapper with a tiny compiled native wrapper that sets SIG_IGN and execs target without parsing or rewriting environment. Keep original 60s run limit, compare raw outputs, and verify full focused executable differential on Ubuntu and local control.

Final acceptance, 2026-09-23: Bashy `f7fc6be5f667c53b07f7c1a18ad78ea0b99e68d6` uses a test-only native signal-ignore exec wrapper; it preserves the caller's `OPTIND=caller-optind` and all other environment entries before native oracle and Bashy startup. The unchanged executable differential passed locally on macOS (23.834s) and on Ubuntu 24.04 with the final sh/Bashy product siblings (66.193s). Hosted workflow `35914770533` Ubuntu job `107363346594` completed success. The later Bashy product candidate `56bbb856b5578aa5204cc004c5d9bfedc4514197` passed all four hosted `test` workflow jobs in run `35916946227`: Ubuntu, macOS, Windows and meet-spa-fresh. Original Go source, raw comparison, and 60s per-executable run contexts remain unchanged. Release notes and sh pin at `3b570e1c3a4466ceb768302c910c744256bfcc47` do not change this test or product behavior; the exact final docs head will receive a fresh three-OS workflow after Sprint 250 closure metadata lands.
