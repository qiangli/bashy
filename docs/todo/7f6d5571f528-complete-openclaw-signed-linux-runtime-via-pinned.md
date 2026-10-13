---
id: 7f6d5571f528
kind: feature
title: Complete OpenClaw signed Linux runtime via pinned upstream Dockerfile CI
seq: 1930
status: done
priority: p1
created: 2026-10-13T18:27:36.163362Z
assignee: codex-gpt6-sol
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T20:06:49.342735Z
closed_by: s434-muse-manager
---

Parent380ef55d transferred by operator to S434. Plan: upstream OpenClaw v2026.10.2 b8952d477f0c932d8ddc6da5c89cab5a169b6ff1 Dockerfile in own CI, free hosted runner disk first, signed image/extracted Linux runtime tree, bundled http-cache-semantics4.3.0 security patch, production audit+SBOM, whole-image scan, no-egress and exact runtime identity gates. Existing source14186f25 integrated via297c35dd; preserve current shared pipeline/S436 changes. Review prepared fork b48e136 (local scratch /tmp/s433-openclaw-bashy-build) against generated openclaw fork; expected next immutable tag bashy-v2026.10.2-17, confirm remote first. Prior local proof already used approved VM60GiB/16GiB and failed finalCOPY ENOSPC; do NOT resize or retry local build. Prefer hosted CI after focused validation. Worker owns all needed OpenClaw-specific code commits; commit with this story trailers, no main push/shared-pipeline changes without manager sequence. Send reviewed exact source/fork diff and proposed tag to manager for integration/publication; manager has operator authorization to publish. No metered runners; macOS may be explicitly deferred, Linux required. No upstream third-party runtime fallback, no vulnerability waivers, no private hostnames/source in public notes. No containers/heavy builds on development machine. ACK manager, progress3m, bounded20m initial review; signing/release success only from actual artifacts/gates.
