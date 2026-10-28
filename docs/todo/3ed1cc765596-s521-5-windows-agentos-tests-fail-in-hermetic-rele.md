---
id: 3ed1cc765596
kind: bug
title: S521.5 Windows agentos tests fail in hermetic release-preflight (CI skips them on bare clones)
seq: 2818
status: todo
priority: p0
labels:
    - known-failures
created: 2026-10-28T06:18:22.261574Z
sprint: 521
sprint_id: 8215f52e-1496-5a0f-b6f6-729bf3056119
sprint_title: 'Fleet upgrade fix release + ivy deploy: ship outpost upgrade-ordering fix (cd8b98c4) as bashy rc.22, move every host incl. ivy (lern, lj2ivy) onto it once, then promotions auto-flow'
---

From S514 hermetic preflight (bashy eff3625e, rc.20): ./internal/agentos on Windows peer fails TestGenieRecipeGeneratesCallerWorkspaceAndModel (274s timeout-class), TestInboxHookSeededDeliveryEnvelope, TestSprintInboxAckCommitsRenderedRoleBatchOnly, then panic: test timed out after 10m (sbom_test.go:221 running). CI windows-latest is green only because these skip without umbrella siblings. Repro: bashy dag release-preflight BASHY_COMMIT=<sha> (log on Windows peer release-preflight/run.Sx3l3YkL-windows/workspace/tests.log). Blocks S521.1 fast-loop step 2 (preflight local gate) - fix or storied-date each failure. Surfaced by s514-manager to Sprint 517; moved here as it gates rc.22.
