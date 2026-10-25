# S503.6 Linux musl second hosted probe: C-driver verbose flag failure

**Review state:** failed experiment; `probe_passed=false` and
`product_admitted=false` on both targets. No Whisper ELF, final link map, or
runtime inference resulted from this run.

The [manual GitHub Actions run](https://github.com/qiangli/bashy/actions/runs/40218694323)
used Bashy commit `c97e31b1ce8478b74e38b36c6130dc4457fd41b8`. Its native
`ubuntu-24.04` amd64 and `ubuntu-24.04-arm` arm64 jobs retained artifact IDs
`12608950095` and `12608299588`, respectively.

| Native target | `configure.log` SHA-256 | `probe-report.json` SHA-256 |
| --- | --- | --- |
| amd64 | `22af954a419a1898d3ca52a447ff072c04c7ea2cac6e3aaaebc6126ce6072027` | `4d2a02351a8335aeb839f414a74771d775ff8f88e7225d901254a90ab1acc466` |
| arm64 | `22f8a6bcc43dfeac720e1527975a136b2774e3e9cd3afb55ee5baeb2197e0891` | `6ee0c62655c71015f48e25e8d3624e573be42c0eea29234c34d8d653d77ee976` |

Both configure logs fail the first CMake C compiler `TryCompile` link after
invoking `zig cc -static -Wl,--print-map --verbose-link ...`: `error: Unknown
Clang option: '--verbose-link'`. Zig's build-mode argument parser accepts
`--verbose-link`, but its C-driver compatibility parser rejects it.
The [Zig 0.15.2 C-driver source](https://github.com/ziglang/zig/blob/0.15.2/src/main.zig)
maps its accepted `-v` option to `verbose_link=true` and forwards it to
Clang. The correction substitutes `-v` and adds a bounded native smoke link
of one trivial C source before CMake. It retains the smoke log, map, and LLD
command; it stops before the full build unless all three appear. This route
is source-supported, pending real native evidence.

Both reports retained the pinned Zig and whisper.cpp archive identities.
Their minisign logs verify the Zig signatures, and all nine retained Zig
source/notice anchors rehash to the report entries. These input checks do not
establish the linked runtime license closure or product acceptance. A new
hosted run requires manager review and coordination.
