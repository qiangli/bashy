# S503.6 Linux musl third hosted probe: link built, audit stopped on libzigc

**Review state:** both native jobs failed closed at link-input classification.
`probe_passed=false` and `product_admitted=false` for both targets. No
runtime inference, installation, or product acceptance was performed.

The [manual GitHub Actions run](https://github.com/qiangli/bashy/actions/runs/40218975176)
used Bashy commit `8175d4219725c660f867d1847dc53b66735a254f`.
The retained amd64 and arm64 artifact IDs are `12609031122` and
`12609300757`, respectively. Both tiny flag preflights passed, CMake
configured, and the full `whisper-cli` target linked. Each artifact contains
the preflight log/map/LLD command, final build log/map/command, and ELF.

| Native target | `probe-report.json` SHA-256 | `whisper-cli` SHA-256 | final map SHA-256 |
| --- | --- | --- | --- |
| amd64 | `86da5d805a2a8016219894780f5f71d4ed581f42c99cc5d078dab9ca41b2c9ec` | `3f645450720f14df65f0e838321a88543ed70770d4bd38b975b22df2c9283bdb` | `508a8c95bf1dfc1482e00aca5dd14bf51c40b593ac7b0c1883f9cd2492c5835e` |
| arm64 | `2e168040ea402da7dbeacf406339192ce4d026b02df07a763b58ca937f4a6719` | `800b28881e3c7e99cbafe9f7d6b219c2969473806f354ebc5244791cf419809b` | `3426b0e1b27b2164d6f5ef6a1b39fecab5e75b52ffb4185217bbb978b3cb2eb1` |

Both reports stop at `unclassified Zig link input; source review required:
.../zig-cache/o/.../libzigc.a`. The inventory had not finished, so neither
ELF audit nor help smoke ran; these ELF bytes are **experimental**. The final
map plus LLD argv enumerate 13 distinct inputs per target. The only
unclassified basename was `libzigc.a`; all other names match the existing
Whisper build or Zig musl/libc++/libc++abi/libunwind/compiler-runtime cases.
Each map shows a single loaded `libzigc.a` member, `libzigc_zcu.o`, with
`strcmp`, `strlen`, `strncmp`, and `strcasecmp` code sections.

Zig 0.15.2 [Compilation.zig](https://github.com/ziglang/zig/blob/0.15.2/src/Compilation.zig)
queues `buildLibZigC` for static musl and builds it from the Zig library root
`lib/c.zig`. That root imports `lib/c/{inttypes,stdlib,string,strings}.zig`;
each imports `lib/c/common.zig`. The linked string functions use the Zig
standard library `mem` and `ascii` modules. The distribution's top-level
`LICENSE` is MIT/Expat. I independently downloaded both exact Zig 0.15.2
archives, checked their pinned sizes and SHA-256 values, and compared these
source/notice bytes. The two distributions contain identical copies:

| Source/notice path | SHA-256 |
| --- | --- |
| `LICENSE` | `5c537d6853e005298a285d508cff9ac7192cea23576c840d485b2b586a7ff177` |
| `lib/c.zig` | `b39fec9941872a61508e463ee0bc35087bf7613b56c95f1193c93c2d1a59ad9c` |
| `lib/c/common.zig` | `7d4484d9e301f122f1a5be474d72df28ace94cd159274c698937f5a616684d0b` |
| `lib/c/inttypes.zig` | `246d6bad1f2172393c86f461db87aae744213e7486b1fbe36b4f7931922d2e54` |
| `lib/c/stdlib.zig` | `f077ab46b0e5137d003962a26c058b65eb2e5b36bc37bc1d578e46fd0e1f8e7b` |
| `lib/c/string.zig` | `be4e20409e7eebacd77e982fd10e7cf1dbe888a6b24280f5c11c4fd508d83c02` |
| `lib/c/strings.zig` | `a9728029705607997f8302e6cab5e8f7f61b19bc324cf1a76790cf6ad7a93db8` |
| `lib/std/std.zig` | `b78c7a6dec7a3ea3381bae30533309bd8de71994422c3f87fc9760e378c7e427` |
| `lib/std/mem.zig` | `d4b891a60b1bb54e106c56d626dfd575ed39a85b1a09795b9c0a534fc10fd9ca` |
| `lib/std/ascii.zig` | `245e0e29ff75a5cbcf294bade7a32b35d1a5e0242bad59342822831c56f97b88` |
| `lib/std/c.zig` | `a8138d10955a61fae402e1235e81685732f46e4cacca1e922609034183824bb1` |

The correction recognizes only `libzigc.a` under the verified Zig cache and
only when the map and `ar t` both identify its sole `libzigc_zcu.o` member.
It retains the actual generated archive and SHA-256 in the link-input
inventory, as well as the ten source anchors and MIT notice above. Any different
member or unresolved input still stops the probe. The third run could not
retain `libzigc.a` because classification raised before input retention;
the next native run must provide its exact bytes and hash. This review does
not claim complete license closure or static ELF acceptance.
