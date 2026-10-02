# Sprint 355: single Bashy startup investigation

This note records a bounded startup experiment for the one-file `cmd/bashy`
payload. It is not a Profile D result. No licensed test was run for this
experiment, and the macOS timings do not establish the cause of the Linux
`kill:9` failure.

## Measurement

On macOS arm64 with Go 1.27.1, build `cmd/bashy`, `cmd/sh`, and standalone
Coreutils from the Sprint 355 signal-fix source (`bashy` `9fe58aa`), then invoke
the combined executable through `sh` and `kill` symlinks. After ten warmups,
run 120 randomized, interleaved subprocess launches per route with output
discarded. Each observation measures launch through exit, including the
Python subprocess call overhead. The small route commands are `sh -c :` and
`kill -l STOP`.

| Route | Size | Median | p99 |
| --- | ---: | ---: | ---: |
| Lean `cmd/sh` | 22 MB | 3.755 ms | 4.557 ms |
| Combined `sh` alias | 161 MB | 20.116 ms | 21.835 ms |
| Standalone Coreutils `kill` | 38 MB | 6.127 ms | 6.806 ms |
| Combined `kill` alias | 161 MB | 19.911 ms | 21.957 ms |

`GODEBUG=inittrace=1` reported 763 initialized packages, 18.16 MB of init
allocations, and 126,213 init allocations for the combined `sh` alias. The
lean shell initialized 71 packages, allocating 0.129 MB across 893
allocations. The largest measured combined package initializers were
Filebrowser `users` (4.2 ms, 6.8 MB), `modernc.org/libc/.../netdb` (3.9 ms,
3.66 MB), Excelize (1.8 ms, 0.98 MB), and Filebrowser `files` (1.1 ms, 0.96
MB). These package clock values are diagnostic estimates, not a sum of process
wall time.

Filebrowser `users` built its 100k common-password map during package init,
although only `ValidateAndHashPwd` reads it. Filebrowser commit `0b059ae2`
loads the map through `sync.Once` on first validation. Its known-password,
unknown-password, and concurrent first-use test passed with `go test -race
./users`. Bashy commit `e0594f9` pins the Filebrowser change, and `go test
./cmd/bashy` passed with that sibling. In a second 120-launch paired run:

| Route | Before median / p99 | Lazy map median / p99 |
| --- | ---: | ---: |
| Combined `sh` alias | 20.521 / 22.428 ms | 16.939 / 18.171 ms |
| Combined `kill` alias | 20.342 / 21.808 ms | 16.756 / 18.032 ms |

The combined init trace fell to 11.35 MB of allocations; the Filebrowser
`users` initializer disappeared. A single `/usr/bin/time -l` sample reported
maximum resident size of 60.3 MB before and 52.5 MB after the change. This is
only a spot check, not a memory distribution.

## Options and constraints

Go programs are already compiled ahead of time to native code, so adding a
JIT would add work to this short-lived shell/utility startup path. Go
initializes every statically imported package before `main`; branching on
`argv[0]` in `main` cannot defer transitive package init. The practical
near-term method is to move expensive setup behind its first relevant call,
as with the password map, while keeping one physical executable and all
functions. [Go FAQ](https://go.dev/doc/faq),
[Go program initialization](https://go.dev/ref/spec#Program_initialization),
[Go init diagnostics](https://go.dev/doc/diagnostics).

Go plugins load from a separate filesystem artifact and carry strict
toolchain/dependency matching restrictions, so they do not satisfy the current
one-physical-executable certification route. Profile-guided optimization can
improve representative hot runtime code, but it does not remove mandatory
package initializers and needs a representative profile; it is not the first
fix for this measured startup cost.
[Go plugin documentation](https://pkg.go.dev/plugin),
[Go PGO guide](https://go.dev/doc/pgo).

Stripping the local build with `-ldflags='-s -w'` reduced its size from 161 MB
to 122 MB but changed median startup by only about 0.3–0.4 ms in a paired run.
Do not infer that it is safe for the Linux certification build: Coreutils'
Linux inherited-signal route currently reads `runtime.fwdSig` from the ELF
symbol table and fails closed if the symbol is absent. The build flags need a
separate signal-semantics check before use.

Filebrowser `files` registers MIME extensions in global `mime` state during
init. Deferring that registration could change callers outside Filebrowser,
so it has not been changed. The `modernc` netdb initializer populates exported
protocol/service tables used by libc and also has not been changed; its
presence and cost on Linux require a Linux-specific trace. Neither cost was
assumed safe to defer merely because shell and `kill` do not call those APIs
directly.
