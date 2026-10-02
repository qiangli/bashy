---
id: 33099a9a1849
kind: feature
title: Provision Java SDK toolchain through binmgr
seq: 387
status: todo
priority: p2
labels:
    - toolchains
    - licensing
created: 2026-10-01T17:31:45.833161Z
sprint: 350
sprint_id: 406ae7fe-78aa-5ddc-9508-6e4dbf105b2a
sprint_title: Bashy release SBOM and license boundary
---

Goal
Give a bare Bashy host a managed Java SDK toolchain while keeping the JDK a separately downloaded runtime program, outside Bashy's compiled and distributed artifact.

Source reference
- https://github.com/openjdk/jdk is the JDK mainline source. Trace the chosen Temurin release to its OpenJDK source tag/commit and distribution patches; verify the exact archive's LICENSE, Classpath Exception, and additional license information rather than treating the source repository's top-level label as a complete binary inventory.

Implementation contract
- Add a binmgr-backed, pinned Eclipse Temurin JDK provisioner with explicit OS/architecture assets, verified digest, cache-first resolution, version override, and clear unsupported-platform/offline errors. Expose `bashy java`, `bashy javac`, and `bashy jar` from the same cached JDK; make Java available to Bash# toolchain resolution and `bashy check --prepare`. Do not fetch three copies for three commands.
- Provision Apache Maven separately with its own verified archive/cache and expose `bashy mvn`, using the managed JDK when a host JDK is absent. Reconcile the existing `java`/`javac`/`mvn` synopsis entries with live dispatch and the command atlas; add the missing platform declarations.
- Preserve subprocess behavior, JAVA_HOME/PATH, arguments, stdio, exit status, and Windows native path handling. Document the pinned JDK/Maven versions, override and cache behavior, and how an operator can inspect the actual downloaded assets.
- Verify the licenses and third-party notices of the exact Temurin and Maven archives. Temurin's GPL-2.0 with Classpath Exception is non-permissive for this inventory: record it explicitly in Sprint #350's runtime-acquisition section, with source/digest and any applicable notice or redistribution duties. Neither the JDK nor Maven may be embedded or shipped inside Bashy's permissive-only binary.
- Test cache hit, checksum failure, platform rejection, CLI passthrough, shared JDK use, Maven's JDK selection, and one clean-machine compile/run/package smoke on each supported OS; run the existing relevant cross-platform gates.

Acceptance
- On a supported clean host, `bashy javac` compiles a small Java program, `bashy java` runs it, `bashy jar` packages it, and `bashy mvn` runs with the managed JDK, with no system Java install.
- The release SBOM still describes only Bashy's shipped bytes; Temurin and Maven are listed separately as runtime downloads with their real license expressions and no permissive-only label for Temurin.
