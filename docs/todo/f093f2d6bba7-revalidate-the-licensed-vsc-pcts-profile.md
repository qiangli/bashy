---
id: f093f2d6bba7
kind: task
title: Revalidate the licensed VSC-PCTS profile
seq: 35
status: todo
priority: p1
created: 2026-09-03T00:23:52.71079Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
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

## Carried from Sprint #110 to Sprint #100 (steward decision 2026-09-30)

Not run in #110 (on-time/KISS). Inputs ready for the baseline:
- Frozen linux/amd64 candidate, **SUT digest 0844a95e** — bashy 2db80ed / sh
  831b6b2d / coreutils c0c6cae7 / yoke 578a169, go1.27.1, CGO_ENABLED=0,
  two cold builds byte-identical; manifests in umbrella
  docs/sprint-110-evidence.md and dragon ~/.bashy/sprint/110/evidence/freeze.
  Stage with `CGO_ENABLED=0` and diff /vsc/stage sha256s against it first.
- Preflight gates on that candidate: see 34046901 / 401837b6 (Sprint #110).
- GNU control: reuse the retained 2026-08-23 Profile A control per
  RUNBOOK ("do not rerun profile A merely because Bashy changed"); no second host.
- Duration: a full D arm on 2 vCPU was ~7h tests + ~45m setup in Sprint 85
  r1 (~8.5h validation to retrieval) — give it its own time box.
Blocked on two harness contract gaps (stories filed on #100 in
vsc-pcts-harness-kit): adopting a dhnt-ephemeral host with a genuine
provision record, and approval_ref on a pushed main/approval ref with exact
gitlinks. The #110 runner s110-cert-amd64 and its .vsc-phase-gate were torn
down at #110's end; the phase-gate record must be re-issued for the #100 host.
