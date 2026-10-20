# Local inference and sphere sharding: recovery map

Read this before changing the bashy inference engines, the ollama build, the
fleet service defaults, or outpost sharding. It records what the feature is,
how it was lost once, how it was restored, and how to restore it again
without starting from scratch.

The source of this file is `bashy/builds/ollama/files/RECOVERY.md`. The
rendered copy lives on the `bashy-build` branch of `qiangli/ollama` as
`bashy-build/RECOVERY.md`. Edit only the source.

## The feature

**A freshly paired host runs local inference, and two or more paired hosts
form a cross-host shard, out of the box, using only bashy-built, signed
engines.**

Out of the box means no operator configuration after pairing: the service
units default on, the engines are provisioned and verified automatically,
and `outpost shard trigger <leader> <model>` forms the ring.

## Lesson learned (2026-10)

The feature worked in mid-2026 and was then lost **silently, piece by piece,
while other work was being added**. No single change removed it:

- refactors moved code but not the build script that produced its binary;
- a lean-build change made the engines opt-in and nobody re-enabled them in
  the release;
- a "retained surfaces become service units" change declared the sphere units
  `enabled: false` with no compatibility reader for the old settings;
- a port change moved the engine but sharding still hardcoded the old port.

Every step passed its own tests. Nothing tested the end-to-end behaviour, so
the loss was found months later, by an operator, from memory. Recovery then
took a sprint, much of it spent on adjacent work instead of the feature.

Rules that follow from this:

