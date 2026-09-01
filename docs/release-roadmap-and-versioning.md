# Bashy release roadmap and versioning policy

Status: **plan of record, 2026-08-08**. This document defines release order,
public promises, upstream compatibility coordinates, and version-number rules.
Detailed gates remain in `bashy-v1.0.0-readiness.md` and the component plans.

## Principles

1. **One product version, several compatibility coordinates.** `vX.Y.Z` is the
   Bashy product/API version. GNU Bash, POSIX, Go, GNU Coreutils, and Tessaro
   versions are recorded independently in release metadata and the support
   matrix. One SemVer tuple cannot losslessly encode five independent streams.
2. **SemVer describes user impact.** Major means an incompatible Bashy public
   API/language/default-behavior change; minor means backward-compatible public
   functionality; patch means backward-compatible fixes. This follows
   [Semantic Versioning 2.0.0](https://semver.org/).
3. **Compatibility is a tested profile, not an implication.** Every release
   names exact upstream sources, patches, test profiles, host platforms,
   denominators, and known exceptions.
4. **Profiles accumulate when practical.** Adding a newer GNU/POSIX/Go profile
   is normally a minor release if the previous profile remains supported.
   Replacing a default or removing a supported profile may require a major.
5. **No aspirational claims.** “Compatible,” “conformant,” “certified,” signed,
   and packaged each have independent evidence gates.

## Roadmap

### Foundation milestones before v1.0

1. **GNU Bash 5.3 compatibility — achieved; continuously gated.**
   - Keep the serial GNU Bash 5.3 fixture gate and clean-room differentials.
   - Track the exact upstream tarball and applied Bash 5.3 patch level. GNU
     announced Bash 5.3 in July 2025; the release and patch series are separate
     provenance inputs.
2. **IEEE Std 1003.1 Shell and Utilities — shell milestone achieved; utilities are next.**
   - Correct standard family name: **IEEE Std 1003.1**, not 1002.1.
   - Current licensed campaign profile: VSC-PCTS2016/POSIX08, as already
     recorded by the harness and license documents.
   - Lane A: shell language/builtins. **Achieved 2026-08-08:** the complete
     493-TP `POSIX.shell` scenario placed all 493 TPs in the certification
     pass group with zero blockers, zero manual resolutions, and zero caps.
     The measured shell-isolation PATH used GNU Coreutils 9.11 and excluded
     Bashy's Go applets. Evidence tag: `vsc-pcts-posix-shell-2026-08-08`.
     The dedicated shell-closure sprint (#45) is ended; reopen this lane only
     for a demonstrated regression.
   - **Profile B, the primary corrective arm:** complete all 116 Commands and
     Utilities sets (8,844 configured TPs) with Bashy `sh` and the frozen
     GNU/system provider manifest. Bashy's Go multicall is excluded so the
     matched A/B delta isolates the shell.
   - **Profiles C/D, the Go-utility track:** place the canonical Bashy Go
     multicall first under GNU Bash or Bashy. These profiles measure the 76 Go
     applets and the assembled provider gaps independently of Profile B. See
     `posix-command-coverage.md` for the exact accounting.
   - Profile B exit: all 116 sets and 8,844 TPs measured, no unexplained
     Bashy-only failures, `UNRESOLVED` results, or caps in the declared scope,
     repeatable matched-host evidence, and finalized provider limitations.
   - After Lane B: execute one uninterrupted 117-set/9,337-TP assembled formal
     profile, generate the suite's official report (`vrpt`), finalize the
     conformance statement, and prepare the human submission.
   - Formal Open Group submission/certification remains a separately named
     human/legal milestone; engineering completion must not be worded as a
     certification award.
3. **Bash++ language foundation.**
   - Stable target for v1.0: Go 1.26-shaped constructs supported by the declared
     Bash++ grammar/profile, including the race and lifecycle gate in
     `bash-plus-plus-design.md`.
   - Go 1.26 is an upstream toolchain/language coordinate, not a promise to
     accept arbitrary Go source. Every supported construct must be enumerated.
   - Python, TypeScript, and Rust embedding starts experimental, opt-in, and
     capability/effect-gated. It becomes stable only after runtime discovery,
     version negotiation, sandbox/effect policy, packaging, cancellation,
     serialization, and cross-platform gates exist. Recommendation: do not make
     all three stable embeddings blockers for v1.0.

### v1.0.0 — official foundation release

Public promise: stable Bash 5.3-compatible shell foundation, declared POSIX
profile and limitations, stable Bash++ Go-construct profile, and reproducible
official packages.

- Unix/Linux archives and packages, with checksums and provenance.
- macOS universal/per-architecture artifacts, notarization/signing and package.
- Windows artifacts, signing and package/installer.
- Cross-platform install, upgrade, uninstall, smoke, signature, and checksum
  tests. A cross-build alone does not satisfy this milestone.

### v1.1.0 — expanded GNU Coreutils compatibility

- The POSIX Go-utility C/D profiles are pre-v1.0 engineering work, not a v1.1
  introduction. They are separate from the Profile B shell arm.
- Expand the declared command and option/behavior matrix beyond the POSIX
  profile toward GNU Coreutils compatibility.
- Gate GNU Coreutils 9.11 differentials and uutils-derived tests by provenance
  and relevance; do not require foreign-suite 100% where it tests extensions.
- Preserve external-command fallback and provider reporting where commands are
  not promoted.

### v1.2.0 — stable agentic surface

Public promise: **the agentic foundation becomes supported API** — addressing,
schema/versioning discipline, and the relation vocabulary's extension rule.
Individual agentic features graduate on top of it one at a time, as feedback
arrives, and not all of them by v1.2.0.

- Promote the documented AgentOS/agentic commands and schemas to supported API
  **as each one graduates**, not as a single batch.
- Require capability routing, safety/effect policy, structured output/schema
  versioning, observability, and benchmark evidence.
- Experimental agentic verbs may exist earlier; v1.2.0 is the stability
  commitment for the foundation they stand on.
- **Agent protocol interoperability — tracked for development no earlier than
  v1.2.0.** Plan MCP in both directions (expose Bashy tools and consume
  external servers), ACP in both directions (drive local harnesses and let
  editors drive Bashy), and A2A in both directions (delegate to and serve
  remote peers). These are adapters over the unified agentic graph's identity,
  policy, evidence, rendezvous, and lifecycle model, not three new control
  planes. This item is **not an additional v1.2.0 release gate or a promise
  that every protocol surface graduates together**: each surface enters at
  `experimental`, remains subordinate to Step 1 below, and graduates only on
  protocol conformance, fail-closed security, measured runtime evidence, and
  a named consumer. The protocol-specific plan of record is
  `dhnt/docs/bashy-agent-protocols-1.2.0-plan.md` in the umbrella. It does
  not reopen the deferred generic mount/adapter layer or `bashy://` scheme.

#### Stability tiers — how a feature ships before it is frozen

The agentic surface releases in **stages**, so a feature can ship, collect real
feedback, and change shape without spending a major. Every agentic verb and
schema carries exactly one tier, reported by `bashy commands --atlas` and
`bashy context --json`:

| Tier | Promise | Change cost |
|---|---|---|
| `experimental` | May change or disappear without notice. Off by default; not in the support matrix | patch |
| `preview` | Shape is believed settled and is being validated in real use. Breaking changes are announced in release notes and carry a migration note | minor |
| `supported` | Bound by the version rules above | major to break |

Rules: a feature enters at `experimental` and graduates only on the evidence
named below — never on age. Graduating is a **minor**; demoting or removing a
`supported` feature is a **major**; changing an `experimental` one is a
**patch**. The tier is per feature, so `weave` may be `supported` while a new
relation is `experimental`. **Nothing graduates to `supported` without a
consumer** — an unused feature has produced no feedback, so its shape is
untested regardless of how long it has existed.

#### Foundation first, then an MVP — not a full-blast re-architecture

The agentland re-architecture (umbrella
`docs/bashy-unified-agentic-graph-architecture-plan.md`, revised 2026-08-08)
is rank-3 work: **build the seam, prove it runs, stop.** It is scoped here in
three steps, and only the first is a v1.2.0 gate.

**Step 1 — foundation, and the only v1.2.0 gate**, because deferring it is what
costs a major. Two properties must hold before any agentic schema reaches
`supported`; neither requires the re-architecture to be finished:

- **Addressing is indirected.** A single `Ref` type exists and PID is advisory,
  so agent name / control socket / episode / seat are attributes rather than
  keys. Twelve identity keys are in use today; unifying them *after* the surface
  is promoted is an incompatible public change. Indirection now keeps the later
  unification a minor.
- **The relation vocabulary is open.** A well-known core exists, and **an
  unknown relation round-trips through an unmodified reader.** A closed
  vocabulary promoted to supported API cannot grow without a major.

That is the whole gate. Everything below ships at `experimental`, collects
feedback, and graduates on its own schedule across later 1.2.x/1.3 releases.

**Step 2 — MVP: one path, proven, `experimental`, off by default.** The
narrowest useful slice of runtime-authored edges: emit the attribution →
generation → gate-verdict chain for **one** run type, from weave and the gate
(not execlog, which records argv and exit but no stdout). Baseline to beat is
effectively zero — 17 `graph` invocations in 6.7 days, and one unprompted
knowledge-store access in 61,084 dispatched commands. Prove the chain exists on
real runs, then stop. Broaden only when ranks 1 and 2 are not waiting, and
**expect the relation set to change once something consumes it** — that is the
point of shipping it at `experimental` rather than designing it further on
paper.

**Step 3 — iterate, each on its own evidence and its own tier.** Not milestone
gates: collapsing the three context-injection paths into one budgeted assembler;
the fake-edge lint in `dag check`; the admission rule (four measurable benefits
must exceed the coordination tax before work is expressed as a graph); an
oscillation detector (tool calls ÷ distinct, with a stop rule) in
`weave`/`supervise`. Each enters at `experimental`; each graduates when it has a
consumer and the evidence below, or is removed if it does not earn one.

**Separately, and not a milestone gate at all:** three silent data-loss races
are live now — `room.Join` read-check-write, `bus.MarkRead` rewriting a file the
sidecar appends to, `mb.PostMessage` colliding on a sequence number that is also
a filename. Those are bug fixes under the write rule (`O_APPEND` under
`PIPE_BUF`, `rename(2)`, or `O_CREAT|O_EXCL`; otherwise take an `flock`). Fix
them as patches whenever, on their own evidence — do not hold them for a minor.

#### Evidence gates

Per principle 5, scaled to the step:

| Graduation | Evidence required |
|---|---|
| Addressing → `supported` (step 1, the v1.2.0 gate) | Attachment survives carrier replacement and works for a session with no meaningful local PID |
| Relation vocabulary → `supported` (step 1, the v1.2.0 gate) | The well-known core is classified (knowledge vs task, reflexive or not) and an unknown relation round-trips through an unmodified reader |
| Runtime-authored edges → `preview` (step 2, may be post-1.2) | Fraction of completed runs producing a full attribution → generation → gate-verdict chain, against the ~0 baseline |
| Anything → `supported` | A named consumer that would break if it changed. No consumer, no graduation |
| Write-rule fixes (patch, any time) | Concurrent-writer test per offender; `go test -race` clean |

A conformance test that only shows the machinery matching itself satisfies none
of these. State the negative result if it comes, and demote or remove rather
than carrying an `experimental` feature indefinitely.

#### Explicitly out of scope for v1.2.0

Deferred with a blocking reason in the plan's appendix; none may be implied by
the v1.2.0 promise: the mount/adapter layer and any `bashy://` reference scheme;
a full Graph IR with typed ports and a node union; the pattern library and
`graph pattern *`; ForeignRuntime/polyglot embedding; Bonsai as a derived query
index; and all Bash++ graph-module integration — that last is workstream (b) and
belongs to the v1.0 language profile, not here.

#### Sequencing

Agentic work is rank 3 of bashy's three workstreams, behind POSIX certification
and Bash++. Nothing here may preempt the v1.0 or v1.1 gates. Step 1 is small by
construction; steps 2 and 3 add no front-door verb, land inside existing
packages, and touch no file in `sh/`. A rank-3 item that needs a large
measurement campaign to justify itself is not ready to be worked — it is ready
to be written down and deferred.

### v1.3.x — Tessaro integration track

- **v1.3.0:** Sphere/P2P pairing and Ollama execution, with identity, trust,
  discovery, transport, upgrade, and offline/degraded-mode gates.
- **Recommended v1.4.0, or v1.3.1 only if no new public API:** cluster/DKS
  integration. DKS is a materially larger control-plane surface than Sphere;
  it should not inherit readiness merely because Sphere is green.

Minor milestone numbers are planning coordinates, not permission to bypass
SemVer. If a milestone requires incompatible public changes, it becomes the
next major release.

## Product version rules

| Change | Normal Bashy bump | Reason |
|---|---:|---|
| Bashy-only backward-compatible bug/security fix | patch | Corrects promised behavior |
| Upstream patch/toolchain update with no public semantic change | patch | Dependency/provenance refresh |
| Add compatible GNU Bash/POSIX/Go/Coreutils profile or feature | minor | New supported functionality |
| Expose additional Go-version constructs in Bash++ compatibly | minor | Language surface grows |
| Change or drop an `experimental` agentic feature | patch | No stability was promised; this is what the tier is for |
| Graduate a feature `experimental` → `preview` → `supported` | minor | New supported functionality, on evidence and a consumer |
| Add a relation to the agent-graph well-known core (post-1.2) | minor | Additive; an unknown relation must already round-trip through an unmodified reader |
| Break a `preview` feature | minor + migration note in release notes | Shape was believed settled but was still being validated |
| Remove or re-classify a well-known relation, demote a `supported` feature, or change how agents are addressed | major | Stored edges and existing readers require migration |
| Change default semantics, remove a profile, or break public CLI/language/schema | major | User migration required |
| Documentation/evidence-only correction | normally no release, or patch if republishing corrected metadata | No new API |

Consequences:

- GNU Bash 5.3 patch 006 does **not automatically** dictate Bashy `x.y.6`.
  It produces a compatibility-profile revision and a Bashy patch/minor/major
  chosen by user-visible impact.
- GNU Bash 6.x does **not automatically** force Bashy 2.0.0. Adding a Bash 6
  profile while retaining Bash 5.3 can be a minor; changing defaults
  incompatibly or dropping 5.3 requires a major.
- Go 1.26.x compiler/security updates are normally Bashy patches. Adding Go
  1.27 language constructs to Bash++ is normally a minor. A hypothetical Go 2
  only forces a Bashy major when Bashy’s stable language/API becomes
  incompatible.
- A new POSIX edition is a named profile. Adding it is normally minor; making
  it the default when behavior changes incompatibly is major.
- Patch releases remain reserved for backward-compatible fixes and upstream
  refreshes that do not add a promised public feature.

## Compatibility coordinates and release metadata

Every release publishes a machine-readable manifest and human support matrix:

```text
bashy:       1.0.0
gnu_bash:    5.3 + patch-level/hash
posix:       VSC-PCTS2016 / POSIX08 / IEEE 1003.1 campaign profile
go_build:    1.26.x
bashpp_go:   go1.27-profile-v1
coreutils:   GNU 9.11 reference; Bashy command-set revision/hash
agent_graph: none (pre-1.2) | contrib-v1 + relation-registry revision/hash
tessaro:     none | sphere-v1 | dks-v1
platforms:   exact signed/package artifact matrix
evidence:    run IDs, totals, checksums, limitations
```

The binary should expose the same coordinates through a stable `--version` or
`version --json` schema. Annotated Git tags and release notes repeat them, but
the product tag remains simply `vX.Y.Z`.

## Maintenance cadence

- Watch stable GNU Bash releases/patches, POSIX editions and certification
  profiles, supported Go releases, GNU Coreutils, and packaging/signing policy.
- Open one compatibility campaign per upstream change; never silently float.
- Re-run the smallest relevant gate during development and the complete
  single-source release gate before tagging.
- Maintain at least the currently declared profile until a documented major
  migration, and state end-of-support dates before removal.
- Apply critical dependency/security fixes promptly even when they do not
  change Bashy’s feature roadmap.

References: [GNU Bash project](https://www.gnu.org/software/bash/),
[GNU Bash 5.3 announcement](https://lists.gnu.org/archive/html/bash-announce/2025-07/msg00000.html),
[Go release policy](https://go.dev/doc/devel/release),
[Go 1.26 release notes](https://go.dev/doc/go1.26), and
[The Open Group Issue 8 online publication](https://pubs.opengroup.org/onlinepubs/9799919799/).
