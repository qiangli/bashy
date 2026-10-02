---
id: db42d6dd2efc
kind: feature
title: S370.2 Java code fence in the engine (~~~java), lowering parity
seq: 399
status: todo
priority: p1
labels:
    - bashsharp
    - toolchains
created: 2026-10-02T23:04:19.487008Z
sprint: 370
sprint_id: d64d3ab6-4bbb-5bca-8a47-3d7e5dbf9bc6
sprint_title: 'Java fence: built-in ~~~java like Python, Rust and C/C++, with a provisioned JDK'
---

Goal
A first-class ~~~java code fence: parsed exports, typed values, an alias, lowering — like the Python, Rust and C fences. Not a runner fence.

Implementation contract
- sh/polyglot/java.go: runtime, analyzer and worker. RegisterLanguage row (canonical java). One persistent JVM worker per module (javac compiles the fence body plus a generated dispatcher into the bashy cache keyed by content hash; the worker speaks JSON lines like the Rust worker). Exports: public static methods of the fence's top-level class(es); typed values int, float, bool, string, bytes, object (JSON both ways) and handle; compiler errors reported with fence line numbers.
- Discovery in sh/polyglot/environment.go with BASHPP_JAVA / BASHPP_JAVAC overrides and the ToolResolver step for javac/java (S370.1 rows in bashy toolchains.go and check_prepare.go); a fence never resolves from PATH.
- LoweredRuntime: the lowered program embeds the compiled classes and launches the same worker, so interpreted and lowered runs agree on stdout and status (the Rust fence is the model).
- embed java "./Util.java" as util reuses the embed syntax.
- Engine work waits for the single-owner rule on sh/interp and sh/polyglot; check design card af65f24e132b first.

Acceptance
- Unit, interp and syntax tests pass; TestLanguageTable has the row; a fence method is callable by alias with typed arguments and returns a typed value on Windows, Linux and macOS with only the S370.1 JDK provisioned; a second run of an unchanged fence does not recompile; Bash OFF stays 86/86.
