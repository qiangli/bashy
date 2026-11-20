---
id: 8793e820356f
kind: test
title: H7a Pre-release real-agent Hello World shaping run (one cheapest working agent, budget-capped)
seq: 3246
status: done
priority: p1
labels:
    - shaping
    - e2e
created: 2026-11-19T16:28:17.460033Z
assignee: s670-codex-manager
sprint: 670
sprint_id: 4b474d28-ab40-5a06-8d82-f14e76759a4a
sprint_title: Desktop journey e2e regression gate, then ship v0.18.78 navigation
closed: 2026-11-20T04:44:10.791362Z
closed_by: s670-codex-manager
---

Slice of H7 515557c02505 (which stays in Sprint 579 for the full 24-cell matrix). Before each bashy release, run bashy/examples/shaping/hello-world/e2e-agent.sh once with the cheapest working agent (pick from the probed fleet; record which) over the CLI interface. It goes wish 'Hello, Shaping!' -> real agent edit -> real verify (checks/smoke.bsh) -> preview -> approve -> second principal unchanged -> undo. This catches regressions in the shaping brief, model output parsing and the real BashyDelegate, which CI never exercises. Opt-in (an env flag), capped (one run, and a budget note per release), and evidence recorded in the release packet. A failure blocks the release unless the operator records an override with a reason. Done when: one passing run is recorded against the current release candidate, and the release procedure names the step and its override.


## Gate preparation findings (2026-11-20)

No model generation ran in the first two gate attempts. Gate 1 was a fixture/interpreter defect: the launcher entered GNU Bash explicitly but recursively selected a different `bash` through PATH; preserve the invoking `$BASH` and use nounset-safe variable enumeration. Gate 2 was a macOS test-venue failure: the console was listening on loopback, while an unbound IPv4 client returned EADDRNOTAVAIL; explicitly binding the client source address connected. The venue had 38,407 TIME_WAIT sockets against a 16,384-port default ephemeral range. No product changes, skipped browser assertions or kernel parameter changes are justified by this result. A separate fixture cleanup defect left the server behind because the recorded PID identified an intermediate function subshell; direct exec now records/reaps the actual server. The identified run-owned orphan was stopped. Raw manager evidence retains both failed gates; they are not passing acceptance.

Venue diagnosis subsequently confirmed 16,383 of 16,384 default ephemeral ports occupied; a temporary widening of the ephemeral range restored ordinary loopback connectivity, and the original setting was restored by the gate wrapper. Gate 3 then reached the browser preflight and exposed a missing browser launcher name: the native browser library searches for `google-chrome`, while the prepared cached-browser wrapper was named `chromium`. Added the expected alias in the private venue tool directory. Still no model generation; all failed setup evidence retained.

Gate 4 passed the native browser baseline, then the delegate rejected a retired Flash-low agent alias before provider invocation (8 ms). Updated private catalog selection to current `agy:gemini3.8-flash`; dry-run preflight now records resolved identity/model before any spend. The existing agent executable was frozen privately: agy 1.5.13, SHA-256 `3f669d53b6a9807b161c327bf6ce33cdd24ecbf3026edcbb7347c004d1a2d9f2`. No agent installer or download was run. The isolated tool catalog references this frozen binary and the existing private provider-login symlink.

Gate 5 resolved the current agent and passed the browser baseline, but launch admission correctly refused the seeded permission-bypass argument on an uncontained native host. An expired-deadline diagnostic exposed the structured denial; no provider call ran. The private profile is corrected to use provider sandbox and accept-edits mode, preserving the production containment guard. The model-free final fixture separately passes all 27 native checks with the same candidate.

Gate 6 was the first provider invocation: it read the HTML, requested `git status`, and stopped when accept-edits mode denied that command. Two model responses were recorded; no edit occurred, and the unchanged-tree guard failed correctly. Provider documentation confirms sandboxed Auto mode approves workspace edits and safe sandbox commands while separately reviewing commands outside the sandbox and retaining prompts for uncleared actions. The private profile will use that documented mode. Manager authorizes one additional, deliberate 600-second capped invocation after this diagnosed setup correction; no automatic retry or blanket permission bypass. Auto review may consume additional internal model calls within that single bounded provider invocation.

Gate 7 completed the real edit using sandboxed Auto mode, then exposed a fixture assertion defect: the original source commit and managed imported-base commit have identical trees but different identities (initialization versus import). The original source reflog contains only initialization and its worktree remains clean. Comparing those two commit IDs falsely reports a source mutation. Correct the assertion by snapshotting the original source HEAD before the wish and comparing that snapshot afterward. Preserve the failed record; permit one final bounded qualification of the corrected driver. No model-output substitution or weakening of source preservation is involved.


## Accepted qualification (2026-11-20)

Final full lifecycle: **38/38 PASS**, 50.021 seconds, native macOS test venue. Selected current subscription-backed `agy:gemini3.8-flash` (resolved model `gemini-3.8-flash-high`), frozen agy 1.5.13, provider sandbox and documented Auto mode, one provider invocation capped at 600 seconds. Five setup failures preceded provider invocation; two diagnosed provider-attempt failures remain separately retained, followed by this passing qualification. No automatic retries or failed-as-passed results.

- Bashy production source: `5a0a79be60fc65fa5ace4c69cf77f6e2e6ef2bff`, published Yoke pin `79e1473d372a5dc6ea2c78a3f4244ae1178bdf1c`; native binary SHA-256 `6a10b2d81a6706eacbcf774aa9d058ef62083d6357fcd6041679e56e465d8ce7`.
- Lifecycle driver SHA-256 `3ca75148987ab2b97ab162447c97ab640c25456bfaca58fbba3e8be9a327ebab`.
- Real candidate `eda0c9eeac5f604a98c36d0fa55b0aaf850bd2ed` changes only the greeting. Original source HEAD/worktree, trusted verifier and contract remain unchanged.
- Actual smoke verifier passes. Six fresh browser checkpoints pass: original, preview, still-original active page, approved page, second principal unchanged, and undo. Submission does not activate.
- Private evidence archive SHA-256 `8e6c44edd5e94d6bef31b513983c21336145964c823e4cbc5df20895a2fc77e6`; transcript SHA-256 `0bfc4f396716453c1d2c7d877be7b8948db4e7c8bdacd31c031950c219288d75`. The archive contains selected test observations, not provider credentials or cookie jars.

The test-owned server is reaped and the original venue network setting restored. The release procedure requires this passing real-agent gate and names the operator-only reasoned override. No release or deployment was performed.
