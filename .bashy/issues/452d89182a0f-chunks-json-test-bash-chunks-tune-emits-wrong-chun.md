---
id: 452d89182a0f
kind: bug
title: 'chunks.json: test-bash-chunks-tune emits wrong chunk count; manifest path relative; coverage test uses hand-copied list'
status: open
reporter: qiangli
created: 2026-07-13T00:04:05.115281Z
---

Judge (claude-opus) revise findings on merged run #11 (chunks.json, commit in f3c197c) — follow-ups, none block the release gate (authoritative run is unchunked serial, 86/86):

MAJOR: test-bash-chunks-tune's doc dropped the CHUNKS=8 guidance but its body passes no CHUNKS to dag-fanout-tune, which defaults to chunks=${CHUNKS:-12} (dag.md:509) — so the tuner emits a different chunk count than the pinned manifest (8). Align the tuner default with chunks.json's chunk_count or pass CHUNKS explicitly.

MINOR: -chunks-manifest defaults to relative 'chunks.json' while -tests-dir/-bash are absolutized, so 'bin/bash53suite -chunk 1/8' from any cwd != repo root fails with 'open chunk manifest'. Absolutize the manifest path too.

MINOR: the manifest coverage test (tools/bash53suite/main_test.go) validates chunks.json against a hand-copied 86-name list rather than the discovered corpus — so a fixture add/rename passes green while the manifest silently drifts. Derive the expected set from the tests dir. (Judge on #13 flagged the same class: tests that assert string assembly, not the failure-mode logic.)

NIT: hardcoded '/bin/bash scripts/test-bash-parallel.sh' bypasses the script's own env-bash shebang; breaks on hosts without /bin/bash (NixOS/minimal containers).
