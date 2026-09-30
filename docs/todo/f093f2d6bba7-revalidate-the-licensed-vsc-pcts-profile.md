---
id: f093f2d6bba7
kind: task
title: Revalidate the licensed VSC-PCTS profile
seq: 35
status: todo
priority: p1
created: 2026-09-03T00:23:52.71079Z
sprint: 110
---

Execute the licensed Profile B/VSC-PCTS procedure against the frozen candidate when the human-gated harness is available; compare results to the prior evidence, update the private handoff record, and keep submission/trademark claims separate from test completion.

## Review 2026-09-30 (steward)

- Status: valid but needs a refit. The last complete full arm is still Sprint 85 r1 (2026-08-30, 117/117 sets, 9,337 TPs, 98 blockers) at month-old pins; since then the coreutils/yoke split (Sprint 208), the Bash# engine work, the Sprint 269 reset and bashy v0.31.0 all landed. No full arm has run since.
- Outdated: "Profile B" and "prior evidence". It also overlaps Sprint 100 story cc4098a3f823 (the final certification campaign).
- Refit: make this story the fresh BASELINE full arm at the frozen candidate - Profile D plus the matched GNU control arm - so Sprint 100 works from today's blocker list instead of Sprint 85/88 identities. The final certification run stays in Sprint 100 (cc4098a3f823).
- Acceptance: 117/117 sets, 9,337/9,337 TPs, zero caps / runner failures / input drift; results retrieved, checksummed and stored in the private evidence store; blocker list diffed against Sprint 85 r1; no certification claim.
- Human gate: licensed suite availability; if unavailable, stop and record it on the card.
- Depends on: 1727b5446a3b, ae9b5ef7e7e8; start after 34046901d606 and 401837b6352f are green.
- Addendum (certification-body guidance, 2026-09-30): scored evidence must come from the supplied official harness itself (no substitute runner without the certification body's review), and certification is per architecture. So this baseline is a Linux x86_64 run under the supplied harness only; an arm64 arm is a separate budgeted run, and no Windows or macOS-native scored run is in scope (Windows is a quality target pending the certification body's review).
