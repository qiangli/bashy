# Ollama bashy build

This declaration builds the latest stable upstream Ollama release, `v0.40.3`
(`eab97e9f92b9a25c2d52d2cc6c1b1c99bd9fae21`), for Linux and macOS on amd64
and arm64. GitHub reports that release as neither draft nor prerelease, and
`git ls-remote --tags https://github.com/ollama/ollama.git refs/tags/v0.40.3`
resolves the tag to that commit. The [license at the tag](https://github.com/ollama/ollama/blob/v0.40.3/LICENSE)
is MIT.

## Build and archive layout

The Go binary alone is not runnable for inference. Each `.tgz` therefore uses
the standard install layout that Ollama's own runtime searches:

```text
bin/ollama
lib/ollama/llama-server
lib/ollama/llama-quantize
lib/ollama/<ggml libraries, CPU variants, dependency/project licenses, and Metal payloads>
```

Linux follows the CPU portion of the tag's `Dockerfile`: the upstream
`llama/server` `cpu` CMake preset builds and installs llama.cpp/ggml, then the
Go command is built with cgo, PIE mode and the release version/mode stamps.
Darwin invokes the tag's own `scripts/build_darwin.sh build` for each selected
architecture and archives its per-architecture `dist/darwin-ARCH` output.
The arm64 leg uses upstream release.yaml's `macos-26-xlarge` runner because
that script builds Metal v3 and v4 and explicitly requires the macOS 26.2 SDK
for v4; the amd64 Metal v3 leg runs on `macos-15-intel`.
The build verifies the executable and both llama.cpp helpers after extracting
the final archive; the no-egress gate then proves that archive can start
`ollama serve` and answer `/api/version` and `/api/tags`.

## Accelerator scope

| platform | included | deferred |
|---|---|---|
| Linux amd64/arm64 | CPU runner and all CPU variants selected by upstream's `cpu` preset | CUDA 12/13, ROCm 7.2, Vulkan, JetPack and MLX CUDA. Upstream builds these as independent, very large toolchain/payload lanes; they are not required for a working CPU service and should become separately signed additive assets rather than inflate the base archive. |
| Darwin arm64/amd64 | CPU llama.cpp runner plus the Metal MLX payload produced by upstream's release script (`metal_v3`, and `metal_v4` where the SDK supports it) | CUDA/ROCm/Vulkan are not upstream Darwin release backends. |

## Network and privacy knobs

| setting | value | evidence at `v0.40.3` |
|---|---|---|
| `OLLAMA_NO_CLOUD` | `1` | [`envconfig/config.go`](https://github.com/ollama/ollama/blob/v0.40.3/envconfig/config.go) defines it as disabling remote inference and web search. |
| `OLLAMA_NOHISTORY` | `1` | The same envconfig file defines it as disabling readline history. |
| OTel | disabled; loopback endpoint reserved | This tag does not initialize an OTel SDK. `otel.env` uses the repository-standard disabled exporters and `127.0.0.1:31415` endpoint so a future SDK addition remains fail-closed. |
| updates / telemetry | not applicable to this payload | Upstream's updater is under `app/updater`; these archives build the server/client command at repository root, not the desktop app. No separate update or telemetry knob is exposed by this tag's server envconfig. |

The Linux no-egress verdict is intentionally the shared AF_INET-only rule:
`strace -f` captures `connect`, `sendto`, `sendmsg`, and `sendmmsg`, and only
non-loopback AF_INET/AF_INET6 destinations fail. Local AF_UNIX, AF_NETLINK, and
AF_UNSPEC activity does not count as internet egress. macOS runs the same
server/probes/idle sequence under `sandbox-exec` with non-loopback outbound
traffic denied.

## Audit and install handoff

The shared Go audit runs source-mode govulncheck. `vuln-ignore` has no entries:
every reachable finding fails. The tag's Go 1.26.0 and original module graph
have reachable findings in the current database, so the PIN uses the shared,
fail-closed security mechanism:

- `GO_TOOLCHAIN_SECURITY=1.26.9` stays within upstream's declared 1.26 line
  while taking its standard-library security fixes.
- `GO_SECURITY_OVERRIDES` upgrades `x/crypto`, `x/image`, `x/net`, and `x/text`
  to their fixed releases, plus the exact `x/mod`, `x/sync`, `x/sys`, and
  `x/term` versions those releases require.

The shared gate rejects any other `go.mod` change and uses the same overridden
tree for build, audit, and SBOM. Source-mode govulncheck against that exact
graph reports zero reachable findings; a reachable finding is never waived.

After `builds/render.sh ollama && builds/check.sh` is green, the manager copies
`fork/` to the orphan `bashy-build` branch of `qiangli/ollama`, pushes a
`bashy-v0.40.3-1` tag, waits for all four archives to be signed, and only then
pins the matching archive plus Sigstore identity in binmgr. This story does not
push, tag, change binmgr, or edit yoke.
