# Plan — distributed chunked execution (the bashy uplift)

Status: **plan of record, 2026-07-12.** Approved. Supersedes the ordering in
`chunked-fleet-conformance-plan.md` (see §Docs to amend).

## Context

**The product is a general chunked/distributed task runner.** `bashy dag` grows from a
one-box task graph into a runner that takes any workload with a large item corpus —
pipeline, workflow, HPC sweep, ML training, benchmark, reference run — splits it into
chunks, and runs them in parallel across the machines you own.

**The conformance matrix is the validation case, not the goal.** It is the workload we
already have whose correctness we can check *exactly*: a chunked result must equal the
serial result, case for case. That makes it the right first dogfood and a permanent
regression test for the runner itself.

**MVP target:** the whole GNU-bash/POSIX conformance + compliance matrix in **minutes, not
hours**, run chunked and distributed across a real fleet, from one command.

**Design rule:** the *architecture* accommodates every axis below; the *implementation*
ships only what the dogfood needs. Each axis is a declared seam with exactly one
implementation today.

---

## Ground truth (measured 2026-07-12 — trust over any doc, including this one's ancestors)

1. **The CI conformance gate has not actually run for ~10 merges.** `bash53-gate` burns
   exactly its `CI_GATE_DEADLINE=1200` every run: fixture `execscript` infinite-loops on
   Linux/glibc (0.87s on macOS). `.github/workflows/test.yml:79 continue-on-error: true`
   reports it green. **20 of CI's 20.7 min is that hang.** Fixing it needs no distribution.
2. **bash-5.3 is not the slow suite.** 87 fixtures, **278.6s serial (4.6 min)**. Three are
   83% of it: `jobs` **112.4s** (real work — an indivisible **atom**, a hard makespan
   floor), `trap` **60.05s** and `coproc` **60.04s** (both are *timeouts*, not work). The
   other 80 total ~46s.
3. **The hours are in the other suites, and the cost is environment construction, not test
   execution** — container image builds (multishell builds two, incl. a Debian `apt-get`),
   runtime clones of upstream suites (yash/zsh/oils — deliberate, for GPL cleanliness), and
   one Rust compile (uutils). Nine suites have **zero** recorded timings.
4. **The container lane may delete the timeout waste for free.** `coproc` = **3.02s** in the
   Linux container vs 60.04s (a timeout) natively. If `trap` behaves the same, ~120s of the
   278.6s disappears with no distribution at all. Untested — it costs one command.
5. **Two different fixture runners exist.** `Makefile:182 test-bash-run` (shell — the release
   gate; has `BASH_TEST_SKIP`, the `memwatch.sh` 4 GB cap, the expect-filter, `HOME`
   isolation, the serial history group) and `tools/bash53suite/main.go` (Go — what every
   `dag.md` chunk target actually runs; has **none** of those guards, and chunks by stride
   `i%N`). "Chunked vs serial" today compares **two different programs**, so the equality
   property that would make chunking trustworthy is currently vacuous.
6. **The duration history the packer depends on is destroyed by any subset run.**
   `dag.md:491` does `cp durations.new "$durations_file"` — overwrite, not merge.
7. **The existing aggregator already commits the sin the design forbids.** `dag.md:483` sums
   `Results:` across all chunks into one scoreboard — in the recorded 3-host run it merged
   macOS and Windows chunks into one board. Both a cross-`ContextKey` merge and an
   absolute-count gate, where the design mandates a fail-set diff.
8. **Already shipped — reuse, do not rebuild:** `dag.md` chunk targets (`test-bash-chunk`
   `CHUNK=I/N`, `dag-fanout` with duration-balanced greedy packing, `PLAN_FILE`, `HOSTS=`
   ssh fanout, container lanes, `yash-chunks`) — already run across 3 hosts. In Go:
   `coreutils/pkg/dag/{cache.go, timings.go, contract.go:43 Attestation+attest(), expand.go,
   exec_mesh.go}`. `ContextKey` is real code at **`coreutils/pkg/spacetime/probe.go:246`**
   (docs that cite `pkg/skills/probe.go` are wrong — that file does not exist; it is
   re-exported via `pkg/skills/spacetime_alias.go`, and requires-scoped probe selection is
   `pkg/skills/source.go:137 KeyProbes()`).

---

## Direction: four axes, each a seam

### Axis 1 — work model (what a chunk is)

