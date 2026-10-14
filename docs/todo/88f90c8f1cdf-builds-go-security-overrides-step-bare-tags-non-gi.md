---
id: 88f90c8f1cdf
kind: feature
title: 'builds: GO_SECURITY_OVERRIDES step, bare tags, non-GitHub upstream; seaweedfs + act_runner declarations'
seq: 1900
status: done
priority: p1
labels:
    - supply-chain
created: 2026-10-13T00:03:58.904881Z
assignee: claude-opus5
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T01:02:17.945405Z
closed_by: s434-manager
---

bashy builds/ - you OWN builds/_shared and builds/render.sh for this story (others edit only builds/<tool>). Rebase on the fail-fast:false change if it lands first.
1. Shared Go security mechanism, module half (contract): tool.env key GO_SECURITY_OVERRIDES="mod@vX.Y.Z ..." optional. A step right after pin verify runs `go get mod@ver` for each in src/ (no go mod tidy); gate: src/go.mod diff may change ONLY require lines of exactly those modules (go.sum free), anything else fails; the same overridden tree feeds audit AND build; listed in PIN and release notes. Empty/absent = no-op (existing forks render byte-identical except the new no-op wiring). Toolchain half already exists: GO_TOOLCHAIN_SECURITY_OVERRIDE (bashy 0607f515). NO waivers for reachable vulns.
2. Bare tags: seaweedfs tags releases X.Y (latest 4.48). Allow a declared tag form so UPSTREAM_TAG=4.48 works in render.sh / verify-pin.sh / pin job; bashy tag = bashy-4.48-N.
3. Non-GitHub upstream: act_runner lives at https://gitea.com/gitea/act_runner (MIT). Support UPSTREAM_REPO on a non-GitHub host: the build checks out the official tag from that URL (git clone --depth 1 --branch <tag> + verify commit) instead of actions/checkout repository:. Keep the pin gate identical.
4. Add declarations: builds/seaweedfs (FORK_REPO=qiangli/seaweedfs, latest stable 4.48 or newer, weed binary, Apache-2.0, no-tracking: -master.telemetry=false etc. per yoke external/seaweedfs + docs/service-runtime-privacy.md in yoke) and builds/act_runner (FORK_REPO=qiangli/act_runner, latest stable tag at gitea.com, MIT). 4 platforms (linux/darwin x amd64/arm64), free standard runners only (ubuntu-24.04, ubuntu-24.04-arm, macos-15, macos-15-intel), shared egress-verdict.sh in their no-egress tests, otel.env 127.0.0.1:31415. Run govulncheck locally per tool; use the overrides, never waivers, for reachable findings.
Gate: builds/check.sh exit 0 (all tools) incl. shellcheck/actionlint. Commit with trailers, sprint submit with SHA + gate + local govulncheck summary. Do NOT push to qiangli/* or tag. Do not edit yoke.
