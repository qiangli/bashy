---
id: 02288146478d
kind: feature
title: 'builds: headlamp signed server binaries + OCI image (kubernetes-sigs/headlamp latest stable)'
seq: 1901
status: done
priority: p2
labels:
    - supply-chain
created: 2026-10-13T00:04:00.396947Z
assignee: codex-gpt5.6-sol
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T01:18:31.555161Z
closed_by: s434-manager
---

START FROM the previous attempt: commit b2976d73 on local branch s434/headlamp-b2976d73 of the bashy checkout (/Users/qiangli/projects/poc/dhnt/bashy) - git fetch that path s434/headlamp-b2976d73, cherry-pick it onto your base, then rework.

REWORK of 02288146 (your bashy b2976d73, branch agent/weave-issue-65, NOT merged). Rebase onto bashy main (24680fb5 or newer): 88f90c8f landed the shared GO_SECURITY_OVERRIDES step and render.sh now REFUSES it on KIND=custom: "render: headlamp: GO_SECURITY_OVERRIDES is a go-kind key, but KIND=custom".
Fix (prefer A): (A) KIND=go and override only the fragments headlamp needs (build.yml for frontend+server+image; keep the shared go setup/audit so the shared override step, its go.mod gate and the audit-tool toolchain apply), like builds/gitea overrides build.yml. (B) only if A is impossible: stay custom and drop GO_SECURITY_OVERRIDES, applying nothing - then govulncheck must still fail for anything reachable. Do NOT edit builds/_shared (claude-opus5 owns it in a60d93b4; if you need a shared change, say so in submit).
Conductor decision stands (option a): CI must keep refusing to sign while GO-2026-5064/5338/5622/5932 are reachable with no fix; record in README/PIN as blocked-on-upstream with the 4 IDs. No waivers.
Gate: builds/render.sh headlamp && builds/check.sh exit 0 on the rebased tree. Commit with trailers (same Story-ID 02288146478d), sprint submit with SHA + gate.

--- original brief ---
bashy builds/headlamp only (do NOT edit builds/_shared; KIND=custom with your own fragments, like gitea overrides build.yml).
outpost today deploys headlamp.DefaultImage = ghcr.io/headlamp-k8s/headlamp:v0.27.0 (third-party image; audit row docs/bashy-supply-chain-audit.md in umbrella). Upstream moved to github.com/kubernetes-sigs/headlamp (Apache-2.0, latest stable v0.45.0 - verify).
Deliver a bashy-signed headlamp build from the pinned tag: the in-cluster server (Go backend + built frontend, upstream's own Makefile/Dockerfile steps) as (a) raw server binaries linux amd64/arm64 + darwin arm64/amd64 if upstream supports, and (b) an OCI image built from those binaries on a minimal base, pushed to ghcr.io/qiangli/headlamp:<bashy tag> and signed keylessly (cosign sign by digest, same identity shape as the other forks; record the digest). Analytics/telemetry off (verify upstream flags at the tag, e.g. plugin/analytics settings) and no-egress test (shared egress-verdict.sh logic copied into builds/headlamp/files if you cannot source _shared; note it). FORK_REPO=qiangli/headlamp. Free standard runners only. govulncheck reachable = fail (GO_TOOLCHAIN_SECURITY_OVERRIDE exists; module overrides being added by another story - declare GO_SECURITY_OVERRIDES if needed). npm audit for the frontend: high/critical reachable = fail, document.
Gate: builds/render.sh headlamp && builds/check.sh exit 0. Commit with trailers, sprint submit with SHA + gate output. Do NOT push to qiangli/* or tag. Do not edit outpost/yoke (the image flip is a later story).