| Mode | Meaning | Status |
|---|---|---|
| **`independent`** | chunks run alone, any order, any host; results merge at the end (test chunks, sweeps, batch inference, ETL maps) | **MVP** |
| `dependent` | a chunk's output is another chunk's input across hosts (classic pipeline/ETL) | seam: existing `Requires:` + content-addressed artifacts |
| `gang` | chunks run *simultaneously* and communicate (multi-node training, MPI, all-reduce) | seam: `Gang:` reserved; needs co-scheduling + rendezvous + all-or-nothing placement — a different scheduler |

MVP scheduler = **pull queue + LPT** (Graham 1969, `4/3 − 1/(3m)` bound). No speed model.
A pull queue is a strict subset of what a gang scheduler needs, so this forecloses nothing.

### Axis 2 — reach (how the orchestrator gets to a worker) — **orthogonal to isolation**

`Venue:` says what *isolation* a chunk gets (userland / workspace / sandbox). `Transport`
says how the orchestrator *reaches* it. A `Venue: sandbox` chunk can be satisfied by a local
container engine, a peer's, or a scheduled pod — the chunk does not care.

| Transport | Status |
|---|---|
| `local` (in-process pool) | **MVP** |
| `ssh` | **MVP** — exists in `dag.md`, has run on 3 hosts |
| `mesh` (remote agent on a paired host; reaches machines ssh cannot) | seam — **the second implementor, and the reason `Transport` is an interface at all** |
| `cluster` / `cloud` (disposable declared reference host) | seam |

**This is why the Go `Pool`/`Transport` layer earns its place.** With ssh as the only
transport it would be a rewrite of working shell. With a mesh transport in the picture it is
the product: ssh only reaches machines you can already reach.

### Axis 3 — data plane (what moves)

| Mode | Meaning | Status |
|---|---|---|
| **control-plane + artifact return** | worker fetches its own inputs (clone / pull image); **results** return as structured per-item records | **MVP** — exactly what a merged scoreboard needs |
| full staging (inputs pushed, outputs collected, content-addressed) | required for real ML/ETL (datasets, checkpoints, weights) | seam: `Inputs:`/`Outputs:`; the blob store already ships as bashy externals |

### Axis 4 — the language (bashy++)

**`dag.md` is the *declarative* workflow surface; bashy++ is the *in-language* one.** Two
faces of one runner: a chunked ML/HPC pipeline must express fan-out, typed records, and
stage-to-stage handoff *in the script*, not only in a task heading.

**The seams already exist — this is additive, not a rewrite:**

- **`expand.ValueKind` is already a union** (`String` / `NameRef` / `Indexed` /
  `Associative` — `sh/expand/environ.go:67-125`). A structured value is a **new Kind
  (`Object`)**, not a refactor of `map[string]string`.
- **`syntax.LangVariant` already gates the parser by dialect** (`LangBash` / `LangPOSIX` /
  `LangMirBSDKorn` / `LangZsh` — `sh/syntax/parser.go:29-51`). `LangBashPP` slots in beside
  them.
- **Subshells are already goroutines**, so concurrency costs almost nothing at the runtime
  layer.

**bashy++ is a true superset of bash — and the claim is MEASURED, not asserted.**

The design rule that makes supersetness achievable — and the lesson C++ teaches by *failing*
it (C++ is not a strict superset of C, and the reason is new reserved words: any C program
with a variable named `class` stops compiling):

> **Prefer two-word keywords and new operators over new single reserved words.**

- ✅ **`go routine { … }`** — two-token lookahead in command position. `routine` is not a
  `go` subcommand, so the collision surface is nil and `go build ./...` keeps working. (Bash
  resolves reserved words pre-expansion and only unquoted, so `go "routine"` and `go $x`
  stay ordinary commands for free.)
- ✅ **`:=`, `<-`** — new operators. Today `x := y` is a command `x` with two args; nobody
  writes that. Check `<-` against `<` redirection and `<<-` heredocs.
- ⚠️ **bare `struct` / `chan` / `func`** — the C++ mistake. Any script invoking a *program*
  of that name breaks. Use two-word forms or `declare`/`typeset` extensions instead.

**The gate is empirical, and the dogfood IS the guard:**

> Run the **entire** conformance matrix with bashy++ **ON** and require a **byte-identical
> fail-set** to mode-off — not just the 86 bash fixtures, but the 719-script clean-room
> differential, the 10-shell panel, oils, yash, modernish. If the fail-set is identical with
> extensions live, the superset property is *measured*. Permanent regression test: the
> conformance suite guards the language, and the language is what makes the conformance
> suite's own records structured. The two halves check each other.

