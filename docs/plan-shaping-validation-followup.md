# Software shaping validation follow-up — Sprint 579

Sprint UUID: `b3b68c8f-69de-5654-bab3-e0665d048fd4`.

The operator authorized closing Sprint 564 as implementation delivered and moving
unfinished qualification here. This is a backlog plan, not validation acceptance
or release approval. Preserve the original story identities and acceptance bars.

| Story | Remaining work | Starting evidence and dependency |
| --- | --- | --- |
| H5 `5f0412e066bd` | Full CLI/service/console lifecycle, negative guards, exact rollback and data preservation on Linux, macOS and native Windows | Real Linux CLI/service cycles passed on the recorded controller; complete platform coverage remains open. Genuine self-handoff depends on Sprint 570 H1. |
| H7 `515557c02505` | Both scenarios across four agents and CLI/MCP/REST, plus actual UI self-shaping | 7/24 reviewed passes. Original failed CLI cell remains failed despite operator recovery. Prepared next MCP fixture requires a matching artifact and fresh prerequisites. |

Read [the validation record](s564-validation.md) and the original
[implementation plan](plan-s564-master-execution.md) before running anything.
The implementation is integrated in Bashy `0b1fb009` / Yoke `698d3161`, pinned by
umbrella `605fe78`; later handoff commits change tracking/documentation only.
Affected merged-source checks passed. Historical native lifecycle results remain
tied to Bashy `282a314f` / Yoke `6b865c24`; its serial shell gate passed 86/86.
Its full Make run failed in Desk registration/aggregate timeout, outpost-profile
path conversion, source-pin checks, skill help and ELF-audit timeout. Combined
source-pin checks subsequently passed; retain the other failures and baseline
reproductions, assigning owning-layer follow-ups rather than claiming green.

1. Confirm Sprint 570 H1 delivery and native host/provider/storage prerequisites.
2. Freeze exact source, dependencies, toolchains and artifacts. Reuse evidence
   only when its relevant inputs match; integrate changes before expensive runs.
3. Complete H5 on isolated native hosts, claiming each shared resource first.
4. Resume H7 with the prepared MCP lane, then one complete CLI/UI self-shaping
   scenario before wider fanout. Real agents and browser assertions are required.
5. Reconcile every matrix cell and unresolved failure, run affected regression
   gates, and accept stories only against their original criteria.

Private raw evidence and all original failures are retained off-host in a
checksum-verified archive. Required Git bundles were restored and OCI images
reloaded/inspected independently. The temporary Linux runner was deleted after
retention verification; a new claimed runner is needed for further Linux work.
Do not reuse disposed resource addresses, copy credentials into stories, clean
shared caches without authorization, or count unavailable coverage as passing.
