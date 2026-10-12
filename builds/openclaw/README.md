# OpenClaw gateway — bashy VERIFIED BUILD

bashy never starts a third-party-built service. This directory holds the
template the mirror fork needs to build the OpenClaw gateway from pinned
upstream source on fork CI, with tracking removed and OpenTelemetry pointed
only at the local collector.

## Where this runs (fork model)

The workflow does NOT live in bashy and NEVER runs here. The manager copies
the whole `fork/` tree as-is at the root of the mirror fork
(`github.com/qiangli/openclaw`) on a `bashy-build` branch:

- `fork/.github/workflows/bashy-build.yml` → the fork's build workflow
- `fork/bashy-build/` → runtime files the workflow needs (PIN, no-tracking
  env, OTel env, gateway config, no-egress gate, mock collector)

Trigger: push of tags matching `bashy-v*` (e.g. `bashy-v2026.9.8-1`) on that
branch. No `workflow_dispatch`. The release is published ON THE FORK with
`GITHUB_TOKEN` (`contents: write`), signed keyless via cosign bundles
(`id-token: write`).

## Pinned upstream (`fork/bashy-build/PIN`)

`openclaw/openclaw` tag `v2026.9.8`, commit
`fc23bc864e4553c2d215e479eeec47b67a0bf943` (MIT licensed). The tag object was
corroborated out-of-band (annotated tag → that commit). The workflow checks
out `refs/tags/v2026.9.8` from upstream and FAILS unless `git rev-parse HEAD`
equals the pinned sha. PIN bumps are deliberate: only official stable release
tags are accepted — the workflow REFUSES `main`/dev branches, pre-releases
and RCs before checkout.

## How to bump the pin

1. Review the upstream diff tag-to-tag (`openclaw/openclaw` compare view);
   pay attention to new network calls, new telemetry knobs, Dockerfile base
   image changes, and `packageManager`/lockfile changes.
2. Update `fork/bashy-build/PIN` (tag + full commit sha), re-verify every
   knob table below against the new tag's docs/source, and update source
   lines.
3. The manager syncs the tree to the fork's `bashy-build` branch and cuts a
   release tag `bashy-v<upstream>-<n>` (e.g. `bashy-v2026.9.8-1`).

## No-tracking knobs

All source lines below are at the pinned tag (`v2026.9.8`).

| Knob | Effect | Upstream source |
|---|---|---|
| `telemetry.enabled: false` (config) | Anonymous feature statistics off (already default-off; forced) | `docs/gateway/telemetry.md` § "Turn anonymous feature statistics on or off" (`telemetry: { enabled: false }`); field ref `docs/gateway/config-observability.md` § Telemetry |
| `DO_NOT_TRACK=1` env | Forces feature statistics off even when `telemetry.enabled` is true. Does NOT stop the update check | `docs/gateway/telemetry.md` ("Set `DO_NOT_TRACK=1` … even when `telemetry.enabled` is `true`" / "`DO_NOT_TRACK` does not disable the daily update check") |
| `update.checkOnStart: false` (config) | Stops both tiers and every automatic update request, even when `update.auto.enabled` is true | `docs/gateway/telemetry.md` § "Disable every automatic update request"; field ref `docs/gateway/config-runtime.md` § Update (`checkOnStart` port list) |
| `OPENCLAW_NO_AUTO_UPDATE=1` env | Also prevents automatic update requests (incl. headless-node policy) | `docs/gateway/telemetry.md` (same section); `docs/gateway/config-runtime.md` § Update |
| `OPENCLAW_TELEMETRY_ENDPOINT=http://127.0.0.1:9/` env | Defense in depth: any residual telemetry request hits a localhost blackhole (discard port), never `https://telemetry.openclaw.ai` | `docs/gateway/telemetry.md` ("set `OPENCLAW_TELEMETRY_ENDPOINT` to your complete replacement endpoint URL") |
| `CI=true` env | Upstream sends nothing (no update check, no statistics) in automated environments | `docs/gateway/telemetry.md` § "Automated environments" |
| `models.catalogRefresh.enabled: false` (config) | Stops hosted model-catalog (metadata + pricing) refresh traffic | `docs/gateway/config-runtime.md` § Models ("use `models.catalogRefresh.enabled: false` when OpenClaw must avoid all hosted catalog traffic") |
| `discovery.mdns.mode: "off"` (config) + `OPENCLAW_DISABLE_BONJOUR=1` env | Suppresses mDNS/Bonjour LAN multicast advertising outright (default mode is `minimal`, which still advertises) | `docs/gateway/config-runtime.md` § Discovery ("`minimal` (default)" / "`off`: suppress LAN multicast advertising" / "`OPENCLAW_DISABLE_BONJOUR=1` disables mDNS advertising outright") |

