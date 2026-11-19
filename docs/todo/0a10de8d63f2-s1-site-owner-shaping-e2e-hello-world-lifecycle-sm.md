---
id: 0a10de8d63f2
kind: test
title: 'S1 Site-owner shaping e2e: Hello World lifecycle smoke on the release binary (gate) + two-host promotion round trip'
seq: 3149
status: todo
priority: p0
labels:
    - release
    - shaping
created: 2026-11-06T21:50:00.33224Z
sprint: 670
sprint_id: 4b474d28-ab40-5a06-8d82-f14e76759a4a
sprint_title: Desktop journey e2e regression gate, then ship v0.18.78 navigation
---

Proves rc.25 is ready for shaping. On the dev host, use the installed frozen candidate (at the canonical installed path) to run the hello-world CLI lifecycle from examples/shaping/hello-world: shape wish -> verify -> approve -> undo -> restore.

Then run one real round trip with the P1 rod (a130d972) between two hosts (named by role):
1. Host A shapes a change.
2. `shape outgoing` lists it (exit 3, --json).
3. `shape push --to org`.
4. Host B: `shape incoming --from org` lists it.
5. Host B: `shape pull --from org` (gated, from the side slot), then `shape undo`.
6. Host A: `shape push --to upstream` opens the upstream PR.

Record the exact candidate SHA, the installed binary digest, the commands and the results as one evidence record. A failure is tracked in the known-issues ledger and never reported as passing. If P1 misses the cut, record the local lifecycle, and record the rod legs as BLOCKED.

Carried over from Sprint 580 at close (2026-11-07). Status: NOT DONE. v1.0.0-rc.27 is released and promoted, so run this against the released v1.0.0-rc.27 build. The rod CLI shipped in rc.27 (shape remote/outgoing/push/incoming/pull). Still to do: the hello-world lifecycle on the installed release, then the two-host round trip through a scratch org remote.

## Rescoped into Sprint 670 (operator, 2026-11-19): automated, release-gating

An audit found that site-owner shaping has never been tested end to end automatically: every test fakes the agent, the build or the promotion. Promotion between hosts has never run at all. This story becomes two automated checks instead of one manual smoke:

**Part 1: Hello World lifecycle smoke, model-free (bashy release gate).** A script or DAG target (bashy dag, never make) takes the *built* bashy binary under test, a disposable BASHY_HOME and a temp copy of `examples/shaping/hello-world`. It runs, asserting each step:
- `register.sh` registers the app;
- `shape wish --from <prepared workspace changing the greeting to "Hello, Shaping!">` creates a candidate;
- show and diff the candidate;
- verify runs the app's real `checks/smoke.bsh`, with no fake verifier;
- preview serves the new greeting;
- approve;
- a second principal still sees "Hello, World!";
- undo restores "Hello, World!";
- submit records the candidate without activating it.

It also runs the existing browser checkpoint test (`TestDOMExternalShapeCheckpoint`, -tags verifydom) against the console this run started, with BASHY_E2E_CONSOLE_URL set. Deterministic and free: it runs on every bashy release candidate and blocks the release on failure. Document it in the bashy release procedure.

**Part 2: two-host promotion round trip.** outgoing → push to an org remote → incoming/pull on a second host → undo → push upstream (a PR or recorded ref) between two disposable hosts. It runs on the fleet e2e harness, and is required for releases that touch shaping, promotion or the shape CLI.

Done when: part 1 passes on the test host against the release candidate binary and fails when a step is deliberately broken (for example a smoke check that expects the old greeting); part 2 passes once on the fleet harness with evidence; and both are written into the release procedures.
