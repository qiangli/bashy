# Bashy v1.0.0 — Release Notes (DRAFT)

> **Status: DRAFT — not a release declaration.**
> Final candidate commit SHA, module pin table, gate evidence, refreshed Go
> corpus measurement, certification wording and operator approval are finalized
> on the frozen `v1.0.0` release candidate prior to promotion. Items that cannot
> be measured until the candidate is frozen are marked **`OPEN`**.

## Release identity

- **Product version:** `v1.0.0` (draft; untagged)
- **Frozen candidate tag and commit SHA:** `OPEN` (to be filled on the frozen
  candidate)
- **Build coordinate (current `main`):** `go 1.27.0`, toolchain `go1.27.4`
  ([`go.mod`](../go.mod)); Bash compatibility `5.3` with release stamp format
  `5.3.0(1)-bashy-<tag>` (`internal/cli/version.go`,
  [`docs/public-claims-evidence.md`](public-claims-evidence.md))
- **Changelog:** [`CHANGELOG.md`](../CHANGELOG.md)
- **Security policy and contact:** [`SECURITY.md`](../SECURITY.md) and
  [`.well-known/security.txt`](../.well-known/security.txt) (RFC 9116)
- **Third-party licenses:** [`THIRD_PARTY_LICENSES.md`](../THIRD_PARTY_LICENSES.md)
  and [`docs/licensing-supply-chain-policy.md`](licensing-supply-chain-policy.md)

## Certification

POSIX certification statement: to be inserted by the operator from the authority's registration.

## Supported installs

Sources: [`docs/install.md`](install.md), [`docs/setup.md`](setup.md),
[`docs/release-runbook.md`](release-runbook.md), [`README.md`](../README.md).

The supported install paths for `v1.0.0` are:

1. **Install scripts** — download the release archive, verify it against
   `checksums.txt`, and run `bashy self install`:
   - **macOS (Apple Silicon) / Linux:**
     ```sh
     curl -fsSL https://dhnt.io/install.sh | sh
     ```
     (pass `-s -- --no-service` to skip registering Outpost, or `-s -- --service`
     to register Outpost as a boot service).
   - **Windows (PowerShell):**
     ```powershell
     irm https://dhnt.io/install.ps1 | iex
     ```
     (or `& ([scriptblock]::Create((irm https://dhnt.io/install.ps1))) -NoService`
     / `-Service`).
2. **Release archives** — published as GitHub release assets alongside
   `checksums.txt`:
   - `bashy-linux-amd64.tar.gz`, `bashy-linux-arm64.tar.gz`
   - `bashy-darwin-arm64.tar.gz`
   - `bashy-windows-amd64.zip`, `bashy-windows-arm64.zip`

   Each archive carries `bashy`, `outpost`, `bash`, and `sh` (`.exe` on
   Windows); install side by side with `./bashy self install` after verifying
   against `checksums.txt`.
3. **Linux packages** — `.deb`, `.rpm`, and `.apk` packages published directly
   as GitHub release assets alongside the tarballs
   ([`docs/release-runbook.md`](release-runbook.md),
   [`docs/install.md`](install.md)).
4. **Local `FROM scratch` container image** — built locally with
   `bashy self image` from checksum-verified `bashy-scratch-linux-{amd64,arm64}`
   release assets. A public GHCR scratch image is not published
   ([`docs/release-runbook.md`](release-runbook.md),
   [`docs/install.md`](install.md)).

### Not supported at v1.0.0

- **Homebrew, Scoop, and winget are not supported** (operator decision
  2026-11-06; [`docs/release-runbook.md`](release-runbook.md),
  [`docs/install.md`](install.md), [`docs/setup.md`](setup.md)). No Homebrew
  tap formula, Scoop bucket manifest, or winget package is published for
  `v1.0.0`.
- **macOS Intel (`darwin/amd64`) is not supported**; macOS releases support
  Apple Silicon (`darwin/arm64`) only
  ([`docs/release-runbook.md`](release-runbook.md),
  [`CHANGELOG.md`](../CHANGELOG.md)).
- **`go install github.com/qiangli/bashy@latest` is not supported** because
  `go.mod` contains `replace` directives ([`docs/install.md`](install.md)).

