---
id: 0a10de8d63f2
kind: test
title: 'S1 Site-owner shaping e2e: Hello World lifecycle smoke on the release binary (gate) + two-host promotion round trip'
seq: 3149
status: done
priority: p0
labels:
    - release
    - shaping
created: 2026-11-06T21:50:00.33224Z
assignee: s670-codex-manager
sprint: 670
sprint_id: 4b474d28-ab40-5a06-8d82-f14e76759a4a
sprint_title: Desktop journey e2e regression gate, then ship v0.18.78 navigation
closed: 2026-11-20T04:38:32.52316Z
closed_by: s670-codex-manager
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

## Worker rules (Sprint 670)

Do part 1 only; the conductor schedules part 2 separately. Build with GOWORK=off. Use a bashy dag target, never make. Run only the new smoke and its focused Go tests locally: no full suites, no podman, ollama or VMs on the dev box. The conductor runs the gate on the test host. Commit with trailers Sprint: #670, Story: #3149, Story-ID: 0a10de8d63f2 and your Agent line; do not push tags or release.

## Part 1 evidence (2026-11-19)

Delivered: `scripts/test-shaping-hello-world.sh`, dag targets `test-shaping-smoke` and `test-shaping-smoke-regression`, and the release-runbook section (bashy 85c7d897, plus 7a944a04 for an optional prebuilt browser-checkpoint binary, so the venue needs no Go toolchain or module downloads).
- Test host (by role), inside a Debian image with Chromium, git and curl, using a bashy binary built locally from the module cache (GOPROXY=off): **27/27 PASS**. Covered: register; candidate from a prepared change; show and diff; verify runs the app's real smoke.bsh with sealed evidence; preview; approve; alice sees the new greeting while bob still sees the original; the browser checkpoint for both principals; undo; submit without activating.
- Deliberate break (`SHAPING_SMOKE_BREAK=greeting`): FAIL at verify, exit 1.
- Remaining: part 2, the two-host promotion round trip.

## Delivery review (2026-11-20)

Authoritative two-container test on the existing Linux test venue, using the retained S670 candidate, reproduced a product defect before promotion: `shape remote set org <scratch-git-url> --app hello-world --json` rejects the registered app as an unknown source repository. The two disposable hosts and real Git transport initialized successfully. Root cause: promotion configuration calls the source-acquisition allowlist in `src.SetTier`/`ConfiguredTier`/`ConfiguredUpstream`; existing promotion unit fixtures use the allowlisted name `yoke`, hiding the custom-app case. Repair scope is explicit registered-app promotion configuration while preserving source-fetch restrictions. Initial harness workdir and network-DNS failures were venue setup issues and are not product failures. No passing promotion result is claimed yet.


## Final candidate verification (2026-11-20)

The custom-app promotion defect is fixed in Yoke `79e1473d372a5dc6ea2c78a3f4244ae1178bdf1c`. Promotion gets explicit app configuration accessors; source-fetch allowlisting remains intact. Focused `pkg/src` and `pkg/shape` regression: 180 pass, three existing Windows-only cases skipped on the Linux lane. Independent review approved the boundary. Bashy consumes published module `v0.0.0-20261120041602-79e1473d372a`, verified with `GOWORK=off`; six promotion CLI tests and Linux amd64, Darwin arm64 and Windows amd64 builds pass in the locked Go 1.27.2 builder.

The final Linux candidate SHA-256 is `4fd06d93d03ff85a570c4a6b01468a0f0297e5e6787a63fdd7c982e1692d50cd`.

- Two isolated hosts using real Git transport: **23/23 pass**. Includes verified submission, outgoing exit 3, exact org proposal, incoming exit 3, side-slot pull without activation, unchanged trusted verifier/contract, receiving-host verification and approval, independent undo, and exact upstream recorded ref.
- Negative control deletes the scratch org proposal: fails at incoming (exit 0 / pending 0 instead of required exit 3), proving a missing remote submission cannot pass.
- Model-free lifecycle with browser checkpoints passes on the same final binary. Deliberately broken greeting fails at the app's actual verifier, exit 1.
- Raw promotion evidence SHA-256: `794ea0650cc74a61e1bbcc7352d2115215018325233d9da4baa2f13a38a7e05d`. Raw records remain private because they contain venue addresses and temporary paths.

Harness setup failures were investigated before repair: local-directory import does not preserve source Git identity (use the scratch remote for host B); macOS archive metadata introduced an unintended extra check (disable AppleDouble transfer metadata when packaging). Neither failure was bypassed or counted as passing. The committed harness retains failed-command evidence and fails on cleanup errors. DAG target and release-runbook requirements cover both lifecycle and promotion; no release was performed.

Published Bashy implementation and promotion harness: `5a0a79be` (production file and module-lock hashes checked against the exact remote build inputs).

The final lifecycle fixture (including direct server PID cleanup and dry-run agent preflight) also passes **27/27** with native browser checkpoints on the macOS candidate, SHA-256 `6a10b2d81a6706eacbcf774aa9d058ef62083d6357fcd6041679e56e465d8ce7`. This is fresh native evidence; the earlier Linux lifecycle evidence remains separately retained.
