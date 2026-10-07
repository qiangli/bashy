# Product startup signal contract

The inherited SIG_IGN contract must hold before package initialization and
before shell routing, including a loaded startup. Capturing dispositions in a
C constructor or recovering runtime.fwdSig after startup does not enforce it:
Go replaces ordinary inherited ignores. Blocking them in a launcher also fails
because Go unblocks fatal signals on its threads.

The product build now uses `scripts/go-product.sh`. It creates a private source
overlay for Go 1.27.1's `runtime/signal_unix.go`, verifies the original SHA-256,
and enables the change with `-X runtime.bashyInheritedIgnore=1`. The installed
GOROOT is never written. An unknown version/source is an error. The generated
source retains the Go authors' copyright and BSD license notice; the Go runtime
remains covered by its existing license attribution. No library is added.

The change is deliberately confined to `sigInstallGoHandler`: preserve an
inherited ignore only for `_SigNotify` signals without `_SigPanic`, `_SigUnblock`
or `_SigSetStack`, excluding preemption, profiling and per-thread syscall
signals. It is disabled for shared/archive builds and defaults off without the
link flag. Go's normal initialization records the ignored bit. Explicit
`os/signal.Notify` can install a handler, and `Stop` restores the inherited
ignore. Default dispositions and synchronous faults keep their stock behavior.
The caller's environment and thread signal mask are not changed.

This covers ordinary asynchronous signal numbers, including ABRT, QUIT and
TERM. It does **not** promise pre-main ignore for user-generated synchronous
fault signal numbers such as SEGV/FPE, or runtime-reserved signals.

The root `./bashy` bootstrap, Make host/FIPS/dist/scratch, artifact builds, native Darwin release and
GoReleaser use the product wrapper. Windows uses the unmodified runtime. A
plain upstream `go build` is a developer build and does not provide this early
startup contract. Nested signal regression builds use the same product wrapper.
The wrapper supports `build` only; `run`/`test` are rejected rather than
misinterpreting a target program's arguments. It accepts explicit linker flags
but rejects changes to the runtime activation, caller overlays, and GOFLAGS
linker overrides. It requires the selected toolchain to be Go 1.27.1 and fails
on a newer selected toolchain instead of downloading or switching silently.
Artifact/DAG callers supplying BASHY_EXE keep that managed Go executor for both
the driver bootstrap and every Go subprocess, even without host Go on PATH.

Validation plan: first reproduce on the unchanged runtime with the same Go
version, then repeat isolated and under load on Linux. The test-only
`startup_signal_probe` dependency stops before main; the parent waits for that
stop, queues the signal and resumes the child. This removes startup timing from
the assertion. The test then attempts trap/reset and another delivery. Guards
cover default TERM, Notify/Stop, the disabled overlay and a real nil-pointer
fault. Required remote full tests and the release build matrix remain gates;
a focused pass is not a full-suite pass.

Bootstrap follow-up: both managed and plain-Go bootstrap paths use the product
builder. Explicit BASHY_EXE takes precedence over BASHY and PATH discovery.
The focused bootstrap regression executes the real entry script with a
recording builder and checks all four selections, build flags and forwarded
arguments. Against the previous entry script all four cases fail by invoking
Go directly; with the correction all four pass. On Darwin/arm64,
`go test ./tools/productgo -count=1 -v` passes (5.475s), including the runtime
ignore/Notify/Stop/fault controls and managed executor without Go on PATH.
These focused results do not supersede the outstanding Linux full-suite gate.
