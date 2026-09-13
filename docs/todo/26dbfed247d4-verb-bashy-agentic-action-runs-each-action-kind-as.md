---
id: 26dbfed247d4
kind: task
title: 'Verb: bashy agentic ACTION runs each action kind as intended; externals are handed off'
seq: 270
status: done
priority: p0
created: 2026-09-13T14:36:13.493758Z
weave: 15
assignee: claude-fable5
sprint: 166
closed: 2026-09-13T16:31:11.932686Z
---

SPRINT: #166. The verb: bashy agentic [--no-fix] [--no-elide] [--json] ACTION [ARGS...] and its bare-name shim (agentic ls -l inside a bashy shell). ACTION is a command (bashy verb, coreutils applet, builtin, PATH program), a script (file or -), an agent/skill record (file.yaml or -), or a skill name.
GATE: (a) bashy agentic ls is byte-identical to bashy -c ls (the coreutils applet, not /bin/ls); a script, an agent record (runs via bashy invoke) and a skill (runs via skill run) each run as intended with BASHY_AGENTIC=1 in effect; (b) an EXTERNAL binary (PATH program with no atlas/tool entry) is spawned directly with BASHY_AGENTIC=1, inherited stdin/stdout/stderr, exit status propagated, no post-processing (operator decision D3; Windows has no execve, so spawn + wait); (c) cmd/bash links none of it; (d) bashy agentic --help is help, never ACTION --help (e2e dispatch gate); (e) atlas row (Stage required; EffExec; never EffNet), verbSynopsis and naming ratchets green; (f) preflight = the existing check analyzer / dry-run resolution, then ONE execution (execlog shows one run).

HOW (verified seams): case "agentic" in bashy/internal/agentos/agentos.go + alwaysShimVerbs; native actions re-exec bashySelfPath() -c 'command "$@"' with inherited stdio - the dispatchFull shape in output_reduce.go:437-458 - so the userland and the exec chain apply; NOT run.go's runCommand (a PATH child with /dev/null stdin that bypasses the chain). Reuse runEnvelope/procStatus/advisor from run.go for the envelope (bashy-agentic-v1 wraps bashy-run-v1). If the front door turns out not to reach the observing chain for recording, fix it inside this story (141's PR1 932ada6d29ae describes the defect).

NOT THIS STORY: new rules (input story), the yield, any change to the agentic keyword or sh/.