## Code signing (unsigned macOS and Windows binaries)

Sources: [`docs/setup.md`](setup.md), [`docs/install.md`](install.md),
[`docs/release-runbook.md`](release-runbook.md),
[`docs/releases/v1.0.0-rc.27.md`](releases/v1.0.0-rc.27.md).

The `v1.0.0` macOS and Windows binaries are **unsigned**
(`Signing: darwin.signed=false windows.signed=false` in the release metadata;
[`docs/release-runbook.md`](release-runbook.md)).

- The `install.sh` and `install.ps1` scripts do not trigger browser quarantine
  or Mark-of-the-Web prompts ([`docs/install.md`](install.md),
  [`docs/setup.md`](setup.md)).
- Browser-downloaded archives prompt at launch:
  - **macOS:** right-click the executable and choose **Open**, or remove the
    quarantine attribute with `xattr -d com.apple.quarantine <file>`.
  - **Windows:** choose **More info → Run anyway**, or unblock the downloaded
    file with `Unblock-File <file>`.
- Code-signed macOS and Windows builds are planned for the `v1.0.x` track, not
  `v1.0.0` ([`docs/setup.md`](setup.md), [`docs/install.md`](install.md)).

## Declared limitations

Every limitation below is drawn from current public documentation in `bashy`
and `bashsharp`.

### Build linkage and runtime footprint

- **Pure Go (`CGO_ENABLED=0`), Linux dynamic locale loading:** Linux, macOS,
  and Windows release binaries build with `CGO_ENABLED=0`; no release links C
  ([`docs/public-claims-evidence.md`](public-claims-evidence.md),
  [`scripts/dist.py`](../scripts/dist.py),
  [`scripts/build-native-darwin-release.sh`](../scripts/build-native-darwin-release.sh)).
  The product runtime build snapshots inherited ignored signals before the Go
  runtime installs handlers (`tools/productgo/main.go`,
  `cmd/bashy/inherited_ignore_runtime.go`). The Linux `bashy` binary is not
  fully static because the locale gate loads glibc dynamically via `purego`;
  the `bashy self image` scratch binary is built without it
  ([`docs/public-claims-evidence.md`](public-claims-evidence.md)). Artifact
  re-verification (`bashy --version`, `go version -m`, `elfaudit`,
  `bashysignalprobe`, and `otool -L` on macOS) on the frozen `v1.0.0` candidate
  is `OPEN`.
- **Go runtime file-descriptor footprint:** The Go runtime opens housekeeping
  file descriptors (such as epoll/eventfd) not present in a C shell's low-fd
  table ([`docs/conformance-statement.md`](conformance-statement.md)).
- **Mixed stdout/stderr flush ordering:** Interleaving of stdout and stderr
  when both streams are merged can differ from GNU Bash due to Go buffering
  ([`docs/conformance-statement.md`](conformance-statement.md)).

### Shell parser, job control, and interactive depth

- **Windows job-control boundary:** Unix builds implement OS-backed
  process-group job carriers (`Setpgid`) and controlling-terminal handoff
  (`internal/cli/carrier_unix.go`). Windows provides a basic job carrier with
  process identity (`CREATE_NEW_PROCESS_GROUP` and signal-marker exit codes in
  `internal/cli/carrier_windows.go`), but does not provide Unix
  process-group/TTY job control or a live signal proxy (`os/exec` rejects
  `ExtraFiles`) ([`docs/public-claims-evidence.md`](public-claims-evidence.md),
  [`docs/job-carrier.md`](job-carrier.md),
  [`docs/conformance-statement.md`](conformance-statement.md)). On Unix and
  Windows, a background job that survives a relayed (trapped or ignored) signal
  loses its external kernel carrier while remaining tracked by `wait`/`jobs`
  ([`docs/job-carrier.md`](job-carrier.md)).
- **`((` arithmetic vs. nested subshell ambiguity:** `((cmd)||(cmd))` and
  nested `( ( ... ) )` require spaces; the streaming, no-backtrack `mvdan/sh`
  parser does not disambiguate `((`
  ([`docs/conformance-statement.md`](conformance-statement.md)).
