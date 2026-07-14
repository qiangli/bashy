# Observability

*Shipped 2026-07-14. bashy emits, and the stack that receives it is Victoria-only.*

> **2026-07-14, end-to-end verification against a live stack — read this first.**
> The claims below ("bashy emits", "ycode records", the query verbs) were written
> before anyone ran the whole path against a real store. Doing so found three bugs,
> two of which meant the plane did not actually work:
>
> 1. **`bashy otel serve` panics at init.** VictoriaLogs and VictoriaTraces both link
>    `app/.../logsql`, which registers the same global flag (`search.maxQueryLen`), so
>    linking two Victoria stores into one process collides *before `main()`*. The
>    in-process stack cannot start today. This is exactly what the build doctrine
>    (`docs/bashy-build-architecture.md`: *engines never linked; exec'd as separate
>    processes*) warns against. **Unfixed — needs the exec-not-link redesign.** The
>    verification below ran the three stores as separate containers behind a
>    route-mirroring proxy to get around it.
> 2. **The query default pointed at a store's own port** (`8428`), not the proxy
>    (`31415`), so every verb 404'd out of the box. *Fixed* (coreutils `ea0cc46`).
> 3. **Every trace verb returned a false "0 matches."** The verbs queried a flat
>    attribute schema; the real VictoriaTraces prefixes everything
>    (`span_attr:`, `resource_attr:`, `event:event_attr:<name>:<idx>`). *Fixed and
>    now verified live* (coreutils `0c79aca`, ycode `d20df52`).
>
> After the fixes, bashy emitting its own spans through the proxy:
> `otel failed → 1x ls exit 2 duration=67ms`; `otel guessed → 1x GUESS-default-rate
> context.tokens amount=6482`; `otel bounds → 1x iterations limit=25 actual=25`; and
> ycode read 6 spans back with `cmd.exit_code` reachable by its bare name.
>
> The lesson is the doc's own thesis turned on the doc: **a telemetry plane nobody
> queried end-to-end is a plane that reports "0 matches" and means "I never looked."**

Two halves, and until this date only one existed:

| | before | after |
|---|---|---|
| **the stack** (`bashy otel`) | collector + Jaeger + Prometheus + Perses + Alertmanager, **286 MB** | VictoriaTraces + VictoriaLogs + VictoriaMetrics, **109 MB** |
| **the emit side** | **nothing** | bashy + ycode, all three signals |

**bashy could RUN an observability stack and fed it nothing.** A collector with no data.
The umbrella's own trace-propagation contract lists `service.name` for `ycode`,
`cloudbox-hub`, `outpost`, `loom` and `act_runner` — and **not for bashy**. The foundation
of the whole stack was the one tier that was invisible.

---

## What it records, and why those things

Not "instrument everything." Six hours of debugging produced seven bugs, and **every one
was invisible in the same way** — a number used without saying where it came from, or a
limit hit without saying so. See `docs/absence-of-evidence.md`.

So there are exactly two primitives, and they are the two that would have caught all seven.

### Provenance — the number, next to WHERE IT CAME FROM

```go
telemetry.Provenance(ctx, "context.tokens", 6482, "provider")
```

`6482` tells you nothing. **`6482 from the provider`** says the context gate is running on
fact. **`6482 estimated`** says it is running on a guess.

That difference *is* the bug: `MeasureTokens` was reading `ConversationMessage.Usage` out
of `[]api.Message` — a type with **no `Usage` field** — so it returned `nil` on every turn
and the whole *"ask the provider, do not guess"* mechanism fell back to the estimator it
exists to replace. Silently. For every model.

The only reason it was caught is a log line printing `from_provider=false` next to the
count. **As a log line that took a human staring at stderr. As a span attribute it is a
query:** *"show me every turn where the gate ran on a guess."*

Emitted today for: `context.tokens`, `context.window` (source: `capabilities-table` — a
hardcoded table, which is itself the next latent bug, now visible without anyone having to
remember it), and `llm.cost.micros` (source: `pricing-table` or **`GUESS-default-rate`**).

### BoundHit — a limit records when it BINDS

```go
telemetry.BoundHit(ctx, "iterations", 25, 25, "the agent had not finished")
```

**A bound you cannot see is not a bound, it is a trap.** Emitted for:

| bound | what it did before it was visible |
|---|---|
| `iterations` (25) | cut an agent off mid-investigation **twice**, exited 0 both times |
| `iterations` (15) | subagent cap — **discarded a delegate's findings** |
| `rate_limit` | three 429s, all recovered, **left no signal at all** |
| `bytes` (256KB) | the last unconditional cut in the system |

