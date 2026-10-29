---
id: 0e455f197e46
kind: task
title: S521.1t rc.22 native tier smoke on published assets -> tier verdict b5bf7a47
seq: 2831
status: todo
priority: p0
labels:
    - release
created: 2026-10-29T10:09:40.109992Z
sprint: 521
sprint_id: 8215f52e-1496-5a0f-b6f6-729bf3056119
sprint_title: 'Fleet upgrade fix release + ivy deploy: ship outpost upgrade-ordering fix (cd8b98c4) as bashy rc.22, move every host incl. ivy (lern, lj2ivy) onto it once, then promotions auto-flow'
---

Part of S521.1 (bashy rc.22 fast loop rule 6): native tier smoke on the PUBLISHED v1.0.0-rc.22-dev assets, producing the release tier verdict that promote takes as tier_verdict.

Candidate: bashy b5bf7a47d950b18656dfd0b1a96a7e057b49fc4b = tag v1.0.0-rc.22-dev (published, 39 assets incl. checksums.txt).

Do exactly what Sprint 514 R16 did for rc.21 - read the umbrella record docs/sprint-514/tier-smoke-rc21.md first and copy its method:
1. Fetch + sha256-verify the published assets on each host (bashy-darwin-arm64.tar.gz on the macOS peer, bashy-windows-amd64.zip on the Windows peer, bashy-linux-amd64.tar.gz in that peer's WSL as user qa, plus the scratch images bashy-scratch-linux-{arm64,amd64} for tier3).
2. Run umbrella docs/tier-smoke.dag.md tier1, tier2, tier3 natively per OS (OS=darwin|windows|linux, HOST=...), then tier-all-native per OS. Scope native only (tier4/tier5/tier-all-fleet NOT RUN, listed as such, never PASS).
3. Assemble the verdict into bashy release/tier-verdicts/b5bf7a47.json (schema dhnt.tier-smoke/v1, scope native, hosts redacted via TIER_REDACT to macos-host/windows-host/linux-host-wsl) and prove it: `go run ./tools/releaseeligibility --tier-verdict release/tier-verdicts/b5bf7a47.json --commit b5bf7a47d950b18656dfd0b1a96a7e057b49fc4b` -> PASS rc=0.
4. Commit ONLY the verdict file in bashy (your branch; the manager lands it on main and passes its raw URL to promote). Write the per-leaf table (target, OS, verdict, UTC start/finish, run dir) and asset sha256s into your result/commit body; the manager writes the umbrella record.

Hosts: the Windows peer and the macOS peer from ~/.bashy/release-hosts (claimed by s521-manager for this). Remote execution ONLY via outpost ssh / outpost scp (never OpenSSH). One leaf per host at a time; never run two lanes on the Windows peer and its WSL at once. Remove your scratch on every host at the end. Never put real host names or user names in bashy files (OSS). A leaf that does not finish is INCOMPLETE, never PASS - report it, do not paper over it. Do not run anything heavy on the dev box (no full test suites, no containers on the dev box).