- **`<<${a}` expansion-shaped heredoc delimiters:** Rejected as a parse error
  (deliberate upstream `mvdan/sh` strictness)
  ([`docs/conformance-statement.md`](conformance-statement.md)).
- **Interactive Bash / Readline depth:** Selected PTY/ConPTY paths are covered
  by `TestInteractiveDepthPTY` and `tools/interactive-depth`, not full GNU
  Readline parity ([`docs/conformance-statement.md`](conformance-statement.md)):
  - Programmable completion has no readline `TAB` callback wired (`-F`/`-C`,
    `COMP_*`, and `compopt` are unproven; registered completion specs are lost
    in command substitution).
  - `bind` interpreter dispatch is a no-op (`-p/-l/-x/-r/-q/-u/-m/-f`);
    `READLINE_LINE`, `READLINE_POINT`, and `READLINE_MARK` have no `bind -x`
    integration.
  - `checkjobs` and `histappend` are not wired into the interactive exit/history
    loop; arbitrary history/fc flag combinations and Windows external-editor
    parity are not claimed.
- **Yash suite reporting:** No current assertion percentage is claimed from the
  historical `yash-chunks.json` record (50 shell-only fixtures completed on
  2026-07-20, excluding job-control/signal/TTY fixtures; fixture completion does
  not equal zero assertion failures)
  ([`docs/public-claims-evidence.md`](public-claims-evidence.md)).

### Security, supply chain, and containment boundaries

- **Secret output firewall:** The secret output firewall guarantee ends at
  `execve` ([`SECURITY.md`](../SECURITY.md)).
- **Checksum transit integrity only:** `checksums.txt` fetched from the same
  release verifies transit integrity, not cryptographic provenance
  ([`SECURITY.md`](../SECURITY.md)).
- **Genie `@guard` vs. `@sandbox` boundary:** Recorded on `v1.0.0-rc.27`
  ([`docs/releases/v1.0.0-rc.27-known-issues.md`](releases/v1.0.0-rc.27-known-issues.md),
  KI-2): bashy child processes inherit genie's `@guard` network deny and fail
  closed, while non-bashy child programs can bypass it unless run inside an
  explicit `@sandbox` with declared `@runs`. Status on the frozen `v1.0.0`
  candidate: `OPEN` (confirm against the frozen candidate known-issues ledger).
- **Scratch images are not sandboxes:** A `FROM scratch` image built with
  `bashy self image` narrows the supply-chain surface rather than providing a
  host isolation boundary
  ([`bashsharp/docs/claims.md`](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md)).

### Thin core, adapters, and provisioned toolchains

- **On-demand adapted commands:** `bashy` ships a thin core (`bsh`, agentic
  shell hooks, internal verbs, and dispatcher). Adapted commands under
  `optional/`, `toolchain/`, `yoke/`, `custom/`, and `examples/` are compiled
  on first use into the adapter cache and require a Go toolchain and adapter
  source on first use, including in `bashy_scratch`
  ([`CHANGELOG.md`](../CHANGELOG.md), `1.0.0-rc.40`).
- **Provisioned toolchains and runtime engines:** Fenced-island toolchains
  (Go, `zig cc`, uv-managed CPython, Node/TypeScript, `rustup`) and runtime
  engines (`podman`, `ollama`) are downloaded on first use, checksum-verified,
  and cached rather than bundled in the base archive
  ([`bashsharp/docs/claims.md`](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md),
  [`docs/licensing-supply-chain-policy.md`](licensing-supply-chain-policy.md)).
  Use `bashy check --prepare` or `bashy self bootstrap` ahead of offline use.
- **External command catalog and optional utility providers:** On the audited
  dependency pin in [`docs/public-claims-evidence.md`](public-claims-evidence.md),
  the visible agent-mode external catalog contains 49 names
  (`TestDocumentedExternalCount`), and the 116-row shipping utility inventory
  in [`docs/posix-command-coverage.md`](posix-command-coverage.md) retains 6
  optional names backed by pinned external providers (`ar`, `ctags`, `ex`, `nm`,
  `strip`, `vi`). Recount on the frozen `v1.0.0` candidate: `OPEN`.

### Bash# language profile and Go delta