Endpoint/CI interaction (verified, same doc): configuring
`OPENCLAW_TELEMETRY_ENDPOINT` lifts the CI suppression by design, so the
blackhole alone is not the control — `update.checkOnStart: false` plus
`OPENCLAW_NO_AUTO_UPDATE=1` stop every automatic request, and the blackhole
catches residuals.

Applied via [`fork/bashy-build/no-tracking.env`](fork/bashy-build/no-tracking.env)
(sourced into the build and gate environments) and
[`fork/bashy-build/openclaw-bashy.json`](fork/bashy-build/openclaw-bashy.json)
(gateway config file).

Corrections vs the previous draft: `OPENCLAW_CONFIG_DIR` never existed
upstream and is gone — config reaches the gateway through
`OPENCLAW_CONFIG_PATH` (a file path;
`docs/gateway/configuration.md`: "point `OPENCLAW_CONFIG_PATH` directly at
the real file"). `OPENCLAW_EXTENSIONS` is a docker-build plugin selector, not
a runtime switch — runtime plugin enablement is the `plugins.allow` block
below.

## OTel-local config

OpenClaw exports diagnostics through the official `diagnostics-otel` plugin
over OTLP/HTTP (protobuf); exporters attach only when both the diagnostics
surface and the plugin are enabled, so cost is near zero by default. bashy
enables both and points export ONLY at the local collector.

| Knob | Value | Upstream source |
|---|---|---|
| `plugins.allow: ["diagnostics-otel"]` + `entries.diagnostics-otel.enabled: true` (config) | Loads the exporter plugin | `docs/gateway/opentelemetry/setup.md` § Quick start (`plugins.allow` / `entries`) |
| `diagnostics.enabled: true` + `diagnostics.otel.enabled: true` (config) | Turns on the diagnostics surface and the OTLP pipeline | same doc (Quick start); field ref `docs/gateway/config-observability.md` § Diagnostics (`otel.enabled`, default `false`) |
| `OPENCLAW_EXTENSIONS=diagnostics-otel` (build) | Selects the plugin for source/docker builds (`--build-arg`) | `Dockerfile` (`ARG OPENCLAW_EXTENSIONS`); `docs/install/docker.md` § Observability (`export OPENCLAW_EXTENSIONS="diagnostics-otel"`) |
| `OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318` | Local collector only | `docs/gateway/opentelemetry/configuration.md` (`OTEL_EXPORTER_OTLP_ENDPOINT` fallback for `diagnostics.otel.endpoint`) |
| `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf` | OTLP/HTTP; only `http/protobuf` enables plugin-owned export | same doc (`OTEL_EXPORTER_OTLP_PROTOCOL` row) |
| `OTEL_SERVICE_NAME=openclaw-gateway` | Service name fallback | same doc (`OTEL_SERVICE_NAME` row; default service name is `openclaw`) |
| `diagnostics.otel` in `openclaw-bashy.json` | `enabled: true`, local endpoint, logs off | same doc (`diagnostics.otel.*` field reference) |

Applied via [`fork/bashy-build/otel.env`](fork/bashy-build/otel.env) and the
`plugins` + `diagnostics` blocks in
[`fork/bashy-build/openclaw-bashy.json`](fork/bashy-build/openclaw-bashy.json).

## Gateway invocation (verified at the pinned tag)

| Fact | Value | Upstream source |
|---|---|---|
| Start command | `node openclaw.mjs gateway` (equiv. `openclaw gateway`) | `docs/cli/gateway/running.md` (`openclaw gateway` / `openclaw gateway run # equivalent, explicit form`) |
| Config file | JSON5 at `~/.openclaw/openclaw.json`; `OPENCLAW_CONFIG_PATH` points at the real file | `docs/gateway/configuration.md` |
| Start guard | Refuses unless `gateway.mode=local` | `docs/cli/gateway/running.md` § Startup behavior |
| Port / bind | `gateway.port` default `18789`; `--port` > `OPENCLAW_GATEWAY_PORT` > `gateway.port` > `18789`; bind default loopback | `docs/gateway/config-gateway.md` § field details (`port` precedence); `docs/cli/gateway/running.md` (`--bind`: `loopback` default) |
| Readiness | `/healthz` liveness (answers as soon as the server can); `/readyz` stricter | `docs/cli/gateway/query.md` (`gateway health`) |

The no-egress gate stages `openclaw-bashy.json` (which sets
`gateway.mode: local`, port `18789`, bind `loopback`), exports
`OPENCLAW_CONFIG_PATH` at the file, and polls
`http://127.0.0.1:18789/healthz`.

## No-egress gate (`no-egress-test.sh` + `mock-otlp-collector.py`)

Runs BEFORE signing in the workflow:

1. Positive control: a connect to an unroutable TEST-NET-1 address from
   inside `unshare -rn` (user + network namespace, loopback only) must FAIL —
   otherwise the sandbox proves nothing.
2. Starts the mock OTLP collector (stands in for the local collector)
   and the freshly built gateway in the same isolated namespace under
   `strace -e trace=%network`, with `OPENCLAW_CONFIG_PATH` exported.
3. Waits for `/healthz`, lets it run so startup and maintenance jobs would
   fire, then FAILS on any `connect()` to a non-loopback address.

Defaults (`GATEWAY_ARGV=gateway`, health `http://127.0.0.1:18789/healthz`,
config `bashy-build/openclaw-bashy.json`) are overridable via env.

## What is verified

- PIN tag is an official stable release; upstream checkout at
  `refs/tags/v2026.9.8` equals the pinned commit sha.
- `pnpm install --frozen-lockfile --ignore-scripts` from the upstream
  lockfile (upstream is pnpm-based — `pnpm-lock.yaml`, `packageManager
  pnpm@12.5.1`, no `package-lock.json`; Node 24 matches the upstream
  Dockerfile `node:24-bookworm` base and the supported range in
  `openclaw.mjs`). Lifecycle scripts stay off except the documented
  `build:docker` + pack entrypoints, run explicitly.
- `pnpm audit --audit-level high` gate.
- CycloneDX SBOM published as a release asset.
- No-egress gate passes before signing.
- Every asset keylessly signed: `cosign sign-blob --bundle <asset>.sigstore.json`
  (cosign v3.1.3 via pinned installer) AND `actions/attest` provenance —
  identity is this workflow at the release tag, issuer
  `https://token.actions.githubusercontent.com`.
- Release assets: gateway tarball, CycloneDX SBOM, image digest,
  `openclaw-pin.txt`, `SHA256SUMS`, plus one `.sigstore.json` bundle per asset.

## Sigstore identity for binmgr to pin

```
https://github.com/qiangli/openclaw/.github/workflows/bashy-build.yml@refs/tags/bashy-v2026.9.8-1
```

issuer `https://token.actions.githubusercontent.com`.
