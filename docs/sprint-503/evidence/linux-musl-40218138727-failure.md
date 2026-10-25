# S503.6 Linux musl first hosted probe: CMake linker flag failure

**Review state:** failed experiment; `probe_passed=false` and
`product_admitted=false` for both targets. No ELF, final link map, runtime
inference, or product acceptance resulted from this run.

The [manual GitHub Actions run](https://github.com/qiangli/bashy/actions/runs/40218138727)
used Bashy commit `d2d3627cdf9948a64a739bb7408ff245fe715ceb`. Its two native
jobs ran on `ubuntu-24.04` (`amd64`) and `ubuntu-24.04-arm` (`arm64`).
The retained artifact IDs were `12607439819` and `12608700048`, respectively.

| Native target | `configure.log` SHA-256 | `probe-report.json` SHA-256 |
| --- | --- | --- |
| amd64 | `e377cf252de33fd3c9c21be4e9a45df93200fdbeb20b0e2e5f6e10fa83e93b96` | `62fc51a5f9e7978cebc69686e01f37a33fec2674e7de401da487aa666d1ed80f` |
| arm64 | `1c8c1f85354517951c8b8cb7b5e405ee7dae3d911ad4feda236c818f2d3e2935` | `f9f61c2bd10857891eddf4b3f3eaa7654292027f30a6270a01eb0420822c2a6a` |

Both configure logs identify Clang 20.1.2, then fail the first C compiler
`TryCompile` link. Zig 0.15.2 reports exactly `error: unsupported linker arg:
-Map` for the probe's `-Wl,-Map=<path>` flag. This is the common cause; the
Whisper target was never compiled or linked. The reports record the pinned
Zig and whisper.cpp archive sizes and SHA-256 values. Both retained minisign
logs say `Signature and comment signature verified`; all nine retained Zig
source/notice anchors rehash to their report entries. These checks establish
input identity, not a product license conclusion.

The proposed correction uses Zig 0.15.2's accepted `-Wl,--print-map` and
`--verbose-link` options. The [Zig argument parser](https://github.com/ziglang/zig/blob/0.15.2/src/main.zig)
accepts them; the [Zig LLD path](https://github.com/ziglang/zig/blob/0.15.2/src/link/Lld.zig)
passes `--print-map` to LLD and prints the exact linker argv. LLVM's
[LLD map writer](https://github.com/llvm/llvm-project/blob/llvmorg-20.1.2/lld/ELF/MapFile.cpp)
shows the header and input-section rows parsed by the probe. The corrected
probe requires one final `whisper-cli` map and one verbose LLD invocation,
then hashes and retains every direct input and loaded archive. Its focused
parser tests run locally; this is source-level support only until another
native hosted run produces a real ELF and map. A new run requires manager
review and coordination.