Sources:
[`bashsharp/docs/go-delta.md`](https://github.com/bashsharp/bashsharp/blob/main/docs/go-delta.md),
[`bashsharp/docs/claims.md`](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md).

Bash# implements an enumerated Go profile (`go1.27-profile-v1`) rather than
claiming a percentage of Go or whole-module interpreted Go execution. The
single source of truth for differences from Go is `bashsharp/docs/go-delta.md`
(also queryable offline via `bashy explain go`), which enumerates **26 rows**
(**12 supported**, **3 limited**, **4 gap**, **7 excluded**):

- **Limited (3):** `M08` (goroutine started in mixed script text receives a
  snapshot of ordinary Go bindings; use a channel, a package-led Go unit, or a
  Go island for shared state), `M13` (`type NAME TYPE` parses as a Go type
  declaration; use `command type` for the shell builtin), `M14` (unquoted `//`
  at word start opens a comment in mixed text; quote `"//path"`).
- **Gap (4):** `M09` (top-level Go `if` blocks between shell commands; move
  into a `func` or Go unit), `M10` (`range` loops in a mixed-text `func` body;
  use an indexed `for` loop, Go unit, or Go island), `M11` (union type
  constraints `int | float64`; constrain with `any`/`comparable` or use a Go
  unit/island), `M12` (returning a struct field selector from a mixed-text
  `func`; use a Go unit or Go island).
- **Excluded by design (7):** `M15` (`package` after first statement), `M16`
  (raw-string import path), `M17` (`import "C"` in mixed script text), `U02`
  (`unsafe` pointer reinterpretation of interpreter-owned values), `I04`
  (aliased `package main` fence/embed), `I06` (running a non-`main` `.go` file
  as a program), `I07` (Go fence or embed without a nearest `go.mod` or
  `~~~gomod` fence).

Note: the `go-delta-gate.sh` 26/26 proof cited in `bashsharp/docs/claims.md`
was measured on one macOS arm64 build (`sh 2a6d42599`, 2026-10-08); three-OS
verification on the frozen `v1.0.0` candidate is `OPEN`.

## Go interpreted performance disclosure

Source:
[`bashsharp/docs/interpreted-performance-v1.md`](https://github.com/bashsharp/bashsharp/blob/main/docs/interpreted-performance-v1.md).

Bash# supports Go constructs in interpreted scripts and provides explicit
compiled Go execution for substantial computation. Large arithmetic loops,
recursive computations, heavy copying workloads, and contended shared-state
workloads can exceed the interpreted corpus gate's unchanged **60-second**
per-program acceptance limit.

### Verified full-workload completion (2026-10-05 measured candidate)

On the frozen Sprint 376 candidate (`sh 0e1b001086de86b660154b3f165e6a4e85008f34`,
language front end `2389bc254680b7cb67d1911a9dc578b6b426acdf`, interpreter
SHA-256 `1d2caf7618dd5af8a2ef983afbdfb7d874726692051b3668c849b5da48a57dab`,
Go 1.27.1 on Linux/amd64, 4 vCPUs / 12 GB RAM, no swap), all **12/12**
unmodified performance-limited workloads completed full-scale interpreted
execution with exact output matching native and compiled modes
(`bashsharp/docs/interpreted-performance-v1.md`):

| Root | Native wall (s) | Interpreted wall (s) | Compiled wall (s) | Interpreted max RSS (KiB) |
| --- | ---: | ---: | ---: | ---: |
| `64bit.go` | 13.753 | 30.334 | 3.122 | 320384 |
| `divmod.go` | 0.715 | 1677.493 | 0.816 | 97536 |
| `ken/divconst.go` | 0.515 | 441.246 | 0.665 | 121856 |
| `ken/modconst.go` | 0.515 | 413.482 | 0.665 | 125824 |
| `abi/fibish_closure.go` | 0.715 | 898.599 | 0.865 | 121728 |
| `abi/uglyfib.go` | 0.866 | 2232.902 | 1.066 | 120064 |
| `copy.go` | 0.515 | 244.226 | 0.765 | 123264 |
| `stack.go` | 0.415 | 94.667 | 0.565 | 440960 |
| `fixedbugs/issue13169.go` | 1.266 | 174.095 | 1.367 | 207564 |
| `fixedbugs/issue59680.go` | 0.615 | 391.293 | 0.816 | 420740 |
| `fixedbugs/issue78081.go` | 1.367 | 489.810 | 1.567 | 175960 |
| `fixedbugs/issue79186.go` | 13.212 | 822.579 | 1.967 | 131208 |

Notes from `bashsharp/docs/interpreted-performance-v1.md`:

- The ordinary **60-second** acceptance limit is unchanged. Diagnostic runs
  used an **1,800-second** per-stage override (and a separate **7,200-second**
  retry for `abi/uglyfib.go`, whose earlier **1801.516 s** timeout at
  **121728 KiB** max RSS remains recorded). Completion under a diagnostic
  budget proves full-scale correctness for that workload; it does not convert
  an ordinary 60-second gate timeout into a 60-second pass.
- `64bit.go` also passed the ordinary frozen-candidate interpreted gate in
  **34.13 s**; the supplemental **30.334 s** measurement verified both
  `runoutput` phases without replacing that gate record.
- Wall times include harness and compilation overhead (for example,
  `issue79186.go` native includes cache-building cost); RSS is GNU time's
  command/descendant max RSS. These figures are not a controlled
  native-vs-compiled speed benchmark.
- **Frozen `v1.0.0` candidate check:** `OPEN` (the measurement above was on
  Go 1.27.1; confirm unchanged pins or re-measure on the frozen `v1.0.0`
  candidate).

### Workaround and post-v1.0 follow-up

- **Compile the computation:** Run standalone Go files via the compiled route:
  ```sh
  bashy program.go
  bashy --source=go program.go
  ```
  Do not add `--bashsharp` or `--bashpp` when selecting this route, as those
  flags request interpretation (`bashsharp/docs/go-delta.md` row `I05`).
- **Aliased Go fence in a mixed script:** Place the compute-heavy loop and its
  working data inside an aliased compiled `~~~go as ALIAS` fence and call its
  exported function from the script so repeated bridge calls are avoided
  (`bashsharp/docs/interpreted-performance-v1.md`,
  `bashsharp/docs/go-delta.md` row `I01`). Nearest `go.mod` (or `~~~gomod`
  fence) and typed bridge restrictions apply.
- **Post-v1.0 work:** General interpreted loop/copy optimization is post-v1.0
  work; no JIT capability is promised for `v1.0.0`
  (`bashsharp/docs/interpreted-performance-v1.md`).

## Go corpus inventory

Sources:
[`bashsharp/docs/go-corpus-state-2026-10-05.md`](https://github.com/bashsharp/bashsharp/blob/main/docs/go-corpus-state-2026-10-05.md),
[`bashsharp/docs/go-corpus-state-2026-10-05.tsv`](https://github.com/bashsharp/bashsharp/blob/main/docs/go-corpus-state-2026-10-05.tsv),
[`bashsharp/docs/go-corpus-exclusions.tsv`](https://github.com/bashsharp/bashsharp/blob/main/docs/go-corpus-exclusions.tsv),
[`bashsharp/docs/claims.md`](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md).

> **Candidate refresh status: `OPEN`.** The inventory below is the latest
> published full-corpus ledger (2026-10-05, Go 1.27.1). Refreshing this
> inventory from the frozen `v1.0.0` candidate (or verifying identical engine
> and Bash# pins under the release policy) is required before promotion.

### Measured candidate identity (2026-10-05 ledger)

- Engine (`sh`): `0e1b001086de86b660154b3f165e6a4e85008f34`
- Language front end (`bashsharp`): `2389bc254680b7cb67d1911a9dc578b6b426acdf`
- `bashy`: `809a6c6f3fd8cda0b69408d4db84c25b0e6f32ef`
- Harness: `260dee6f8319b844855f78be12cb7ff7bc57d10a`
- Interpreter SHA-256: `1d2caf7618dd5af8a2ef983afbdfb7d874726692051b3668c849b5da48a57dab`
- Environment: Go 1.27.1, Linux/amd64, 4 vCPUs, 12 GB RAM, no swap, `GOMAXPROCS=2`

### Lane totals (`bashsharp/docs/go-corpus-state-2026-10-05.md`)

Total observed inventory: **3,653** observed IDs (**3,497** product inventory =
**3,458** native-applicable roots + **39** expected skips; plus **156**
native-only typechecker roots receiving zero product credit).

| Lane | Mode | PASS | FAIL | SKIP | NATIVE-ONLY | INCOMPLETE |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| `testdir` | native | 2,691 | 0 | 37 | 0 | 0 |
| `testdir` | interpreted | 2,419 | 272 | 37 | 0 | 0 |
| `testdir` | compiled | 2,691 | 0 | 37 | 0 | 0 |
| `typechecker` | native | 741 | 0 | 2 | 156 | 0 |
| `typechecker` | interpreted | 741 | 0 | 2 | 156 | 0 |
| `typechecker` | compiled | 741 | 0 | 2 | 156 | 0 |
| `package` | native | 26 | 0 | 0 | 0 | 0 |
| `package` | interpreted | 17 | 9 | 0 | 0 | 0 |
| `package` | compiled | 26 | 0 | 0 | 0 | 0 |

Summary across the **3,458** native-applicable product roots:

- **Native oracle:** **3,458 / 3,458 PASS** (39 expected skips)
- **Lowered-then-compiled Bash#:** **3,458 / 3,458 PASS** (39 expected skips)
- **Interpreted Bash#:** **3,177 PASS · 281 FAIL · 39 SKIP** (**0** new
  failures, **0** regressions, **0** missing IDs, **0** failing rows outside
  `bashsharp/docs/go-corpus-exclusions.tsv`)
- **Tutorials on the same candidate:** Tour of Go **97 / 97** and Go by Example
  **85 / 85** passed in all three modes (`bashsharp/docs/go-corpus-state-2026-10-05.md`,
  `bashsharp/docs/claims.md`).

### Breakdown of the 281 interpreted exclusions (`bashsharp/docs/claims.md`)

All 281 rows fail interpreted mode only and pass native and lowered-then-compiled
modes; each is keyed by exact root ID in `bashsharp/docs/go-corpus-state-2026-10-05.tsv`
and `bashsharp/docs/go-corpus-exclusions.tsv`:

| Exclusion category | Roots |
| --- | ---: |
| Compiler diagnostics (asserts a `gc` error message) | 108 |
| Assembly checks (`asmcheck`) | 75 |
| `gc`-only checks | 30 |
| `unsafe` memory reinterpretation (no native byte layout for interpreter-owned values) | 20 |
| `gc` observation | 16 |
| Compute-bound (correct at full scale; exceeds 60 s acceptance limit) | 11 |
| Whole-package Go source (compiled path is the supported route) | 9 |
| `cgo` | 8 |
| Assembly input | 4 |
| **Total** | **281** |

No universal Go percentage is claimed from this corpus denominator; whole-corpus
3,497/3,497 is not claimed (`bashsharp/docs/claims.md`).

## Open items to finalize on the frozen v1.0.0 candidate

The following items are **`OPEN`** in this draft and must be completed before
promotion:

- `OPEN` — Frozen `v1.0.0` candidate commit SHA, tag, release date, and pinned
  module coordinate table.
- `OPEN` — Operator insertion of the POSIX certification statement from the
  authority's registration (see [Certification](#certification)).
- `OPEN` — Refreshed Go corpus inventory and performance verification against
  the frozen `v1.0.0` candidate (Go 1.27.4 coordinate).
- `OPEN` — Candidate gate evidence table: unit tests, sealed both-mode GNU
  Bash 5.3 suite, tag conformance, journey/smoke e2e, permissive license gate,
  SPDX 2.3 SBOM, `govulncheck` receipt, per-OS native QA (Linux, macOS,
  Windows), artifact checks (`bashy --version`, `go version -m`, `elfaudit`,
  `bashysignalprobe`, `otool -L`), and external command catalog recount.
- `OPEN` — Browser extension publication references and store links (if
  included in final release notes).
- `OPEN` — Candidate known-issues ledger verification (including KI-2 `@guard`
  status).
- `OPEN` — Hosted website publication of
  `https://dhnt.io/.well-known/security.txt` (requires a hosted change outside
  this repository).
- `OPEN` — Operator review and explicit approval of every external string prior
  to tagging and promotion.
