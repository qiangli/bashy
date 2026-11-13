---
id: 32566c96a2b2
kind: bug
title: 'layer gate red: bashy_core / bashy_cert_base import graph contains optional yoke pkg/boundary and pkg/outpostagent (TestCoreImportBoundary)'
seq: 3230
status: todo
priority: p1
labels:
    - layers
    - imports
    - gate
created: 2026-11-13T11:21:34.215222Z
---

Found by the 2026-11-13 targeted sweep (umbrella script/layer-gate.sh -> GOWORK=off go test -tags bashy_core|bashy_cert_base ./cmd/bashy -run CoreImport): core_imports_test.go:66 'bashy_core import graph contains optional package github.com/qiangli/yoke/pkg/boundary' and '.../pkg/outpostagent' (same for bashy_cert_base).
PRE-EXISTING, not a Sprint 626/627/629 regression: reproduced identically at the pre-sprint baseline bashy 6cb2fd85 (yoke dc698f7c, 2026-11-12 13:51 PST).
Introduced via yoke packages that bashy core reaches transitively: pkg/accounts/adapters/cloudbox and pkg/broker/share.go (8d294b62, 11-09, weave #37 'outpost: prefer canonical portal URLs' -> pkg/outpostagent), pkg/broker broker.go/peershare.go (7f4bad8c, 11-10, weave #40 'broker: gate member-bound peers at request time' -> pkg/boundary), pkg/fleet group_cmd.go / satellite_admit.go (93a4ed01, bf913cba, 11-12).
Fix direction (root cause first, then fix where it lives): either move the boundary/outpostagent dependency behind the optional-initialization seam so core does not import them, or reclassify the packages if they are genuinely core — never widen the allowlist to silence the gate. Verify: script/layer-gate.sh PASS for bashy; no ratchet allowlist growth.
