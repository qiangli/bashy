# S407.6: remove Unicode table construction from process startup

Sprint #407, Story #1949 (`621f736b095e`). The rc.14 Profile D replay found
that every Bashy applet and background job carrier paid the `go-runewidth`
v0.0.27 `init()` cost. The certification host's `inittrace` attributed 39 ms
of 46.5 ms total to that package, and its 100-job loop took 6.79 seconds.

## Implementation

1. `scripts/test-startup-linux.sh` runs the built base-profile binary with
   `GODEBUG=inittrace=1`. It fails if a package takes over 5 ms, total package
   init takes over 15 ms, or 100 `/bin/true & wait $!` jobs take over 5 s. It
   also prints the mean cost of 20 `bashy -c true` processes. `make
   test-startup-linux` builds the cert-tagged binary, and `make test` includes
   the gate on Linux.
2. The MIT-licensed upstream v0.0.27 source is forked at
   `yoke/external/runewidth`. Its `init()` still resolves the locale. A
   `sync.Once` in `Condition.RuneWidth` builds the identical merged tables and
   strict LUTs on the first width query. The copied upstream tests check full
   Unicode checksums; a fresh-process test checks laziness and a sample with
   combining, ideographic, East Asian ambiguous, and emoji-neutral runes.
3. Bashy and standalone ycode pin the fork by versioned replacement. Yoke's
   own module replaces it with its local nested module. Coreutils remains on
   upstream v0.0.23, which has no strict LUT and needs no change.

## Linux evidence

Measured on the same Ubuntu 26.04 arm64 Lima VM, using Go 1.27.2 and native
`CGO_ENABLED=1` builds with `bashy_cert,bashy_cert_base,osusergo`. The product
build wrapper supplied its usual startup overlay. Timing is one 20-process or
100-job run per candidate; scheduling can vary between runs.

| Measurement | Before | After |
| --- | ---: | ---: |
| Package init total, 330 packages | 17.43 ms | 4.26 ms |
| Largest package init | `go-runewidth` 13.00 ms | `x509roots/fallback` 0.59 ms |
| `bashy -c true`, 20-process mean | 19.81 ms | 4.94 ms |
| 100 `/bin/true & wait $!` jobs | 1.93 s | 0.43 s |

The new gate fails against the before binary (separate trace: 29.87 ms total,
16.00 ms `go-runewidth`) and passes against the fixed binary (4.26 ms total,
0.59 ms maximum, 4.94 ms/process, 0.43 s/jobs). The next-largest package init
costs in the after trace are under 1 ms, so there is no other cheap table-build
cost to trim in this base profile. `make test-bash` on the same Linux VM passed
86/86 fixtures with the current shell source and pinned interpreter.

The optional full AgentOS build has a separate expensive init in
`github.com/qiangli/bonsai/x` metrics registration (84 ms in one VM trace).
That package is absent from the 330-package cert base and its collector
registration is unrelated to the runewidth fork; a separate AgentOS startup
story should assess moving the database metrics registration to first use.

The original Ubuntu 24.04 x86_64 certification host has not run the fixed
candidate yet. Its reported pre-fix values were 46.5 ms total init, 76.6 ms
per `bashy -c true`, and 6.79 s for 100 jobs. The proprietary VSC awk set
must be replayed there to establish the 600-second cap; this VM measurement is
the public startup regression evidence, not a certification result.

The optional full AgentOS standalone build was attempted on Darwin but its
linker stopped with `no space left on device` (the host had under 1 GB free).
The focused base-profile standalone test passed; the full-profile build still
needs a host with working space. Its separate `bonsai/x` cost remains outside
this base-profile startup acceptance.

## Producer order

- Yoke fork: `d4224e17d6f6522b6cedd86c556206d538e8bd87`
  (`agent/weave-issue-280-runewidth`)
- Ycode replacement: `eec8245ade514232cffd483060200bab7ec9587b`,
  checksums: `692f9c92af6f28ddf4b3f92888a757e50acd13b0`
  (`agent/weave-issue-280-runewidth`)
- Bashy replacement and gate: this branch, after both producer commits

The worker does not push. The conductor must publish the yoke fork commit
before a normal, cache-cold standalone `go` build can fetch its pseudo-version.
