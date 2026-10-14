---
id: 14e9a668021c
kind: bug
title: 'kopia darwin no-egress: server never listens (likely Keychain credential persistence on headless macOS); dump server log on failure'
seq: 1904
status: done
priority: p1
labels:
    - supply-chain
created: 2026-10-13T02:39:02.12758Z
assignee: codex-gpt5.6-sol
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T02:56:09.014862Z
closed_by: s434-manager
---

bashy builds/kopia only (do not edit builds/_shared). qiangli/kopia run 38336217370 (tag bashy-v0.23.1-2): pin, audit+SBOM and BOTH linux no-egress legs GREEN; darwin-arm64 and darwin-amd64 fail the no-egress gate: `repository create` succeeds but the server never listens on 127.0.0.1:15151, and the script prints no server log.
1. builds/kopia/files/no-egress-test.sh: on any failure dump the server's stdout/stderr (and the repo create output) so CI shows the cause.
2. Likely macOS-only cause: kopia persists the repository password to the macOS Keychain by default, which blocks headless runners. At the pinned tag v0.23.1 check the flags (cite source): --no-persist-credentials on repository create/connect, KOPIA_PASSWORD env for the server, --without-password / --insecure as yoke uses. Fix the darwin path so the server starts headless with no Keychain access; keep KOPIA_CHECK_FOR_UPDATES=false and --otlp-trace global; the sandbox-exec verdict must still deny all non-loopback egress.
3. Reproduce locally on novidesign.local (macOS, 100G free now; first Go build is slow - cache was cleared) with a binary built from the pinned commit (go1.26.9) and the rendered gate script; paste the passing run. Never run it on Dragon with containers.
4. Report (do not change yoke): whether yoke external/kopia's repository create/server path on macOS would hit the same Keychain persistence (yoke a089a6db) - one line with the code reference.
Gate: builds/render.sh kopia && builds/check.sh exit 0. Commit with trailers; sprint submit with SHA + gate + the novidesign gate output. Do NOT push to qiangli/* or tag.
