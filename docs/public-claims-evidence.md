# Public claims and their evidence

Sprint 379 B6 (Story 1515, `34ea7be6eeec`), reviewed 2026-10-09.

The audit plan is to compare public wording with the pinned source and committed
reports, correct current claims, label historical snapshots, and check the claims
with the existing release-package and command-catalog tests. This is a source
and documentation audit, not a new release-candidate conformance run.

## Job control

Unix process groups, stopped-state tracking and controlling-terminal handoff
have shipped. Evidence: [job-carrier.md](job-carrier.md),
[`internal/cli/carrier_unix.go`](../internal/cli/carrier_unix.go),
[`internal/cli/carrier_unix_test.go`](../internal/cli/carrier_unix_test.go), and
[`internal/cli/jobcontrol_autoMonitor_vsc_unix_test.go`](../internal/cli/jobcontrol_autoMonitor_vsc_unix_test.go).

Windows does not provide Unix process-group/TTY job control; its basic job
carrier provides process identity but no live signal proxy (`os/exec` rejects
`ExtraFiles`). This is the precise carrier boundary described in
[job-carrier.md](job-carrier.md) and
[`internal/cli/carrier_windows.go`](../internal/cli/carrier_windows.go), not a
claim that Windows has no jobs or process identity.

## Yash: no current assertion percentage claimed

[`yash-chunks.json`](../yash-chunks.json), `measurement`, records a 2026-07-20
run of 50 shell-only POSIX fixtures against yash source
`7070575eec5accee71cbaa0f46accd970c9e8888`: 50 completed, zero failed, skipped or
timed out. Job-control/signal/TTY fixtures are excluded from that corpus.
[report-yash-dks-bpath.md](report-yash-dks-bpath.md) explains that the historical
runner could report fixture completion even when individual assertions failed.
Do not convert 50/50 into an assertion conformance rate.

The older [panel report](cross-shell-conformance-baseline.md) records
1763/1826 and 1777/1838 assertions in different environments, with percentages
truncated to whole numbers. Those dated results, earlier baselines and later
case-specific bug reports do not yield one current RC percentage. The old
README claim of an assertion rate above 99 percent had no matching committed
measurement. Current public summaries therefore link evidence without a rate.
Historical reports keep their measured counts and versions, explicitly labeled
as snapshots; changing them would falsify the evidence.

**Recheck on RC:** on the approved Linux test host or CI, using a checkout of
that RC, run `bashy dag dag.md yash` (the target runs
`scripts/yash-posix-suite.sh`). Preserve the candidate commit, testee digest,
yash revision, each panel's OK/ERROR denominator, exclusions and full logs.
The host-installed `bashy` is only the DAG driver; the script builds `cmd/bash`
from the checkout. This does not run the excluded interactive fixtures.

## External commands: count the declared set

At the pinned dependencies in [`go.mod`](../go.mod), the visible shipped
external catalog has **49 names** across platforms with agent-mode provisioners
enabled. This excludes aliases, experimental entries and the operator's
registered commands. The source is `classSectionsOn(true, "any")` in
[`internal/agentos/commands_sections.go`](../internal/agentos/commands_sections.go),
which derives its entries from `liveAtlas`. The
`TestDocumentedExternalCount` unit test compares this set with the public count
in [command-atlas.md](command-atlas.md), and logs its complete membership.

Two narrower registries must not be confused with that count:

- The pinned Yoke `external/registry.Names()` has **6** entries:
  `doctl gcloud gitea ollama rg tofu`. This is the declarative managed-CLI
  registry, not every external/provisioner implementation or public origin.
- The POSIX external-provider subset has **10** names:
  `ar ctags ex localedef lp m4 man nm strip vi`. The other 39 external names
  are outside that POSIX subset. The 116-name POSIX inventory has a different
  denominator; see [posix-command-coverage.md](posix-command-coverage.md).

Reproduce the source count with:

```sh
go test ./internal/agentos -run '^TestDocumentedExternalCount$' -count=1 -v
```

**Recheck on RC:** `BASHY_AGENTIC=1 "$RC_BASHY" commands --view external --os any --json`
records the candidate's external view; count its visible records, excluding
hidden aliases and experimental entries. Repeat the unit command above in the
RC checkout so a pin change cannot silently preserve a stale published count.
`RC_BASHY` denotes the executable extracted from that candidate's archive.

## Build linkage and versions

The shell and userland are Go. [`.goreleaser.yaml`](../.goreleaser.yaml) uses
`CGO_ENABLED=0` for Linux/Windows builds. The native Darwin job in
[release.yml](../.github/workflows/release.yml) invokes
[`build-native-darwin-release.sh`](../scripts/build-native-darwin-release.sh),
which builds **`cmd/bashy` with `CGO_ENABLED=1`**. It links the project's
`pre-Go C constructor`, `snapshot_inherited_ignores`, from
[`inherited_ignore_cgo_unix.go`](../cmd/bashy/inherited_ignore_cgo_unix.go).
The constructor calls `sigaction` before Go runtime initialization to remember
inherited `SIG_IGN` dispositions, which the runtime can otherwise change.
The Darwin artifact is therefore not a fully static, CGo-free executable.
The separate Darwin `bash`, `sh` and `outpost` products use `CGO_ENABLED=0`.

[`tools/elfaudit/main.go`](../tools/elfaudit/main.go) rejects a Darwin `bashy`
without CGo and the retained Mach-O constructor symbol. The constructor is
separate from the optional local `native/siglaunch.c.in` launcher and from
Bash's test-fixture C helpers. FIPS selects the Go cryptographic module; it
does not remove this platform linkage requirement.

The module declares **`go 1.27` / `toolchain go1.27.1`**. Release CI selects
Go 1.27.1. Bash compatibility remains **5.3**, with the
`5.3.0(1)-bashy-<tag>` release stamp specified by the release scripts.
[`internal/cli/version.go`](../internal/cli/version.go) uses
`5.3.0(1)-bashy` for an unstamped development build. Do not hard-code a latest
Bashy product version into public examples: use the release badge/tag and
`bashy --version`. Versioned release notes and captured test transcripts are
historical records, not current toolchain requirements.

**Recheck on RC:** run `"$RC_BASHY" --version`, `go version -m "$RC_BASHY"`,
`go run ./tools/elfaudit --bashy-signal "$RC_BASHY"`, and
`go run ./tools/bashysignalprobe "$RC_BASHY"` on the native approved test host.
Use `otool -L "$RC_BASHY"` on macOS to record the actual dylib dependency list;
source inspection alone does not establish that list. These artifact checks
must precede a claim about the RC's build settings or inherited-signal behavior.
