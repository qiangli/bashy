# Sprint 355: stable Bashy base and data-driven extensions

**Story:** #390 (`375033377f88`). **Status:** design for review; no production
split is implemented here. The Sprint 355 Profile D rerun remains gated on the
64 blocker and 9 INSPECT accounting.

## Four layers, one front door

| Layer | Lives in the downloadable `bashy` executable? | Contract |
| --- | --- | --- |
| **Base** | Yes | POSIX shell, Bash# syntax/runtime, and Coreutils applets. This is the certified route. |
| **Core** | Yes | Generic fenced-code parser, versioned runner protocol, minimal language/toolchain CRUD, and command atlas/registry plumbing. A new language is a record, not a Go import. |
| **Builtin** | Mechanism in the executable; first-party records/payloads separately versioned | Curated first-party registrations such as Genie. They use the same runner contract as third-party rows. |
| **Optional/experimental** | Registry metadata and digest-pinned payloads installed on demand | Feature-gated records, with their own lifecycle. They cannot extend the certified POSIX command route. |

The user downloads one `bashy` app. `bashy commands add|show|set|rm|verify`
remains the visible CRUD family. It manages command records today; the core
extension adds language and toolchain record kinds under that family (or a
clearly nested noun) without changing bare `commands rm` disambiguation.
Downloaded runtimes are payloads selected by explicit records, never another
manually required Bashy download.

## Current dependency boundary and target

`cmd/bashy` imports `internal/agentos`; that package's init and imports bring
the optional front door, ycode/Genie, provisioners, network/database stacks,
and static fence tables into every shell and applet process. The base route
already enters `internal/cli`, Coreutils' `cmds/all` and `multicall`. The
certified route must retain exactly those semantics, including the
`VSC_PROFILE=cert` exclusion of the operator's command ring.

Move generic fence parsing and runner interfaces into a small core package
with no imports of `internal/agentos`, ycode, Genie, cloudbox, or managed
engines. Keep shell/applet dispatch in the base and bind it to this core at
the `cmd/bashy` entry. The core owns record validation, collision checks,
digest verification, cache lookup, capability/effect metadata, and the
versioned request/result envelope. Optional handlers run as cached child
processes through the existing `binmgr` materialization path. They do not
register `init()` hooks into the certified process.

The specific tables to migrate are `internal/agentos/polyglot_rows.go` and
`manifest_rows.go` (`polyglot.RegisterLanguage` calls), and
`internal/agentos/toolchains.go` (`islandToolchains`). Text rows with fixed
verbs can become validated data records. Dynamic rows (for example `dag`
method discovery) need a runner adapter that implements the same protocol;
their existing behavior is preserved until the adapter has parity tests.
Provisioned toolchain rows point to pinned, digest-checked payload records
instead of importing each `yoke/external/*` provisioner at startup. Do not
turn an unknown fence into a PATH lookup: the present refusal is part of the
island security contract.

The existing `kind: command` registry is the CRUD precedent, with local and
shared rings, collision handling, offline validation, lazy index, and
digest-checked downloads. A language/toolchain record can reuse its storage
and edit machinery, but requires its own schema and runtime protocol; a
`kind: command` exec template alone cannot express method discovery or
fenced-code staging. The POSIX/cert route continues to bypass extension
lookup, including `type`/`command -v`, shell execution, and front-door
resolution.

## Versioning and release invariants

The base/core executable has a content digest and a signed route manifest
recording Go and Bash# runtime versions, Coreutils pin, fence grammar version,
runner protocol major/minor, and the certified command-resolution route.
Protocol minor versions may add optional fields; a major mismatch is refused
before payload execution. Each registration has schema version, feature
stage, supported OS/arch, effects, payload version, and SHA-256 per target.
Builtin and optional catalogs are separately versioned and signed; catalog
updates do not rewrite the executable.

An optional-only release gate compares the base/core executable digest and
certified route manifest byte-for-byte with the previous accepted candidate.
It also runs a hermetic POSIX/cert route probe with an extension ring populated
and verifies the ring is absent from resolution and atlas claims. Any changed
digest or route requires a new base/core candidate and conformance impact
review. A new GNU Bash baseline, Go runtime/toolchain, Bash# runtime, fence
grammar, or runner protocol is deliberately a base/core change.

The user upgrade path is `bashy` for the base/core app and `bashy commands
verify` or a matching catalog operation to fetch/update a pinned payload.
Offline use succeeds from a verified cache and fails with a precise missing
digest/payload message otherwise; records can be imported and validated
offline. A failed fetch or digest mismatch cannot change the cached active
version. Rollback selects the prior signed catalog while retaining its
verified payload cache.

## Measured lower bound and delivery sequence

On macOS/arm64, 120 interleaved warm launches measured the current one-file
shell at **16.074 ms median**, versus **3.805 ms** for the lean `cmd/sh`.
Standalone Coreutils `kill` measured **5.882 ms**, and full Bashy `kill`
**15.826 ms**. A temporary probe importing only `internal/cli`,
`coreutils/cmds/all`, and `multicall` measured **6.133 ms** as `sh` and
**6.035 ms** as `kill`; binary size fell from **161 MB to 39 MB**, and
`inittrace` allocations from **11.35 MB to 0.75 MB**. It omitted the CRUD
runtime and is a lower bound, not a shippable artifact. Each figure is the
median of 120 warm launches on the same machine; cold start, Linux, and
certification performance remain to be measured. Filebrowser's lazy common
password map is already merged separately and accounts for roughly 3.6 ms
of the prior 20 ms shell median.

1. Extract the minimal command CRUD/index and generic fence runner into the
   core import set; preserve current record semantics and cert exclusion.
2. Add schema/versioned language and toolchain records, migration readers for
   current builtins, and one external language fixture registered and invoked
   without rebuilding the executable.
3. Move static text rows first, then dynamic adapters and provisioners, with
   parity tests for fence output, method discovery, effects, and offline
   behavior. Keep first-party Genie as a builtin record.
4. Add optional-only digest/route invariance gate, release catalog signing,
   and representative Linux startup/binary measurements. Enable the split
   only after current shell, applet, and Profile D semantics are unchanged.

This design is separate from the Sprint 355 Linux signal repair. The latter
must pass its focused host probe before the next full Profile D run.
