---
name: cxx
provides: c++ g++ clang++
run: zig c++
---

# cxx — repair-profile wrapper unit

`c++` for the repair profiles (S549.R4b): the C++ counterpart of
[`cc.md`](cc.md) — when the environment provides no compiler, `CXX` points
here and the wrapper runs bashy's own pinned Zig toolchain
(`bashy zig c++`). The `g++` and `clang++` spellings link here too.

`{{bashy}}` is substituted with the pinned bashy executable at
materialization time; the unit runs through that bashy, never a host script.

```bashpp
"{{bashy}}" zig c++ "$@"
```
