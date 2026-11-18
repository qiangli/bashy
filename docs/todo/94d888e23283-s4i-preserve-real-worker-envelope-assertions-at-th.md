---
id: 94d888e23283
kind: refactor
title: S4I Preserve real worker-envelope assertions at the product boundary
seq: 3242
status: assigned
priority: p0
created: 2026-11-18T01:03:26.688134Z
weave: 110
assignee: agy-gemini3.8-flash
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
---

Implementation follows independently reviewed neutral error boundary in umbrella docs/s575-runtime-boundary.md. Before coreutils drops its upward worker implementation dependency, preserve the real worker-to-envelope integration tests in this product, where both owners may be imported. Copy frozen coreutils 6dd7a3ee pkg/weavecli/foreign_worker_test.go into bashy integration/weavecli/foreign_worker_test.go. Preserve every test, fixture, assertion, timeout, subprocess protocol and rendered expectation byte-for-byte apart from package/import qualification required to call the public github.com/qiangli/coreutils/pkg/weavecli API. Copy exact mustJSON helper from source foreign_error_test.go. A dot import in this test-only package is acceptable if it minimizes changes and preserves unqualified assertions. No production code or module/version changes, no upper test dependencies added to lower modules. Keep current pinned runtime import initially; the coordinated consumer-reference cutover will rewire it to Bashsharp after its accepted public version exists. Do not delete or edit coreutils tests here; its separate producer story removes the old copy after this new home is verified. No lower-module writes, no CLI/catalog edits, no dependency upgrades, no skip/assertion weakening or unrelated cleanups. ALL builds/tests run in manager's locked official image; no local Go build/test/run. Static formatting only. Commit isolated workspace with Sprint575/Story/Story-ID and actual agent, no pushes or branch switches. Report source equivalence and SHA. Manager runs the focused product integration DAG gate, then coordinates final accepted consumer pins.
