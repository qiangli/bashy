---
id: 33099a9a1849
kind: feature
title: S370.1 Temurin JDK toolchain through binmgr and the bashy java/javac/jar front doors
seq: 387
status: todo
priority: p1
labels:
    - toolchains
    - licensing
created: 2026-10-01T17:31:45.833161Z
sprint: 370
sprint_id: d64d3ab6-4bbb-5bca-8a47-3d7e5dbf9bc6
sprint_title: Java as an optional registered runner fence (no built-in JDK)
---

Goal
Give a bare bashy host a managed JDK the way it has a managed Go, Node, Python, Rust and Zig: pinned, digest-verified, cached, executed as a separate program, never linked or shipped. Operator decision 2026-10-02: Java is a BUILT-IN fence language of bashy like the others; the earlier registered-runner-fence idea is withdrawn.

License (state it, do not soften it)
- Every OpenJDK build is GPL-2.0 WITH Classpath-exception-2.0; there is no permissive JDK. That is admissible exactly as GNU make and the POSIX providers are: download + exec under licensing-supply-chain-policy.md §2, recorded in the Sprint 350 runtime-acquisition inventory with the archive's own LICENSE and additional-license notices read from the real Temurin archive per platform. Never call it permissive; never embed or redistribute it.

Implementation contract
- yoke/external/java: Temurin LTS (pin the current LTS line), assets for Windows x64/arm64, Linux x64/arm64 (glibc) and macOS x64/arm64; sha256 from the Adoptium release API committed per platform; cache-first; version override via BASHY_JAVA_VERSION; unpinned platform refused with a clear message. Child env JAVA_HOME set, JAVA_TOOL_OPTIONS untouched; Windows path conversion through binmgr.Command.
- Front doors bashy java, javac, jar from the one cached JDK (no three copies). The synopsis entries removed on 2026-10-02 return with real dispatch, atlas rows and platform declarations.
- The usual touch points: bashy/internal/agentos/{agentos.go,commands.go,toolchains.go,check_prepare.go}, yoke/pkg/atlas/{atlas.go,platform.go}; tests modelled on node_test.go (cache hit, verified fetch, missing asset/digest, version override, passthrough).

Acceptance
- A host with no Java runs java -version and compiles+runs a hello program through bashy javac / bashy java from one verified cached JDK on Windows, Linux and macOS.
- The inventory row exists with per-platform license evidence; the release SBOM is unchanged.
