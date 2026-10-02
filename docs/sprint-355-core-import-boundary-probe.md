# Sprint 355 core import-boundary probe

**Story:** #392 (`65684e7f4df6`). **Status:** implementation probe; the
`bashy_core` build tag is not a release profile. The default `cmd/bashy`
continues to ship its full front door.

Go initializes every imported package before `main`, so choosing a shell or
applet route from `argv[0]` cannot defer AgentOS/ycode/Genie initialization
inside today's full binary. This probe compiles one physical executable with
shell, Coreutils, and minimal registered-command CRUD while excluding those
optional imports. It establishes a measurable target for the later migration
of optional features to records and on-demand payloads.

The tag selects `cmd/bashy/main_core.go`; common route and inherited-signal
logic live in `cmd/bashy/route.go`. `internal/core` wires Coreutils before the
registered ring and PATH, and uses the existing `yoke/pkg/fleet` CRUD tree and
record schema. The `sh` and `kill` symlinks resolve to the same executable
inode. `VSC_PROFILE=cert` excludes registered lookup, as in the full profile.
The route test covers add, set, show, verify, rm, in-shell and front-door
execution, `type`, and cert exclusion. Linux pure-Go and native Darwin cgo
artifacts both pass the inherited-signal structural gate.

| macOS/arm64, 120 interleaved warm launches | Full Bashy | Core probe | Lean `cmd/sh` |
| --- | ---: | ---: | ---: |
| `sh -c true` median | 16.354 ms | 6.640 ms | 3.947 ms |
| `kill -l 9` median | 16.104 ms | 6.338 ms | — |
| Mach-O size, `-ldflags=-w` | 129 MB | 31 MB | 16 MB (`-s -w`) |

Both Bashy variants used `CGO_ENABLED=1` on macOS; the core probe was also
cross-built for Linux/amd64 with `CGO_ENABLED=0`, `-ldflags=-w`, and measured
34 MB. `go list -deps` returned 1611 packages for full Bashy and 537 for
the core tag. `GODEBUG=inittrace=1` on the `sh` route reported 762 package
init lines and 11,345,608 allocated bytes for full Bashy, versus 315 lines
and 1,695,336 bytes for the core tag. The launch benchmark warmed each route
ten times, then shuffled five routes per round with a fixed seed for 120
rounds, using Python `subprocess.run` and monotonic time. These numbers are
comparative on one Mac; the Linux startup comparison awaits a suite-free host
window after the licensed focused replay.

The core tag intentionally omits the current optional front-door verbs and
full Command Atlas presentation. Its collision filter covers the base shell,
Coreutils, and core CRUD words; optional verb collisions are a release blocker
until those verbs are catalog records. The full profile's command CRUD remains
unchanged. Stories #393–#395 cover the generic language/toolchain registry,
an external runner proof, and the optional-only digest/route gate. A full
Profile D rerun still requires the separate 64-blocker/9-INSPECT accounting
gate.
