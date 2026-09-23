---
id: 4b88b6a8063e
kind: bug
title: S250 update CLI POSIX readonly fatality controls after sh repair
seq: 333
status: done
priority: p0
created: 2026-09-23T22:50:35.50976Z
assignee: codex-s250
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-23T23:42:47.928407Z
closed_by: codex-s250
---

Bashy exact-head CI run 35930047604 at 855b7ce failed Ubuntu job 107414182874 and macOS in internal/cli after sh Story #153's first patch. Investigation found the physical-line boundary: GNU Bash 5.3 POSIX mode rejects a readonly command-prefix assignment, discards the remainder of that physical line, and continues at the next newline. The existing CLI runStrictProbe puts its trailing command on a new line, so its completion expectations were correct; the first sh patch was too broad. After sh #153 is corrected, retain the existing CLI next-line and strict argv0-sh controls. Add a focused same-line control to prevent this distinction from regressing; preserve non-POSIX behavior, fixtures, and time limits. Acceptance: matched GNU Bash5.3 controls for same-line versus next-line output/status, internal/cli focused tests, exact-head Ubuntu/macOS/Windows CI green. VSC493 source/limits unchanged and requires fresh approved rerun.
