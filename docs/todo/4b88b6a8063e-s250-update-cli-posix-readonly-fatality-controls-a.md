---
id: 4b88b6a8063e
kind: bug
title: S250 update CLI POSIX readonly fatality controls after sh repair
seq: 333
status: todo
priority: p0
created: 2026-09-23T22:50:35.50976Z
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

Bashy exact-head CI run 35930047604 at 855b7ce failed Ubuntu job 107414182874 and macOS job in internal/cli only: TestBashDropinShModeKeepsEchoWithEmptyPath and TestStrictPosixNotEngagedByPosixFlag expect completion after readonly command-prefix assignment. sh Story #153 corrected that stale semantic; original GNU Bash 5.3 exits 1 before following command in noninteractive POSIX mode, while default non-POSIX mode still continues. Repair test-only assumptions to assert POSIX fatality across sh/bash/rbash/bashy as applicable, retain a separate probe that argv0 sh strict PATH lookup differs from pure Bash drop-in, preserve original product behavior and test limits. Acceptance: focused GNU Bash5.3 differential, internal/cli focused tests, exact-head Ubuntu/macOS/Windows CI green; no fixture exclusion or timeout change. VSC493 source/limits unchanged and requires fresh approved rerun.
