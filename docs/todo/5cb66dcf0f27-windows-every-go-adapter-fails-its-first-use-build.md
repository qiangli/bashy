---
id: 5cb66dcf0f27
kind: bug
title: 'Windows: every Go adapter fails its first-use build — the adapter cache root reaches go.exe in MSYS form'
seq: 3305
status: todo
priority: p0
created: 2026-11-23T06:20:22.04495Z
sprint: 692
---

Root cause (rc.40 tier verdict 6c36c0a7, Windows lane, run by claude-opus5.5): every Go
adapter script builds into `${BASHY_ADAPTER_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/bashy/adapters}`
and hands that path to native `go.exe build -o`. With no Windows HOME the shell exposes
`HOME=/c/Users/<user>`; go.exe roots that on the current drive (`C:\c\Users\...`), the
script's `mv` cannot stat the output and the verb fails (mktemp, jq, seq, dag, weave,
sprint, sdlc, loom, oci, curl). Class: product. The 3-OS dispatch e2e missed it because
the test exports a native `BASHY_ADAPTER_CACHE` itself.

Fix: bashy 802c5f40 — `appendAdapterEnv` exports `BASHY_ADAPTER_CACHE` in drive form with
forward slashes on Windows (override kept, respelled only when MSYS-form; XDG_CACHE_HOME
honoured; other hosts unchanged). Unit tests `TestAdapterCacheExportWindowsUsesDriveForm`,
`TestAppendAdapterEnvExportsCacheOnWindowsOnly`.

Proof 2026-11-23 on the Windows lane: a windows/amd64 cross-build of 802c5f40 from the
official image on the gate host, no cache override, adapter cache cleared: `mktemp`,
`jq --version`, `seq 1 3` each cold-build and run with rc=0; `mktemp.exe`, `jq.exe`,
`seq.exe` present under the user's `.cache/bashy/adapters/optional`.

Not in rc.40: the published v1.0.0-rc.40-dev carries the defect (release notes record the
workaround); the next candidate ships the fix.
