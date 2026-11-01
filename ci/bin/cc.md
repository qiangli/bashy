---
name: cc
provides: cc gcc clang
run: zig cc
---

# cc — repair-profile wrapper unit

`cc` for the repair profiles (S549.R4b): cgo needs a C compiler, and the
repair rule is "bashy is all you need" — so when the environment provides no
compiler, `CC` points here and the wrapper runs bashy's own pinned Zig
toolchain as a C compiler (`bashy zig cc`; zig cc IS clang). The runner also
links the `gcc` and `clang` spellings at this unit, so recipes and configure
probes that ask for either resolve to the same provider.

`{{bashy}}` is substituted with the pinned bashy executable at
materialization time; the unit runs through that bashy, never a host script.

```bashpp
"{{bashy}}" zig cc "$@"
```
