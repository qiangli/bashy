---
id: e9c799a66ea7
kind: task
title: S149.12 --go-package / --go-import-base / --go-list on bashy --source=go
seq: 256
status: todo
priority: p0
created: 2026-09-11T16:08:41.080363Z
sprint: 149
---

Per docs/bashpp-import-resolution.md section 3.2 and the user's requirement that resolution be auditable, verifiable and deterministic like go list. --go-package <importpath>=<file>[,<file>...] (repeatable, ordered), --go-import-base <base>, and --go-list which implies --check and prints one JSON object per recorded resolution in resolution order (from, import, path, origin, name, files) — stable across runs, no timestamps or host paths beyond the given file names. --go-package is accepted only with --check or --go-list; interpreted execution of an explicit package set is refused with a clear message (runtime import bridge does not consume the map yet). Tests: flag parsing, refusal without --check, listing golden output for a two-package set with ./a import under base test.
