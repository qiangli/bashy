---
id: e7fedf196836
kind: task
title: Reference relocated Bash entry and shared CLI from product build targets
seq: 3245
status: done
created: 2026-11-18T05:01:01.871587Z
assignee: s575-codex-manager
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
closed: 2026-11-20T12:26:26.226226Z
closed_by: s575-codex-manager
---

Reference-only continuation after cmd/bash relocation. Update active build/test package arguments to the pinned Bash module cmd/bash package, shared CLI linker package names, and fixed Bash module pin. Preserve build logic and test assertions, no historical documentation rewrites. Manager owns final product commit and official-image build.

Official-image failure classification: TestClassicBashImportGraphRatchet rejects the unchanged pure CLI after its package moves to github.com/bashsharp/bashsharp/cli; permit that exact namespace while preserving all existing bans. TestSrcPinsMatchLinkedModules and TestSrcPinsReplaceTheLinkedModulePath fail because source-pin metadata still names the former fork and old coreutils/Yoke commits. Correct only module/commit metadata to match go.mod; retain the existing sh protocol alias while targeting the direct canonical Bash module. Also correct remaining full cmd/bash package references to the Bash repository. No engine implementation changes or assertion weakening. Fresh focused verification is performed by the manager.

## Manager closeout — 2026-11-20
Build/link references published in 7504b5b7/fbb54d3. Retained TestClassicBashImportGraphRatchet, TestSrcPinsMatchLinkedModules and TestSrcPinsReplaceTheLinkedModulePath all passed after necessary reference updates. Later consumer-pin qualification is recorded at 1ad76277 with unchanged-baseline failures preserved.
Existing receipts and source ancestry were reviewed; no new test/build ran during reconciliation. Evidence belongs to its exact historical source/artifact identities, not arbitrary later heads. S575 full-run evidence remains under posix-baseline/s575-2026-11-19 with its raw 4 FAIL, 54 UNRESOLVED and 9 INSPECT unchanged. RC38 is on operator hold.
