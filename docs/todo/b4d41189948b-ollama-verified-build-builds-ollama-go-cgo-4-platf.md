---
id: b4d41189948b
kind: feature
title: 'Ollama verified build: builds/ollama (Go+cgo, 4 platforms, signed) from latest stable upstream tag'
seq: 1898
status: done
priority: p1
labels:
    - supply-chain
created: 2026-10-12T21:45:36.916872Z
weave: 61
assignee: codex-gpt5.6-sol
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T04:05:43.096693Z
closed_by: s434-manager
---

Conductor decision (operator delegated) for 3054b43b: option (c) then (a). yoke 6f38e3d4 (removes ollama download + PATH fallback) stays UNMERGED until a bashy-signed ollama build is green and binmgr installs it; then both land in the same yoke pin.

This story: add ollama (MIT) to bashy builds/ (see builds/README.md "adding a tool"; Go kind in builds/_shared/kinds/go, tuwunel/victoria declarations as examples).
- Fork qiangli/ollama (the conductor creates it; FORK_REPO=qiangli/ollama), pinned to the LATEST STABLE upstream tag of github.com/ollama/ollama: verify tag commit via git ls-remote, LICENSE MIT at tag.
- Go + cgo build as upstream's release does (llama.cpp / ggml backends via upstream's own build: CMake presets / scripts/build_*.sh), 4 platforms: linux-amd64, linux-arm64, darwin-arm64, darwin-amd64. CPU + Metal (darwin) minimum; record which GPU backends (CUDA/ROCm/Vulkan) are included or deferred and why. Assets must include everything `ollama serve` needs at runtime (lib/ollama runners) - prefer upstream's archive layout (tgz) so binmgr can extract it.
- Reuse kinds/go fragments; override build.yml for the cgo/cmake part (like gitea overrides build). govulncheck: reachable = fail, security-only go get overrides allowed (same mechanism as the Victoria story if it lands first), NO waivers for reachable.
- no-tracking.env: OLLAMA_NO_CLOUD=1 and any update/telemetry knobs (cite upstream envconfig at the tag); otel.env as other tools.
- no-egress test: start `ollama serve` on 127.0.0.1, hit /api/version, /api/tags, idle; strace verdict counts only AF_INET/AF_INET6 non-loopback (tuwunel fb22eaf9 logic / shared copy).
- Gate: builds/render.sh ollama && builds/check.sh exit 0. Commit with trailers, sprint submit with SHA + gate output. Do NOT push to qiangli/* or tag (manager pushes). Do not edit yoke.
