---
id: 4cd03f1f3116
kind: task
title: 'Sprint 118: preserve program arguments after explicit Go source inputs'
seq: 252
status: done
priority: p0
created: 2026-09-09T06:57:33.47468Z
weave: 4
assignee: sprint118-manager
sprint: 118
closed: 2026-09-09T08:49:29.700763Z
---

Parent 6f0c4d9a31be. Implement CLI distinction between repeatable --go-file source inputs and program arguments following --. Actual unchanged Go by Example testing driver fails on --go-file A --go-file driver -- -test.v with cannot be combined with file operand. Preserve positional file and ordinary shell option behavior. Add real multi-file execution with testing-style flags, exit status and source hash verification; both AST interpretation and emitted compiled recipe. No original source rewriting or native whole-program forwarding. Commit proper sprint and story trailers. Manager owns merge gate.

Manager verification: reviewed 4fe4a91b and independently ran CLI selection tests plus actual two-file native/interpreter argument probe on candidate003. Flags after --, an empty argument, whitespace and --source=sh are preserved literally; both executions return7 and identical stdout/stderr, source bytes unchanged. Initial probe included the existing agent-configuration hint; deterministic AGENTS.md setup before both executions removed that harness setup issue. Evidence runtime-integration-003/cli-args-configured-manager-gate.json and cli-arguments-manager-gate.json passed. Full upstream test-driver execution remains tracked by corpus parent and sh#56; this closes the CLI argument defect only.
