# Sprint 561 A4: licensed run results (counts only)

Licensed journals and reports stay outside Git, in the operator's private
evidence store. This file records identities and aggregate counts only.

Approval (separate layout): approval `309645b7`, harness `0b2b185d`, Bashy
`f1bcd711`, sh `74586fde`, coreutils `211789fa`, public umbrella `4e6c63ed`;
transfer validator PASS.

## Profile B — 493-TP shell release gate (2026-11-04)

Arm `shell-release-20261104-s561-r1`, disposable leased host (retired after
verified retrieval). Result: **PASS** — 1 set, 493 declared / 493 observed
TPs, 493 in the certification PASS group; missing 0, caps 0, runner failures
0, blockers 0, manual resolutions 0.

## Profile D — full Sprint-341 base run (2026-11-04)

Arm `profile-d-s561-20261104` on a leased disposable host (retired after
verified licensed and public retrieval). Clean-host gate: 102 commands =
92 Go + 10 shell, 0 external providers, 14 optional excluded. Runner:
`ARM_DONE tsets=117 scope=full caps=0 failures=0`, 0 rejected attempts,
0 skipped. Campaign retrieval PASS (`full_profile_d=1`).

| Metric | Value |
| --- | ---: |
| journals / test sets | 117 |
| test purposes | 9,337 |
| certification PASS group | 9,270 |
| FAIL (code 1) | 4 |
| UNRESOLVED (code 2) | 54 |
| INSPECT (code 102, manual resolution) | 9 |
| UNINITIATED / UNREPORTED | 0 / 0 |

These counts are identical to the previous full Profile D run (4 FAIL, 54
UNRESOLVED, 9 INSPECT): **no regression**. Items from the S355 five-TP list
(`hash:9`, `write:22`, `renice:16`, `kill:19`) no longer appear as blockers.

### Residual non-pass results (dispositions, not code)

| Results | Test sets | Disposition route (existing private packets in the harness kit) |
| --- | --- | --- |
| 53 UNRESOLVED | one empty-symlink setup TP in each of 53 sets | S355 symlink-53 authority packet: no configuration predicate resolves them; needs a VSC authority ruling (prepared, unsent) |
| 1 UNRESOLVED | `man:4` | S355 five-TP questions: the suite's >109-name command-list prerequisite is reached with exactly 109, so `man` is never invoked (prepared, unsent) |
| 4 FAIL | `more` | S355 `more` scope question: editor dispatch at final EOF (prepared, unsent) |
| 9 INSPECT | `stty` (7), `df`, `locale` | S377 operator manual-inspection decisions (manual PASS rule on recorded evidence) |

All 67 are outside Bashy's code: none is a product defect found by S561
triage (A2/A3c). Closing them requires the operator to apply the S377 manual
resolutions to this journal and to send the S355 authority packets.

## Handoff pack

- Candidate: Bashy `f1bcd711`, sh `74586fde` (on sh `master` via `52e71801`),
  coreutils `211789fa`; umbrella approval revision `4e6c63ed`; harness
  `0b2b185d`; approval `309645b7` (private, `cert-approval/341-d`).
- Evidence identity: `scripts/cert-identity-check.sh
  docs/s561/candidate-manifest.json HEAD` prints `SAME` for both cert builds
  on current `main`.
- Public gates on the candidate line: CI green on Linux/macOS/Windows
  (`93a7ce03`), `make test-bash-container` 86/86, full `GOWORK=off go test
  ./...` clean after F1, `make dist` 12 binaries hashed.
- Licensed evidence: private, outside Git (operator's evidence store).

## Operator-approved engineering closure

The operator requested Sprint 561 closure after confirming that engineering
was complete, the final licensed run finished at 7:13 PM PST, both disposable
hosts were retired, all delivery changes were pushed, and the other sprints
could resume. This supersedes the original requirement to finish all authority
and manual dispositions before A4 acceptance: engineering closes with no
regression and the exact raw non-pass counts preserved. It does not establish
full certification readiness, official certification, or release approval.

Evidence reviewed for closure: this licensed aggregate report, the frozen
candidate manifest, A4a public-lane report, and A3d repair report. Existing run
evidence is reused; cleanup does not constitute a fresh test or licensed run.
The residual total is **67** (4 + 54 + 9); the earlier prose total of 62 was
an arithmetic error, with no change to any recorded result.

Outstanding decisions remain with the operator and VSC authority through the
existing S355/S377 harness-kit packets: send the prepared S355 questions for
53 symlink setup results, man:4 and four more failures; apply S377 manual
resolutions to the nine INSPECT results in this run's private journal. Packets
remain unsent and manual dispositions are not claimed complete by this closure.
