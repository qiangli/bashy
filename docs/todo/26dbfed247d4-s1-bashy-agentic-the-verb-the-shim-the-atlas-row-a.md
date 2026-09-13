---
id: 26dbfed247d4
kind: task
title: 'S1 bashy agentic: the verb, the shim, the atlas row, and the external hand-off'
seq: 270
status: todo
priority: p0
created: 2026-09-13T14:36:13.493758Z
sprint: 166
---

SPRINT: #166 (epic yoke). SPEC: docs/bashy-agentic-action-design.md (S0). DEPENDS ON: S0; 141 PR1 932ada6d29ae (front door reaches the chain); 131 5fdd76c6be23 is the same defect and closes with it.
GATE: (a) bashy agentic ls output is byte-identical to bashy -c ls (the coreutils applet, NOT /bin/ls); (b) an EXTERNAL binary (a PATH program with no atlas/tool entry) is spawned directly with BASHY_AGENTIC=1, inherited stdin/stdout/stderr, exit status propagated, no post-processing, and the launch is recorded by the front-door observers first (operator decision D3, exec semantics; Windows has no execve so it is spawn + wait); (c) cmd/bash links none of it (import-graph ratchet); (d) bashy agentic --help is help, never ACTION --help (e2e dispatch gate); (e) atlas coverage, verbSynopsis and naming ratchets green.

WHAT: the front-door verb bashy agentic [--no-fix] [--no-elide] [--json] ACTION [ARGS...], case "agentic" in bashy/internal/agentos/agentos.go dispatch, a bare-name shim in alwaysShimVerbs (shims are registered unconditionally, --posix included - follow precedent), verbSynopsis entry, atlas row addVerb("agentic", Entry{Stage: StageCross, Group: GroupPlatform, Caps: {CapJSON, CapSpawnsProcesses}}) with EffExec in the effect lists and never EffNet (localfirst ratchet); Stage is the required field.

EXECUTOR SHAPE (verified 2026-09-13): NOT run.go's runCommand - it is a PATH child with /dev/null stdin that bypasses the wireExec chain. Native actions re-exec bashySelfPath() -c 'command "$@"' with inherited stdio - the dispatchFull shape in output_reduce.go:437-458 - so the coreutils userland, reduction (already on under BASHY_AGENTIC) and the observing chain apply. Externals are spawned directly by the verb (a bashy -c child would reduce their output, contradicting D3). Reuse runEnvelope/procStatus/newAdvisor().advise from run.go for meta only. Envelope schema bashy-agentic-v1: status ok|error|input_required, action facet, mode{agentic,surface,rung}, fixes[], diagnostics[], hints[], result (wraps bashy-run-v1), skill, context, entities.

TRAPS: bin/bashy defaults to Bash++ (cmd/bashy/main.go:29); in Classic, agentic { ... } is a parse error and the shim never fires - no verb-side refusal is reachable. agentic() { as a shim is safe in Bash++ (the parser rolls back on the paren). The verb sets BASHY_AGENTIC for its child only; gate executors must not inherit it.

NON-SCOPE: rung-1 rules (S2), preflight (S3), the yield (S4/S5), recording (S6).