1. **The end-to-end behaviour is the test.** A change touching any component
   below is not done until the proof in [Verify](#verify-the-feature) passes
   on two paired hosts.
2. **Moving code means moving its build.** A package that embeds a binary
   moves together with the script and release wiring that produce the
   binary.
3. **Defaults are features.** Changing a default for a sphere unit needs a
   migration of the old setting and a regression test of a freshly paired
   host.
4. **Restore first, then harden.** When the feature is lost, restore it from
   the last known-good history below; do not redesign during recovery.

## Components

| Piece | Where | What it does |
|---|---|---|
| Service units | yoke `pkg/fleet/baseline/services/{ollama,ollama-pool,shard}.yaml` | All three `enabled: true` for paired hosts (yoke `b0f5fd30`, with a regression test of a freshly paired host). |
| Legacy settings migration | outpost `1810aa4` | Old `agent.json` toggles become unit overrides; explicit operator disables persist. |
| Local engine (single host) | this fork: the official Ollama **runner** (llama.cpp based), built and signed by `bashy-build.yml` | `bashy ollama serve` runs the vendored Ollama server (compiled into bashy) and the embedded signed runner. Raw engine port 11435, door port 24556. |
| Runner embedding | yoke `external/ollama/runner_embed` (build tag `embed_runner`, files `ycode-runner.gz` + `ycode-runner.sigstore.json`) | `extract.go` refuses to run until `releaseVersion`, `releaseIdentity` and the per-target `releaseSHA256` (raw executable bytes, not the gzip) are filled from a published signed release. |
| Release wiring | bashy `scripts/fetch-signed-runner.sh`, `release.yml`, `.goreleaser.yaml`, `scripts/build-native-darwin-release.sh` | Downloads the target's runner and Sigstore bundle, verifies identity, issuer and digest, embeds it, and fails the release closed if any target lacks it. |
| Shard engine | first-party `prima.cpp` fork, signed release `bashy-bashy-prima-0.1.2-13`, pinned in yoke binmgr (`609529ce`) | Pipelined ring: rank 0 `llama-server`, other ranks `llama-cli`. Windows ships `libomp.dll`. |
| Shard orchestration | outpost `internal/agent/shard`, `cmd/outpost/provision.go`, `cmd/outpost/shard.go` | Leader elected by capacity; same-LAN ring; `--master/--next 127.0.0.1` by design, because outpost forwards those ports to the peer over the mesh. |
| Engine port resolution | outpost `651191d` | Shard provisioning resolves the engine through the ollama unit or `BashyOllamaURL`; no hardcoded 11434. |
| First form | outpost `d6dc63a` | Provisions signed Prima before the readiness gate, so a fresh host can form a ring. |
| Windows ranks | outpost `c48719d`, `915b665` | Windows server/worker/libomp through binmgr; tracked rank PIDs are reaped before re-provisioning so a held file lock cannot wedge the install. |
| Supply-chain rule | bashy service supply-chain policy | Hosts run only engines built and Sigstore-signed by bashy CI from pinned permissive source. Upstream binaries and PATH fallbacks are refused (yoke `cad0d513`). |

Supported platforms: Linux amd64/arm64, macOS arm64/amd64, Windows amd64.
The shard ring is same-LAN only; WAN peers are excluded by design.

## History

**Worked (ycode era, 2026-04 to 2026-06).** `scripts/build-runner-thin.sh`
ran `go generate` and `CGO_ENABLED=1 go build ./cmd/runner/` on the vendored
Ollama and gzipped the result to `runner_embed/ycode-runner.gz`; the
Makefile added `-tags embed_runner`; release.yml built it for darwin-arm64,
linux-amd64 and linux-arm64 (ycode `6a18bb3`, `75a559a`, `bfc023e`). A 2-rank
cross-host split was proven on 2026-06-28; outpost `a732b03` (2026-07-01)
made cross-host shard formation reliable.

**Lost.**

| Change | Date | Effect |
|---|---|---|
| ycode `ccf6551` / coreutils `3ad95af8` | 2026-06-26 | `runner_embed` moved to coreutils; its build script did not. |
| bashy `15d61d6a` | 2026-06-28 | Engines opt-in; release became the lean cgo-off build; bashy never had `embed_runner`. |
| ycode `c5f4e05` | 2026-06-30 | Deleted `build-runner-thin.sh` while its DAG still called it. |
| ycode `085e7ca` | 2026-07-14 | Removed the runner build from release ("local inference is bashy's domain"); bashy never picked it up. |
| bashy `c2be1cfe` | 2026-09-27 | Engine moved to 11435 behind a door; sharding still hardcoded 11434. |
| yoke `7e52e3a3` | 2026-10-09 | `bashy ollama serve` fell back to a system (third-party) ollama. |
| outpost `e3788fb` + yoke `b18612f7` | 2026-10-11 | Sphere units became baseline units with `enabled: false`, old settings ignored: every upgraded host stopped ollama, pool and shard. |

**Restored (Sprint 436, 2026-10-13/14).** Defaults and migration (yoke
`b0f5fd30`, outpost `1810aa4`); port resolution (outpost `651191d`); signed
Prima on all five platforms (`bashy-bashy-prima-0.1.2-13`, yoke `609529ce`);
Windows ranks and first-form fixes (outpost `c48719d`, `d6dc63a`, `915b665`);
third-party ollama fallback removed in yoke (`cad0d513`) and in bashy's lean
build (`5a6cea0f`: lean `bashy ollama serve` now fails closed instead of
downloading upstream Ollama); signed runner release `bashy-v0.21.2-15`
published after the audit was made to pass honestly (bashy `9b9af63e` through
`08e91b3c`, with reviewed waivers in `vuln-ignore`); yoke pins it (`e2909791`);
the release wiring embeds it per target (bashy `b16980c4`).

**The engines build is required.** Local inference only works in a bashy
built with `-tags bashy_engines,embed_runner` (cgo on darwin/linux). Its
`bashy ollama serve` runs the vendored Ollama server, whose runner re-exec goes
through bashy's dispatch to the extracted, signature- and digest-verified
`ycode-runner`. A lean build has no local engine. The next bashy release must
be the engines build, or local inference regresses for users.

**Known gap: Windows.** `engines_windows.go` never supported the bashy-built
Ollama engine, and the vendored server does not build for Windows with cgo off
(`x/mlxrunner` is cgo/MLX). Windows sharding works through signed Prima; a
Windows local engine is follow-up work, not a regression.

Local inference proof on 2026-10-14 (macOS arm64, engines build of bashy
`5a6cea0f`): door, `bashy ollama serve`, `bashy runner`, then the extracted
`ycode-runner` whose sha256 equals the release pin; `llama3.2:1b` answered;
no upstream Ollama process.

Shard proof on 2026-10-14 (outpost `15fc954`, one macOS arm64 host and one Windows
amd64 host on the same LAN): `outpost shard trigger <mac> llama3.2:1b` formed
a 2-rank ring; the leader answered completions at about 45 tokens/s and the
Windows rank's CPU time rose during each request.

## Verify the feature

On two paired hosts on the same LAN, after installing the candidate:

```bash
outpost services            # ollama, ollama-pool, shard: enabled + active
outpost pool status         # Ollama on: yes, Pool on: yes, engine reachable
outpost shard status        # server_bin=true worker_bin=true ring_members>=2
outpost shard trigger <leader-host> <model>   # model architecture Prima supports, e.g. llama3.2:1b
outpost shard status        # active_model=<model> on both hosts
curl -s http://127.0.0.1:11434/completion -d '{"prompt":"The capital of France is","n_predict":12}'
```

Run the last command on the leader. Local inference on each host must work
with no third-party ollama installed or running.

Known traps:

- `qwen3` architectures are not supported by Prima; use a llama-family model.
- A rank orphaned by a pre-`915b665` outpost is not tracked and holds the
  Windows binary lock after an upgrade (`rename ... Access is denied`). Stop
  that exact PID once; later ranks are tracked.
- The leader answers `503 Loading model` until every rank has joined; a rank
  that never starts leaves it waiting, so check the worker host's processes,
  not its last log.

## If it is lost again

1. Run the verification above and record exactly which step fails.
2. Diff the components table against current source: unit defaults, engine
   port resolution, runner embedding pins, release embedding, Prima pin.
3. Restore from the commits listed here; reuse the signed releases if their
   pins still verify. Do not rebuild from scratch and do not redesign.
4. Add the missing regression test next to the restored piece.