So there is **no script pragma** and no opt-in friction. Two gates remain, both host-level:

1. **`--posix` / `set -o posix` turns everything off** — which is what a cert run uses.
2. **The engine is shared.** `interp`/`expand`/`syntax` live in `../sh` and have other
   consumers, so "always on" cannot mean "always on in the engine". Gate at **`LangVariant`**:
   bashy sets `LangBashPP`; other consumers keep `LangBash`. A host-application choice, not a
   script one. **The real work:** the *parser* has a dialect seam; **`interp`/`expand` have
   none**. It must be built — and it is the same seam a zsh mode would need, so build it once.

**What a goroutine can actually run — this shapes L2.** Subshells are already goroutines, but
an **external binary is still `fork`/`exec`** and always will be: `go routine { curl … }` is
`&` with extra syntax. A goroutine pays off only when the body is **in-process** — shell
functions, builtins, and the Tier-1 coreutils that already run without forking. The prize is
in-process fan-out where stages hand each other **native values over channels instead of
serializing through pipes** — exactly where the 0-fork Tier-1 thesis cashes out, and exactly
what the HPC/pipeline case needs.

**Sequencing — types before concurrency.** Parallelism already exists (`dag -j N`, chunks, the
fleet, goroutine subshells); **structured values do not**. The runner consumes structured
records everywhere — run records, host facts, `chunks.json`, the merged scoreboard — and each
is currently text, serialized and scraped back with `awk`. *That is exactly why the existing
aggregator silently merges results across `ContextKey`s: awk cannot tell that it is doing it.*
A channel is also just a `Value` of native kind, so the union lands first regardless.

| Phase | Ships |
|---|---|
| **L0** | `Object` ValueKind; the `interp`/`expand` dialect seam + `LangBashPP`; **auto-JSON at the OS boundary**; **the superset gate** |
| **L1** | typed records via a two-word/`declare` form (**not** a bare `struct` keyword); `:=` tuple-return with an auto-bound `err` |
| **L2** | **`go routine { … }`** + channels (`<-` / `send` / `recv`) — pays off for in-process bodies only |
| **L3** | the reflect **Go bridge** + type registry — most power, most risk: an unbounded bridge is an unbounded effect surface and must answer to the existing `Effects`/`EffectCap` lattice |

**L0 is the only language phase the dogfood needs** (structured per-case records instead of
`awk`-scraped `Results:` lines). L1–L3 follow once the runner has proven itself on a workload
whose correctness we can check exactly.

---

## Invariants (violating one is a bug, not a tradeoff)

1. **Chunk count is a corpus property.** Derived once from `round(T_suite/τ)`, pinned in a
   **committed** manifest, changed only when the corpus changes. **Never** derived from fleet
   capacity — else `(n,i)` names different cases per run, and both selective re-run and the
   fingerprint cache break. *(Today `dag.md:328` does `chunks="${CHUNKS:-12}"` and writes the
   plan into gitignored `bin/`. Currently violated.)*
2. **The authoritative run is single-host and unchunked.** `make test-bash` 86/86 serial on
   one box remains the release gate. A chunked campaign result may **never** tag a release or
   speak for certification. Chunked = "a fast heterogeneous signal."
3. **Never slice below an atom.** Running case 7 of a suite in a shell that never ran 1–6
   yields a *false failure*, not an approximate one.
4. **`makespan ≥ max(T/S, longest_atom)`.** If the longest atom exceeds the target, no fleet
   reaches it and **the target must move** — honestly, and in writing.
5. **Merge only within one `ContextKey`.** The summarizer must *refuse* to merge across keys.
   Hash only the probes the suite's requires-clause names, or every host is unique and nothing
   merges.
6. **Infra failure ≠ conformance failure.** A host that could not run a chunk did not produce
   evidence that bashy fails bash. `preflight_failed` is a distinct outcome.

---

## MVP — phased

### P0 — Unhang the gate, converge on one runner *(no distribution; biggest single win)*

CI 20.7 min → ~3 min, and the conformance gate starts existing again.

