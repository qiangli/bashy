# Plan: `bashy otel serve` — exec the stores, don't link them

*Scope doc, 2026-07-14. No code yet. Grounded in a live investigation, not a guess —
every claim below was checked against the running code or a real store.*

## The problem, exactly

`bashy otel serve` (the `-tags bashy_obs` build) **panics at process init, before
`main()`**, on *every* invocation including `--version`:

```
flag redefined: search.maxQueryLen
```

Root cause, confirmed by building store-subset probes: `search.maxQueryLen` is
registered in an `init()` of a VictoriaMetrics **core library that all three stores
import**. Two stores linked into one binary register it twice → panic. I probed the
pairs: **`vm`+`vl` collides too**, not just `vl`+`vt`. So this is not a two-store
quirk with a clever split — **any two Victoria stores co-linked collide.**

> **Minimum viable topology is three processes, one store each. There is no
> two-process arrangement that links its way out of this.**

This is precisely what the build doctrine already forbids
(`docs/bashy-build-architecture.md`: *engines never linked; exec'd as separate
processes*). The OTel stack violated it by running the stores as in-process
goroutines. The doctrine was right; the stack is the exception that proves it.

## Three findings that make this small

**1. The proxy already speaks HTTP to a localhost port.** Each store component runs
its *own* `httpserver` on `127.0.0.1:<port>` today; the reverse proxy
(`external/otel/stack/proxy.go`, routes in `service.go`) forwards to it by path
prefix. So the wire contract between proxy and store is **already
process-agnostic**. Exec changes *who runs the HTTP server* (a subprocess instead of
a goroutine) and nothing about how the proxy reaches it. **The proxy, the route map,
and the OTLP ingest routes do not change.**

**2. The forks are vanilla.** `external/otel/victorialogs/vlselect` and friends are
**thin re-exports** — `var Init = vlselect.Init` — of upstream, with no bashy
patches. So an **official upstream binary behaves identically.** We do not need to
build or embed a custom store.

**3. Upstream ships per-platform binaries with per-asset SHA-256.** All three repos
(`VictoriaMetrics/{VictoriaMetrics,VictoriaLogs,VictoriaTraces}`) publish
`*-<os>-<arch>-<ver>.tar.gz` + `_checksums.txt`. That is exactly the shape binmgr's
`GitHubSpec` + `resolveChecksum` already consume for `loom`/`zot`/`rclone`. **No
TOFU** — pinned version + published checksum.

## The design

Run the three stores as **downloaded, checksum-verified, supervised subprocesses**,
exactly like `bashy loom` / `bashy zot`. `bashy otel serve` becomes an orchestrator:
resolve → ensure (download+cache+verify) → launch → supervise → run the proxy.

```
bashy otel serve
 ├─ binmgr.Ensure(victoria-metrics @pinned)  → exec  127.0.0.1:8428   (metrics)
 ├─ binmgr.Ensure(victoria-logs    @pinned)  → exec  127.0.0.1:9428   (logs)
 ├─ binmgr.Ensure(victoria-traces  @pinned)  → exec  127.0.0.1:10428  (traces)
 └─ in-process reverse proxy :31415  ── unchanged ──▶ the three ports
```

Each store's config (currently set via `flag.Set("storageDataPath", …)` etc. on the
shared global flag set — itself a symptom of the linking) becomes **argv to the
subprocess**: `--storageDataPath`, `--httpListenAddr=127.0.0.1:<port>`,
`--http.pathPrefix=/<prefix>`. Building that argv per store is the main new code.

### What this deletes

- The three in-process component bodies (`victorialogs.go`, `victoriametrics.go`,
  `victoriatraces.go` — the `Init()`/goroutine/`requestHandler` machinery). They
  collapse to one generic `execStore` component implementing the same `Component`
  interface (`Start`/`Stop`/`Healthy`/`HTTPHandler`), differing only by
  spec+argv+port.
- The thin re-export packages `external/otel/victoria{logs,metrics,traces}/*` — no
  longer imported by anything.
- The heavy Victoria `require`s in `external/otel/go.mod`. **This is where the win
  compounds:** with nothing linking Victoria, the obs binary stops carrying it.

### The bonus: the build tag disappears

The lean build (`obs_stub.go`) **already wires the query verbs** — `failed`,
`guessed`, `bounds`, `cost`, `why-slow` run in the lean worker today. Only `serve`
is gated behind `bashy_obs`, and only because it linked Victoria.

After exec-not-link, `serve` links **nothing heavy** — just `net/http/httputil` and
binmgr, both already in the lean build. So:

> **`bashy_obs`, the `obs_full.go`/`obs_stub.go` split, and `make build-host`'s
> `BASHY_OBS=1` can all be deleted.** `bashy otel serve` joins the lean worker as an
> orchestrator verb, peer to `bashy loom`/`bashy zot`.

