---
id: 94ac7c89208a
kind: task
title: S539.F7 Promote bashy v1.0.0-rc.23 (same bytes) + spot check + announce on mb
seq: 2854
status: todo
priority: p0
labels:
    - release
created: 2026-10-30T07:43:53.807478Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Sprint 539 (rc.23 follow-ups), filed 2026-10-30 so Sprint 525 can close. Operator approved known issues 1, 2, 4 and 5 for bashy v1.0.0-rc.23 (ledger in F0). Release lane (coordinate with s521-manager). After Sprint 525's candidate proofs (R23t, R23c) pass on v1.0.0-rc.23-dev 51271e35: promote via promote.yml version=v1.0.0-rc.23 with a commit-pinned tier_verdict and the F0 ledger. Spot check only: promoted asset sha256 equals the candidate's, and the bashy-scratch :v1.0.0-rc.23 digest equals :51271e35. No re-testing. Post the promoted URL on bashy mb. Then update Dragon's shared bashy install to rc.23, announced to all lanes first; this also heals the dangling goal links on S531/S525.

## Evidence (H1, 2026-11-06)

Promotion receipts audit conducted under Story H1 (#3152, 1a4fe685d0a0) for Sprint 580:

### Receipts Present in Repo
1. **Git Tag `v1.0.0-rc.23`**:
   - Ref: `/usr/bin/git show v1.0.0-rc.23 --stat | head`
   - Resolves to commit `51271e35b8c0bea9dfd31ee6920419be9d0209bd` (`sync: bump yoke pin to 34dab449 (S525.R23a account discover on a fresh host; rc.23)`).
   - Target matches candidate tag `v1.0.0-rc.23-dev` (tag object `cbac1d4e9adf762be2defb161a7b4fdda10ec787` pointing to commit `51271e35b8c0bea9dfd31ee6920419be9d0209bd`).
2. **Tier Verdict Record**:
   - Ref: `release/tier-verdicts/51271e35.json`
   - Recorded: `2026-10-30T07:47:21Z`
   - Schema: `dhnt.tier-smoke/v1`, record: `release-tier-verdict`, sprint: `525`, story: `82681c24b8b0`
   - Release: `v1.0.0-rc.23-dev`, Commit: `51271e35b8c0bea9dfd31ee6920419be9d0209bd`
   - Verdict: `PASS` across all required folds (`tier-all-native-linux`, `tier-all-native-darwin`, `tier-all-native-windows`).
3. **Master Execution Plan Reconciliation**:
   - Ref: `docs/plan-s560-s539-master-execution.md` lines 45, 47–52, 69:
     - Line 45: `| F7 / 2854 / 94ac7c89208a | Verify historical completion and reconcile records | No new promotion or shared-install downgrade |`
     - Lines 47–52: Notes that F7 card note reports rc.23 promoted 2026-10-30 at 09:57:23Z; records that "A promotion timestamp alone does not satisfy every F7 criterion. Before acceptance, retrieve the original candidate/promoted digest comparison, spot check, announcement and installation receipts."
4. **Promotion Pipeline Definition**:
   - Ref: `.github/workflows/promote.yml`
   - Implements byte-promotion of candidate pre-release assets (`dist/bashy-*`, `dist/bash-*`, `dist/outpost-*`, `dist/otel-*`, `dist/checksums.txt`) without rebuilding, retagging `ghcr.io/qiangli/bashy-scratch:${VER}` to candidate digest `:51271e35`, and verifying tier verdict eligibility before promotion.

### Status Determination & Missing Receipts
- **Status remains `todo`**: While the git tag `v1.0.0-rc.23` and `release/tier-verdicts/51271e35.json` prove that commit `51271e35` passed native tier smoke and was tagged as both candidate and release, they do **not** contain asset checksums (SHA256) or container image digests, and thus do not prove that the exact same bytes were promoted.
- **MISSING Receipts**:
  - **Spot check comparison**: Retained record/receipt proving promoted asset SHA256 matches candidate SHA256, and `ghcr.io/qiangli/bashy-scratch:v1.0.0-rc.23` manifest digest equals `:51271e35`.
  - **Announcement receipt**: Confirmation/link of the promoted release URL posted to bashy mb.
  - **Shared install receipt**: Confirmation of the Dragon shared bashy install update to rc.23 announced to all lanes.
