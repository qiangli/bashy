# Sprint 355: stable Bashy base and data-driven extensions

**Story:** #390 (`375033377f88`). **Status:** design for review; no production
split is implemented here. The Sprint 355 Profile D rerun remains gated on the
64 blocker and 9 INSPECT accounting.

## Four layers, one front door

| Layer | Lives in the downloadable `bashy` executable? | Contract |
| --- | --- | --- |
| **Base** | Yes | POSIX shell, Bash# grammar and runtime (including fence syntax), and Coreutils applets. This is the certified route. |
| **Core** | Yes | Generic dispatch for already parsed fences, versioned runner protocol, minimal language/toolchain CRUD, and command atlas/registry plumbing. A new language is a record, not a Go import. |
| **Builtin** | Mechanism in the executable; first-party records/payloads separately versioned | Curated first-party registrations such as Genie. They use the same runner contract as third-party rows. |
| **Optional/experimental** | Registry metadata and digest-pinned payloads installed on demand | Feature-gated records, with their own lifecycle. They cannot extend the certified POSIX command route. |

The user downloads one `bashy` app. Existing `bashy commands
add|show|set|rm|verify` keeps its `kind: command` behavior. Add explicit
`bashy commands language add|show|set|rm|verify` and `bashy commands
toolchain add|show|set|rm|verify` nouns; `show` may accept `view` as an alias.
Neither noun creates a shell command. Downloaded runtimes are payloads
selected by records, never another manually required Bashy download.

## Current dependency boundary and target

`cmd/bashy` imports `internal/agentos`; that package's init and imports bring
the optional front door, ycode/Genie, provisioners, network/database stacks,
and static fence tables into every shell and applet process. The base route
already enters `internal/cli`, Coreutils' `cmds/all` and `multicall`. The
certified route must retain exactly those semantics, including the
`VSC_PROFILE=cert` exclusion of the operator's command ring.

Move generic fence dispatch and runner interfaces into a small core package
with no imports of `internal/agentos`, ycode, Genie, cloudbox, or managed
engines. The `cmd/bashy` base entry must also stop importing `internal/agentos`:
it handles shell/applet dispatch directly and sends an explicitly selected
front-door extension through the core's verified child-runner path. Otherwise
Go still initializes every optional import before `main` and the split saves
no startup cost. The core owns record validation, collision checks,
digest verification, cache lookup, capability/effect metadata, and the
versioned request/result envelope. Optional handlers run as cached child
processes through the existing `binmgr` materialization path. They do not
register `init()` hooks into the certified process.

The core receives a Bash# fence node already recognized by the base parser;
Bash# grammar and its interpreter/compiler semantics stay base-owned. A
language record supplies a tag and runner, never a new token, production,
AST shape, or POSIX-mode parser hook. POSIX mode
never consults the extension index for parsing or command resolution. A
grammar or Bash# runtime change therefore changes the base candidate and
requires certification impact review even when an optional runner motivated
it.

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

## Record, namespace, and runner contract

The proposed `bashy.extension/v1` record has `kind: language|toolchain`, a
canonical name and aliases, `stage: builtin|optional|experimental`, schema
version, minimum runner-protocol version, supported OS/arch, declared effects,
and an exact payload tuple `(version, os, arch, sha256, source)`. A language
also names its fence spellings, analyzer/prepare/invoke/methods operations, and
any required toolchain record IDs; a toolchain names an absolute executable
inside its verified payload and an argv prefix. No field is executable shell
text. The selected catalog version and all dependent payload digests are
locked together before a unit starts; `set` writes a new record atomically,
and an active unit keeps its original snapshot.

The core runner speaks a length-delimited JSON request/result envelope over a
child's pipes: protocol version, operation (`analyze`, `prepare`, `methods`,
`invoke`), unit/content digest, method, arguments, cwd, and explicit
environment/effect grants in; exports or method signatures, stdout/stderr,
exit status, and diagnostics out. It never executes a row through `sh -c` or
falls back to a tool found on `PATH`. Core validates the record, effects,
digest, protocol compatibility, and response bounds before exposing an
export. Major mismatch refuses the row; an additive minor version is accepted
only when both sides declare compatibility. A changed core implementation,
including a protocol change, is a new executable candidate even if a row
claims backward compatibility.

Fence tags and toolchain names are separate namespaces from POSIX commands.
Canonical names and aliases are unique within each namespace after lowercase
normalization; registration refuses ambiguity and names the incumbent. Builtin
fence tags and pinned base toolchain names are reserved, and optional records
cannot replace them with `--force`. A new Bashy release may migrate a reserved
row only through an explicit compatibility gate. Shell builtins, `sh`/`bash`,
Coreutils names, and every shipped front-door name remain reserved in the
command namespace. An extension record cannot add an argv[0] alias or a
`/vsc/cushim` route. An operator's ordinary command record keeps its current
collision rules, but is absent from `VSC_PROFILE=cert` lookup.

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

For each proposed change, record the old/new executable digest, certified
route manifest, effective `VSC_PROFILE=cert` configuration, provider paths,
and affected behavior. If executable bytes or the certified route change,
open a base/core candidate and assess the relevant shell, utility, and Profile
D regression scope before approval. If only data or payload bytes change and
both invariants compare equal, prove the POSIX route with empty and populated
extension rings and check that PATH, environment, startup side effects, and
provider selection are unchanged. Any POSIX-visible difference still triggers
impact review and the relevant replay under the applicable policy, regardless
of digest equality. Thus data-only updates are eligible for an optional-only
release; they have no blanket certification immunity. The official authority
decides whether a changed certified product needs further formal testing.

The user upgrade path is `bashy` for the base/core app and `bashy commands
verify` or a matching catalog operation to fetch/update a pinned payload.
Offline use succeeds from a verified cache and fails with a precise missing
digest/payload message otherwise; records can be imported and validated
offline. A failed fetch or digest mismatch cannot change the cached active
version. Rollback selects the prior signed catalog while retaining its
verified payload cache.

The one downloaded executable contains its trust root and a locator for
first-party catalogs, not Genie or an optional runtime hidden in its import
graph. On first explicit use, `bashy` fetches the signed catalog and exact
payload to a content-addressed cache, verifies both, then atomically selects
the record. Offline users may import those same signed bytes ahead of time;
base shell and Coreutils use require no catalog or network. Catalog updates,
including Genie's builtin record, can be released without changing the
base/core executable; rollback reselects a prior verified record and never
silently substitutes a newer cached payload.

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
   Record the import graph and startup/binary baseline on Linux and macOS.
2. Add versioned language/toolchain record readers and the child runner
   protocol. Prove one external language can be added, verified, invoked,
   upgraded, and removed without rebuilding Bashy; test collisions, digest
   refusal, offline cache, and protocol mismatch.
3. Migrate static text/manifest rows first, then `polyglot.RegisterLanguage`
   worker rows and `islandToolchains` through compatibility adapters. Match
   current exports, method discovery, effects, lowered-program behavior, and
   pinned provisioner selection before removing each Go import. Migrate Genie
   to a first-party builtin record after its existing CLI behavior has parity.
4. Add signed catalog publication and the optional-only executable-digest,
   route-manifest, and populated-ring invariance gate. Measure Linux startup
   and binary size for the integrated core, then review any POSIX-visible
   change before enabling the split on a certification candidate.

This design is separate from the Sprint 355 Linux signal repair. The latter
must pass its focused host probe before the next full Profile D run.
