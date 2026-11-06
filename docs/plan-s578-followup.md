# S578 handoff: remaining S562 fence corpus work

S562 closed early by operator decision on 2026-11-06. Its accepted stories are the delivered scope. The final serial Bash86 gate was not run, so there is no sprint-wide PASS claim. All 26 open S562 stories were carried to S578 with their criteria unchanged and a carry note. Windows, C#, and PowerShell remain in S573.

## First work

1. P0 telemetry `e385262ac250` in Yoke: bound spool rotation and stop Windows disk exhaustion. Retain the existing emergency run and backup evidence; verify on reviewed, merged source before acceptance.
2. P0 caches `1bd89e54e114` in bashsharp-tests: bound per-case compiler caches. Preserve its in-flight branch and test packet; verify on reviewed, merged source before acceptance.

Continue C++ reconciliation, Go E5, supported TypeScript/Rust rows, B2 scrub/publication, B4 frozen-candidate verification, and the final serial Bash86 gate. Unsupported Bash# rows remain deferred work, never PASS. The S578 board is the authoritative list of carried stories; each story retains its body and run/evidence references.

## Continuity and limits

- S562 ended dispatch and new heavy gates at the operator's early-close decision. Resume parked runs from their linked weave workspaces; do not discard branches with unique commits.
- The shared Bashy checkout contains other agents' uncommitted plan and story files. Do not sweep those into a commit or push. Published Bashy `0b1fb009` and Yoke `698d3161` are already integrated; the shared Bashy checkout is behind and dirty.
- B2 evidence must be scrubbed before publication. Never expose the private host identifier.
- The host reported 891 MiB free at 18:33 UTC. Check capacity before resuming builds. The Go E5 follow-up launch was refused below Weave's 2 GiB floor.
- Acceptance requires existing story criteria proven on reviewed, merged source. The old Make gate failure and absent final serial Bash86 result remain recorded as unverified, not success.
- `bashy sprint prune 562` reported an unclean state: uncommitted files in shared repositories, 19 unpushed `sh` commits, and unmerged run branches. `bashy sprint end 562` refused because 34 linked runs lack a disposition. Preserve and review these runs before any close attempt; do not abandon unique commits just to satisfy lifecycle bookkeeping.
- The S562 elephant host claim is lapsed but held by `codex-gpt6-sol`; release by this manager identity was refused. The holder was asked to release it if safe.
- A no-gate `bashy sprint stop 562` also refused to close the time box because linked repositories were not wrapped up: uncommitted files in bashsharp-tests, sh, bashy, and umbrella, plus a stale Bashy `sh` pin. It did not record a verified gate. Reconcile these without sweeping other agents' files into a commit.

See `docs/plan-s562-master-execution.md`, S562 and S578 sprint cards, and each carried story for exact acceptance and evidence.

## Run transfer decision (2026-11-06)

The following runs were transferred from S562 without pulling, abandoning, forcing, or deleting their branches. Each line names the target sprint and story.

- `bashsharp-tests#44 -> S578 3b8b3569ea3f OK (prior command)`
- `bashsharp-tests#45 -> S578 e5765c939c75 OK`
- `bashsharp-tests#46 -> S578 007e20ec5c30 OK`
- `bashsharp-tests#47 -> S578 e0402848f814 OK`
- `bashsharp-tests#48 -> S578 8355d30f6253 OK`
- `bashsharp-tests#49 -> S573 5e925b5d1d84 OK`
- `bashsharp-tests#50 -> S578 8be5394e8ce1 OK`
- `bashsharp-tests#51 -> S573 d73d4ea89087 OK`
- `bashsharp-tests#52 -> S578 b4847d5fb8a6 OK`
- `bashsharp-tests#53 -> S573 f37513c7b9c1 OK`
- `bashsharp-tests#54 -> S578 1bd89e54e114 OK`
- `bashy#21 -> S578 feae8aa96a9a OK`
- `bashy#22 -> S578 ca8cc67e0287 OK`
- `s562-b1-rework-base#1 -> S578 feae8aa96a9a OK`
- `s562-b1-rework-base#2 -> S578 feae8aa96a9a OK`
- `s562-b2-scrub-base#1 -> S578 ca8cc67e0287 OK`
- `s562-cache-p0-base#1 -> S578 1bd89e54e114 OK`
- `s562-cache-p0-base#2 -> S578 1bd89e54e114 OK`
- `s562-combined-go-reviewed#1 -> S573 f3ae837ba6dd OK`
- `s562-independent-vendor-base#1 -> S578 b4847d5fb8a6 OK`
- `s562-independent-vendor-base#2 -> S578 85dd0272619c OK`
- `s562-independent-vendor-base#3 -> S578 007e20ec5c30 OK`
- `s562-pwsh-l844-triage#1 -> S573 5f1ecbdd16bd OK`
- `s562-pwsh51-provider-design#1 -> S573 dc1e8ca9c503 OK`
- `s562-telemetry-emergency#1 -> S578 e385262ac250 OK`
- `s562-telemetry-emergency#4 -> S578 e385262ac250 OK`
- `sh#14 -> S573 a437de22974a OK`
- `sh#20 -> S573 759e75a52741 OK`
- `sh#23 -> S578 007e20ec5c30 OK`
- `sh#24 -> S578 0f4ea3117c92 OK`
- `sh#27 -> S578 49cf2426bee3 OK`
- `sh#28 -> S578 fc03f80cf2ba OK`
- `sh#29 -> S578 007e20ec5c30 OK`

`sh#22` (PowerShell locale/terminal environment) remains intact but unlinked. The CLI rejected both S573 and S562 links because the submitted run lacks a bounded launch runtime. Its intended target is S573 story `093a8fc1a2f3`; the queue is `sh-b1a2e1b7`, branch `agent/weave-issue-22`. Resolve this metadata blocker before further lifecycle cleanup.

After the transfers, `bashy sprint end 562` reported no remaining undecided linked-run blocker. It refused repo wrap-up on `sh` (7 uncommitted files), `bashy` (8 uncommitted files and stale `sh` pin), `bashsharp-tests` (1 uncommitted file), and umbrella (1 uncommitted file). No gate ran and S562 remains open. The supervisor has the exact blocker report; do not commit or push other agents' files to close it.
