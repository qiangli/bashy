---
id: 92ab92b87aec
kind: bug
title: Repair hosted runner compiler capture for Go linker probes and Windows depfiles
seq: 1928
status: done
priority: p1
created: 2026-10-13T18:16:52.897976Z
assignee: s436-codex-manager
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T18:33:31.527956Z
closed_by: s436-codex-manager
---

Hosted Ollama run 38444418574 failed compiled-input capture. Linux amd64 and Darwin arm64 reject Go linker-generated go-link-*/trivial.c under the verified GOTMPDIR; Windows parses GCC drive-path depfile into a malformed relative path. Add narrow provenance for Go linker probe source, parse Windows absolute depfile paths without losing separators, add focused regression cases, preserve fail-closed unknown dependency behavior, and rerun hosted gate. Evidence: run 38444418574 compiler-evidence artifacts.