- **`tools/bash53suite/main.go` becomes *the* runner.** Port the shell harness's guards:
  `BASH_TEST_SKIP`; the 4 GB/fixture memory cap (without it a chunked run can wedge a host on
  `intl/unicode1.sub`); per-group `HOME`/`HISTFILE` isolation and the **welded serial group**
  `histexpand`+`history` (they race on `$HOME/.bash_history`, and stride-chunking currently
  puts them in *different* chunks); `-jobs N` with the memory-slot clamp
  (`scripts/test-bash-parallel.sh:40-62`); a whole-suite deadline that **always** prints
  `Results:`. It already has the right kill primitive (`proc_unix.go` sets `Setpgid` from the
  *parent*, then `killProcessTree`) — which the Makefile watchdog does not: `Makefile:238`
  kills `-$test_pid`, a process group that exists only if the testee honored `BASH_SETPGRP`,
  and it fails **silently** (`2>/dev/null`) and then blocks forever on `wait`.
- **`Makefile`**: `test-bash` / `test-bash-run` / `test-bash-parallel` delegate to the Go
  harness, keeping the `BASH_TEST_*` names as the interface. **This is what first makes the
  chunked-vs-serial equality property meaningful.**
- **`.github/workflows/test.yml:79`**: delete `continue-on-error: true`; gate timeout 30 → 8 min.
- **`scripts/ci-bash53-gate.sh`**: gut the background-make/log-poll/deadline workaround (it
  exists solely to survive the hang); keep the ratchet. `test/bash53-known-failures.txt` has
  **zero live entries**, so the first honest Linux run needs baseline reconciliation.
- **`execscript` on Linux is a real bashy bug** — fix or baseline it, never hide it.
- **Free experiment:** time `trap`/`jobs` through the container lane (ground truth 4).

### P1 — Pin the corpus, merge the records *(correctness; still no fleet)*

- **Create committed `bashy/chunks.json`**: per suite — atom kind, pinned `chunks`, `τ`,
  `welded` groups, `membership_hash`, explicit assignments. Promote the existing
  `bin/bash53-chunks.plan.tsv` (it already isolates `jobs`/`trap`/`coproc`) out of gitignored
  `bin/`. The runner reads membership from the manifest — **not** `i%N`. For bash53, `chunks`
  = 5–6 (τ=60s): more buys nothing, because `jobs` at 112.4s is the floor.
- **Fix the duration clobber**: merge `DURATION` lines keyed by atom; commit
  `test/bash53-durations.tsv` (a corpus property, like the manifest).
- **Per-case records** (`-record out.jsonl`, `bashy.run.v1`): suite, chunks, chunk, case,
  status (`pass|fail|time|skip`), duration_ms, venue, **context_key**, code_sha, image_digest.
- **`ContextKey` must be computed *inside the venue*, not on the launching host.** This is the
  subtlety that sinks the plan if missed: a `Venue: sandbox` chunk on macOS would otherwise get
  `os=darwin` and the identical container on Linux `os=linux` — **different keys, so the
  summarizer refuses to merge exactly the chunks that must merge.** Sandbox → key over
  `{venue, image_digest, arch}`, host OS deliberately excluded. Workspace → `{os, arch} ∪
  requires-probes`.
- **Create `tools/scoreboard/`**: reads the per-case JSONL and **refuses** an incomplete chunk
  set and **refuses** a cross-key merge; emits a **fail-set diff** against a committed
  baseline, not a count. Replace the awk tail in `dag-fanout` with it.

### P2 — `Pool` + `Transport` in Go *(the product seam)*

- **`coreutils/pkg/dag/fleet.go`**: `Pool` (workers, slots, LPT dispatch), `Transport`
  interface, `Worker` (host + capabilities + ContextKey). Implement `local` + `ssh`
  (`exec_ssh.go`), designed against the mesh transport as the known second implementor. Do
  **not** name the host-capability check `preflight` — `pkg/dag/preflight.go` already means the
  `Tools:` in-body check; call it `hostcheck`.
- **`Task` fields** (all absent today): `Venue:`, `Items:`, `Chunks:`, `Image:`,
  `Requires-host:` (venues 1–2 only — a sandbox chunk takes its OS from the image and must
  **not** be constrained on host OS, or the whole macOS fleet is stranded), `Mem-per-task:`.
  Reserved but unimplemented: `Gang:`, `Inputs:`/`Outputs:`.
- **Hermetic sandbox venue** — fixes the two failures the recorded fleet run actually had.
  `xcase: command not found`: `main.go:234` builds the C test helpers only `if cc` exists and
  **silently continues** otherwise, and it **reuses an existing binary** — so a macOS-built
  `recho` gets bind-mounted into a Linux container (a host-artifact leak). Build helpers
  **inside** the container; a missing compiler is `tool.missing`, a **refusal**, never a
  conformance FAIL.
