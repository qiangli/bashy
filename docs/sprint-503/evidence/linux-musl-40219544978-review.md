# S503.6 fourth native Linux musl probe review

**Result:** [run 40219544978](https://github.com/qiangli/bashy/actions/runs/40219544978)
at Bashy `f4f4c9400012ba296106f828a25d1ecc82289f10` failed on both native
targets at the final undefined-symbol check. The independently checked
[review JSON](linux-musl-40219544978-review.json) records all hashes and
inputs. Both reports remain `probe_passed=false` and `product_admitted=false`.

| Target | Artifact ID | Uploaded ZIP SHA-256 | ELF SHA-256 | `libzigc.a` SHA-256 |
| --- | ---: | --- | --- | --- |
| amd64 | `12608911292` | `7d62864205c889350400bc8bb4c82dc9659a6c98dbe69644af8c8ebf5218eb63` | `3f645450720f14df65f0e838321a88543ed70770d4bd38b975b22df2c9283bdb` | `cb261058d51c96e0c69882098a5fcda244654f18cbb2466b9eae0b587f28ab0d` |
| arm64 | `12608364976` | `168a260302738cfa2be51eca6f7d7d394505d03b1e548201f429e0207ed4cd1e` | `800b28881e3c7e99cbafe9f7d6b219c2969473806f354ebc5244791cf419809b` | `c0d1134c22d77bab4c4dd4edd33bed03f271cb959298c3c6523d5c5fed6a3577` |

Both pinned Zig distribution archives match their expected sizes and
SHA-256 values. The runner minisign logs report verified archive and comment
signatures; all 19 retained notice/source anchors independently rehash to
both report entries and the exact pinned archives. The reports record the
pinned whisper.cpp source SHA-256 and retain its license, but the whisper.cpp
source archive itself was not uploaded for an independent byte rehash.

Both full link maps and LLD commands enumerate 13 inputs. Every retained
input was rehashed, the maps' loaded members match the inventory, and no
GCC/glibc path appears among those inputs. I decoded the retained GNU ar
bytes independently: each `libzigc.a` contains only its mapped
`libzigc_zcu.o` member. Its member SHA-256 is
`d9cb640a74d0e77ffd31643812e656e801d89491538814be9c856be80b1b5e91`
on amd64 and
`18da21db2e13dd1643f9115909f3dd2129445f31c9671674d4127a465baf75ac`
on arm64.

Independent ELF parsing finds `ET_EXEC` for the expected machine, no
`PT_INTERP`, `PT_DYNAMIC`, `.dynamic`, or GNU version sections. The retained
`readelf` output likewise reports no dynamic section or version information.
The only named undefined symbol is `_DYNAMIC`, shown by `nm -u` as
`U _DYNAMIC` and by `readelf -s` as `LOCAL HIDDEN UND _DYNAMIC`. The exact
Zig 0.15.2 musl [x86_64 startup header](https://github.com/ziglang/zig/blob/0.15.2/lib/libc/musl/arch/x86_64/crt_arch.h)
and [aarch64 startup header](https://github.com/ziglang/zig/blob/0.15.2/lib/libc/musl/arch/aarch64/crt_arch.h)
declare that symbol weak and hidden. Linked
[TLS initialization](https://github.com/ziglang/zig/blob/0.15.2/lib/libc/musl/src/env/__init_tls.c)
and [program-header iteration](https://github.com/ziglang/zig/blob/0.15.2/lib/libc/musl/src/ldso/dl_iterate_phdr.c)
also declare it weak and hidden, using it conditionally with `PT_DYNAMIC`.
This source and ELF evidence supports a narrow static musl marker exception;
it does not establish runtime behavior. The next probe requires this exact
symbol-table shape and absence of dynamic segments and sections, and rejects
every other undefined symbol.

The four new source anchors are byte-identical in the independently verified
amd64 and arm64 Zig distributions:

| Source path under `lib/libc/musl/` | SHA-256 |
| --- | --- |
| `arch/x86_64/crt_arch.h` | `3dceeb0644f95a5003201705ac78ba869b595c729d868d56f4fd84824a857a7b` |
| `arch/aarch64/crt_arch.h` | `14dcdc316f356e3d51217c1a2d73e7942d852d7f42be3308afce19e927a97999` |
| `src/env/__init_tls.c` | `6cb2c2f1ce53b87b9c7f2452010925ddf3356c4ae40d5e8a77f7bee3960febd6` |
| `src/ldso/dl_iterate_phdr.c` | `802a537924c900ff607b65bda465bf66899890b7b0707c0cd55c7b800db20327` |

The CLI help smoke and attestation steps were skipped after the audit
failure. Independent `gh attestation verify` queries returned HTTP 404 for
both ELF digests. No product admission follows from this build; remaining
work includes a passing native audit, CLI and fixture behavior, full notice
closure, signed release bytes, and the product's existing gates.