The current obs delta is ~22 MB (110 MB obs vs 88 MB lean — the code comment's
"193 MB" is stale, from before the Victoria-only trim). That 22 MB goes to zero, and
the *entire* second build profile for observability goes away.

## The one real decision: download vs embed

| | download (binmgr) — **recommended** | embed (gz blobs, like podman) |
|---|---|---|
| first run | needs network (~3 tar.gz, tens of MB) | offline immediately |
| binary size | lean stays lean | +~60–90 MB on the obs binary |
| matches | `loom`/`zot`/`rclone` (the doctrine's external-tool path) | `podman`/`vfkit` embed path |
| air-gap | fails first run, cached after | always works |

**Recommend download.** The observability stack is an opt-in dev/ops convenience, not
a lifecycle verb — `otel` is not in the `localfirst_test` set, and the philosophy
doc's local-first guarantee is about the *SDLC loop* (issue→weave→gate→judge→dag),
which this is not. Downloading matches the doctrine's default for heavy externals.
Embed stays available as a later `-tags embed_otel` option for a truly self-contained
host image, mirroring the podman embed pattern — but it is not the default and not
this scope.

## Work breakdown

**P1 — version pinning (do first; it gates everything).** Pin each store to a release
tag whose OTLP-native ingest + query paths match what `service.go` and `otelquery`
expect (`/insert/opentelemetry/v1/{logs,traces}`, `/opentelemetry/v1/metrics`,
`/select/logsql/query`). VT is `v0.9.4` in both fork and release — trivial. VM/VL need
a tag chosen and validated (note: VL's fork pins a pseudo-version *above* the real
`v1.x` release tags — the "released tags declare the wrong module path" mess from
`observability.md`; the *binary* wants the real release tag). Deliverable: 3
`binmgr.GitHubSpec`s with `AssetMatch` excluding `-cluster`/`-enterprise` variants.

**P2 — `execStore` component.** One type implementing `Component`, parameterized by
(spec, argv-builder, port, prefix). `Start` = `binmgr.Ensure` then `binmgr.Launch`;
`Healthy` = HTTP GET `/health` (the stores answer it — verified live); `Stop` =
SIGTERM the `binmgr.Process`. Replaces the three hand-written components.

**P3 — argv mapping.** Per store, map today's `flag.Set` calls to CLI flags. Small,
mechanical, one function per store (or a table).

**P4 — supervision.** `stack.go` already models component lifecycle; swap goroutine
components for `execStore`. Decide restart-on-crash policy (binmgr.Process may not
supervise — confirm; if not, add a watch loop like `loom`/`zot` do).

**P5 — delete the linking.** Remove the three in-process components, the re-export
packages, and the Victoria `require`s from `external/otel/go.mod`. Collapse
`obs_full.go`/`obs_stub.go` into one path; drop `bashy_obs` and `BASHY_OBS=1`.

**P6 — the acceptance test is already written.** The live verification from the otel
end-to-end session *is* the gate: `bashy otel serve` starts, three subprocesses come
up, bashy emits its own spans, and `otel failed`/`guessed`/`bounds` return real rows.
Add two regressions: (a) `bashy otel serve --version` does not panic (the literal
bug), (b) a link-level assertion that the binary no longer imports Victoria (e.g.
`go list -deps` in a test, or a size ceiling).

## Risks / edges

- **First-run network.** Mitigate: parallel download, cached after, clear progress,
  a legible error when offline-and-uncached. Same posture as `loom`/`zot`.
- **Version/API drift.** Pinning (P1) fixes it; the acceptance test catches a bad
  pin because the query verbs go empty. Do **not** track `latest`.
- **Checksum trust.** Use binmgr's published-checksum path, not TOFU. Optionally pin
  known-good SHAs in the spec for defense-in-depth (the security plan flags binmgr
  TOFU generally — this is strictly better than the loom/zot status quo, not worse).
- **Windows.** Obs was already excluded there; downloads are cross-platform but keep
  the exclusion until validated. Not this scope.
- **Port contention.** `ports.go` already allocates/checks — unchanged.
- **VictoriaTraces maturity (v0.9.4).** Early; the pin is exact and the container
  image (same artifact) is already proven in the end-to-end test.

## Effort

Small-to-medium, and front-loaded on P1 (get the pins right) and P4 (supervision
policy). The interfaces (`Component`, binmgr `GitHubSpec`/`Ensure`/`Launch`, the
proxy) all exist and are unchanged; this is mostly **deletion plus one generic
component**. The payoff is outsized: the panic is gone, ~22 MB and an entire build
profile disappear, and the stack finally obeys the doctrine it was the sole
violator of.
