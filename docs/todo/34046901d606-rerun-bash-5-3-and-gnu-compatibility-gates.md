---
id: 34046901d606
kind: task
title: Rerun Bash 5.3 and GNU compatibility gates
seq: 34
status: done
priority: p0
created: 2026-09-03T00:23:52.70682Z
assignee: claude-opus5.5
sprint: 110
closed: 2026-09-30T13:51:01.537785Z
closed_by: claude-opus5.5
---

Run the authoritative Bash 5.3 fixture gate against the frozen candidate and the applicable coreutils/GNU differential and regression gates. Compare with the pre-Go-1.27 baseline and classify every delta.

## Review 2026-09-30 (steward)

- Status: valid, not run for this candidate. Most recent comparable evidence is older and at earlier pins: Sprint 119 (2026-09-17) serial hermetic Linux GNU Bash 5.3 86/86 with Bash# OFF and ON, and the Sprint 269 (2026-09-24) "other gates once" pass.
- Outdated: the "pre-Go-1.27 baseline" is gone (Go 1.27 since 2026-09-07); compare against the Sprint 119 / 269 results instead. The GNU gate list was unspecified: use `make test-bash` (86/86 serial, the release gate), the container gate OFF and ON as Sprint 119 ran it, and the uutils scoreboard ONLY through `scripts/uutils-scoreboard.sh` in its OCI sandbox (never native; see the umbrella's conformance-test landmines policy).
- Next step: run those gates against the frozen candidate on the Linux runner; classify every delta against the Sprint 119/269 results.
- Acceptance: 86/86 serial with exit codes and candidate digest recorded; every uutils scoreboard delta classified (regression / fixed / environment); no unclassified new failure.
- Depends on: ae9b5ef7e7e8, 1727b5446a3b. May run in parallel with 401837b6352f.
- Addendum (2026-09-30 guidance): these gates are quality/regression evidence only; certification evidence must come from the official suite's own harness (story f093f2d6bba7), so nothing here is quoted as a certification result.