**Especially when the run recovers.** A bound that binds and recovers is the one nobody
investigates until it stops recovering.

### Plus one span per command

At the ExecHandler chokepoint — the same seam the audit log and the space-time advisor
already share, so it sees the userland, the PATH fallbacks and every agent-issued command
alike, with **no per-command wiring**.

`cmd.name`, `cmd.argv` (redacted), `cmd.cwd`, `cmd.duration_ms`, `agent.principal`, and
**`cmd.exit_code`** — because *"all three harnesses exit 0 when they fail"* is this repo's
own finding (`docs/harness-ab-deepseek.md`), and then ycode did it too.

---

## Configuration

**Pure standard OTEL env vars.** Nothing bespoke.

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318   # unset = TOTAL no-op
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf           # or "grpc"
OTEL_SERVICE_NAME=bashy
```

**Unset endpoint is a complete no-op** — no exporter, no batcher, no goroutine, no cost. A
shell that pays for telemetry it is not exporting is a shell nobody uses. The global
propagator still installs, so the wire-format contract holds without a collector.

`cmd/bash` — the pure Bash 5.3 drop-in — links **none** of it. Verified: 0 telemetry deps.
*A drop-in that dials a collector is not a drop-in.*

**ycode ignored `OTEL_EXPORTER_OTLP_ENDPOINT` entirely** until this date, reading only a
bespoke `settings.json` field. So setting the standard variable — what anyone would do, and
what every doc says to do — exported **nothing. Silently.** Its telemetry package is large,
complete, and had been effectively unreachable in practice.

---

## The stack: 286 MB → 109 MB (−61%)

Measured **before** cutting. Transitive dependency count per component:

| component | deps | fate |
|---|---|---|
| jaeger | **2,240** | → VictoriaTraces |
| perses | **1,478** | → vmui (already in the binary) |
| collector | 833 | → **the proxy, in three map entries** |
| prometheus | 556 | → VictoriaMetrics |
| **victorialogs** | **113** | *the thing that actually stores the telemetry* |

**Jaeger cost twenty times the weight of the store it fronted.**

**Why the collector could not be deleted alone:** it received OTLP and handed metrics to a
`prometheusexporter`, which Prometheus **scraped**. Delete the collector and you delete the
only path metrics had. Both fall together.

**And then it has no job left.** Every Victoria component ingests OTLP **natively**:

```
/insert/opentelemetry/v1/traces   VictoriaTraces
/insert/opentelemetry/v1/logs     VictoriaLogs
/opentelemetry/v1/metrics         VictoriaMetrics
```

OTLP/HTTP is defined as `POST {endpoint}/v1/{traces,logs,metrics}`, and the stack's proxy
already reverse-proxies by path prefix. **So the proxy IS the fan-out.** 833 transitive
dependencies existed to be one address that forwards three signals to three stores.

> **A middleman that only forwards is a dependency, not a feature.**

### The fork that blocked it had zero commits

VictoriaTraces would not compile against `replace VictoriaLogs => qiangli/VictoriaLogs`.
It looked like a real blocker. It was not: **the fork has no commits of its own** —
`merge-base HEAD upstream/master` equals HEAD, and the diff is empty. A pure mirror, pinned
three months stale, **patching nothing**.

It existed only because VictoriaLogs' *released tags* (v1.113–v1.121) point at commits whose
`go.mod` still declared the **VictoriaMetrics** module path — so the tags are unusable as a
Go module and only a pseudo-version resolves. Requiring upstream **by commit** does the same
job with no fork.

> **A dependency you cannot explain is a dependency you cannot maintain.**

---

## What ycode records

| signal | what | store |
|---|---|---|
| **logs** | `LogConversation` — the full system prompt, messages, response bodies | VictoriaLogs |
| **metrics** | `LLMCallDuration`, `LLMTokens{Input,Output,CacheRead,CacheWrite}`, `LLMCostDollars` (+ `pricing.known`) | VictoriaMetrics |
| **traces** | `turn_with_recovery`, `HTTP POST`, `tools.execute`, `exec <cmd>` | VictoriaTraces |

**Cost carries `pricing.known`.** The pricing fallback is `$3/$15 per million` — Claude
Sonnet's rate — so a model the table had never heard of was billed like a frontier Anthropic
model. GLM-5.2 was reported at **5.5× its real cost**, and on a flat-rate coding plan its
marginal cost is **zero**. A dashboard would have shown the cheapest model in the fleet as
the most expensive one.

> **A number that looks like a fact and is not one is worse than a missing number.**