- **`doctor fleet --json` → `HostFacts`** (tri-state; `unknown` refuses placement) + the
  closed-prefix failure classes. Narrow job: refuse placement, report `preflight_failed`.
- **Scrub host names and user paths** out of `dag.md` into a gitignored fleet file.

### P3 — The other suites *(where hours → minutes is actually won)*

Replicate the contract that already works for yash (`--list` of atom names + a subset argument
+ three dag targets) across oils-diff, xcu/posix-diff, multishell, austin, parity, zsh, dash,
uutils, modernish. Then the three levers that are **not scheduling at all** and that dominate:

1. **Publish the oracle images to a registry, digest-pinned; pull by digest.** Kills the
   per-host image-build tax *and* delivers the hermetic venue in one move. Highest value here.
2. **Cache the uutils cargo test binary** per (arch, toolchain) — `UUTESTS_BINARY_PATH` is the
   existing override seam.
3. **Decide `modernish`**: 3 shells × `timeout 420` = **7 min**. If its test set cannot be
   sliced, **7 min is the matrix floor** (invariant 4) — say so plainly, or move it to a
   nightly lane and state that the per-change matrix excludes it.

Honest floors: bash53 → `jobs` 112s (hard). yash / oils / xcu / austin / parity / multishell /
zsh → divide freely; floor = the image build (solved by #1). uutils → the cargo compile
(partly, #2). modernish → **7 min, not sliceable today**.

### P4 — The dogfood

```sh
bashy dag conformance
```

One target fanning every suite's chunks through the pool. Chunk counts from committed
`chunks.json`; hosts from a **gitignored** fleet file; `doctor fleet` first (`unknown` ⇒
refuse); prep once per host; `tools/scoreboard` merges **within each `ContextKey`** and prints
one board per key, a fail-set diff, and the wall time against the floor.

---

## Verification (ordered by what fails hardest if wrong)

1. **Same image, two host OSes, same key.** The identical digest-pinned container on a macOS
   worker and a Linux worker of the same arch ⇒ **equal `ContextKey`**, clean merge. *This
   decides whether the fleet is usable at all, and it is what fails if the key is probed on the
   host instead of inside the venue. Run it first.*
2. **Chunked ≡ serial, case for case.** Serial single-host vs the N-chunk fanout on the same
   host: identical per-case verdict *sets*, diffed from the JSONL — not from summary counts.
   Only meaningful after P0 makes both paths one binary.
3. **`-chunk 1/1` is a strict no-op** — byte-identical atom sequence to no chunk flag.
4. **Cross-key merge is refused.** Force one chunk onto Linux and the rest onto macOS ⇒ the
   summarizer **errors**. (Today's awk tail silently prints `86/86` — this is a regression test
   against live behavior.)
5. **An incomplete chunk set is refused** — no scoreboard may be emitted.
6. **Membership stability** — the case→chunk hash is identical across runs, hosts, and fleet
   sizes; a `TESTS=`-narrowed run must **not** rewrite the durations file.
7. **The weld holds** — `histexpand` + `history` always in one chunk, serial, private `HOME`.
8. **Host-artifact leak fails loudly** — bind-mount a macOS-built `recho` into the Linux lane ⇒
   `lane.host_artifact_leak`; a host with no `cc` ⇒ `tool.missing`. Never a conformance FAIL.
9. **The release gate is untouched** — `make test-bash`, serial, single-host, 86/86, 0 skipped,
   remains the only thing that may speak for a release.

---

## Non-goals (hold the line)

Durable workflow engine · rich pipeline DSL · dynamic matrix expansion from fleet size ·
retry/backoff engine · scheduler leases/resume/replay · OTEL dashboards · SQLite projection ·
Slurm/k8s/cloud adapters *in this slice* · a separate `TaskSpec` envelope (**the `dag.md`
heading IS the task spec**; a second envelope is the workflow engine we disclaim) ·
cert/reference profiles and `--repeat` (that is *certification authority*, a different goal from
wall time) · more chunks for bash53 than the atom floor justifies.

## Docs to amend

- `chunked-fleet-conformance-plan.md` — `lane` → `venue` (one axis, two names); drop
  `TaskSpec`; move `doctor fleet --json` from step 1 to step 3 of its implementation order.
- The closed retrospective minutes — record the two amendments (`lane`, `TaskSpec`) and the
  reason: **the goal changed** from "faster tests" to "a general distributed runner".
- Anywhere citing `pkg/skills/probe.go` for `ContextKey` — it lives in
  `coreutils/pkg/spacetime/probe.go:246`.
