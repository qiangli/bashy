---
id: b9aaa2b3a0fa
kind: task
title: Preserve the caller environment for unchanged Go source execution
seq: 253
status: done
priority: p1
created: 2026-09-09T09:08:50.488205Z
assignee: sprint118-manager
sprint: 118
closed: 2026-09-09T10:17:56.093651Z
---

Assigned and actively executing: Einstein via internal Codex collaboration, isolated bashy companion /tmp/s118-gosource-env-cli/bashy and sh /tmp/s118-gosource-env-worker. Parent corpus Go by Example fa07603b71dc and Tour759341a95870. Capture exact incoming process environment before CLI agent/shell initialization, pass reviewed GoSourceEnv option. Preserve explicitly supplied BASH/SHELL/SHLVL/BASHY_AGENT_MANIFEST values, ordering, empty environment and Go os.Setenv/Unsetenv session behavior. Original environment-variables.go bytes unchanged; exact native/interpreter raw stream comparison. No output filtering or changed Bash environment semantics. Manager review/gates and submodule publication required.


Manager acceptance (2026-09-09): sh GoSourceEnv and CLI entry capture are integrated. Actual CLI/native comparison passed with shell-specific variables absent and explicitly supplied, including environment ordering and empty environment controls. The full 85-program candidate006 replay records environment-variables PASS; exact raw streams and receipts remain in sprint118-evidence/runtime-integration-006. Publication-006a make test, make build, and testing race gates passed. No source or output rewriting.
