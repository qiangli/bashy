---
id: 33099a9a1849
kind: feature
title: S370.1 Java as an optional registered runner fence (no built-in JDK)
seq: 387
status: todo
priority: p2
labels:
    - toolchains
    - licensing
created: 2026-10-01T17:31:45.833161Z
sprint: 370
sprint_id: d64d3ab6-4bbb-5bca-8a47-3d7e5dbf9bc6
sprint_title: Java as an optional registered runner fence (no built-in JDK)
---

Goal
Operator decision 2026-10-02 (reaffirmed after a mis-typed reversal the same day): Java is kept but NOT built in. Unlike Python, Rust and C/C++ it gets no sh/polyglot row and no bashy-provisioned toolchain: every OpenJDK build is GPL-2.0 with Classpath Exception, there is no permissive JDK, and bashy prefers permissive toolchains. Java enters only as an OPTIONAL registered fenced block over a JDK the user already has.

Contract
- The mechanism is the existing runner fence (~~~java as j !<runner> / embed java "./Foo.java" as foo !<runner>, sh/docs/bashpp-polyglot-fences.md §Text fences and the runner override): the runner is a Bash# func in the unit or a command the user registered with bashy commands add (digest-pinned, effects declared), never a PATH lookup and never a bashy download. The runner answers methods <file> and the verbs it chooses (compile, run, jar, test).
- Ship one worked recipe, not a provisioner: an example unit under examples/ plus a doc section "Java in Bash#" showing a func runner over javac/java from JAVA_HOME or a mise-managed JDK, stating GPL-2.0-with-Classpath-Exception and that bashy fetches no JDK.
- bashy check --prepare and toolchain resolution gain no Java row. The java/javac/mvn synopsis entries stay removed. bashy doctor may report a user-registered Java runner like any registered command.
- Record in the Sprint 350 runtime-acquisition inventory: Java = user-supplied, never acquired by bashy.

Acceptance
- With a host JDK present and the runner registered, the example unit compiles and runs a Java method through the fence, interpreted; a func runner lowers, a command runner is refused at compile time by name as the runner-fence rules say.
- With no JDK and no registration, the fence fails with the registration hint; bashy commands advertises no java/javac/mvn verb.
- No binmgr call, no download, nothing GPL in any bashy artifact; release SBOM unchanged.
