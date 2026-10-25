---
id: 8006b239f47f
kind: enhancement
title: 'src: derive srcModulePins from the linked module build info instead of a hand-edited table (every yoke/coreutils/sh pin bump currently also edits internal/agentos/src_mount.go)'
seq: 1944
status: todo
priority: p2
created: 2026-10-23T19:33:38.381188Z
---

Found 2026-10-23 during the Sprint 458 S458.0 pin bump: TestSrcPinsMatchLinkedModules fails after 'go get yoke@X' until the commit literal in src_mount.go is edited by hand. Under go.work the linked versions read '(devel)', so the test also fails in umbrella builds. Derive pins from debug.ReadBuildInfo (or generate the table during the pin-sync step) so a pin bump is one command.
