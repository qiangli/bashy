---
id: 4cd03f1f3116
kind: task
title: 'Sprint 118: preserve program arguments after explicit Go source inputs'
seq: 252
status: todo
priority: p0
created: 2026-09-09T06:57:33.47468Z
sprint: 118
---

Parent 6f0c4d9a31be. Implement CLI distinction between repeatable --go-file source inputs and program arguments following --. Actual unchanged Go by Example testing driver fails on --go-file A --go-file driver -- -test.v with cannot be combined with file operand. Preserve positional file and ordinary shell option behavior. Add real multi-file execution with testing-style flags, exit status and source hash verification; both AST interpretation and emitted compiled recipe. No original source rewriting or native whole-program forwarding. Commit proper sprint and story trailers. Manager owns merge gate.
