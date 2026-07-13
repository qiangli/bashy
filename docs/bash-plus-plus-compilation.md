# bash++ → Go: transpile and compile to a native binary

*Design exploration — companion to `bash-plus-plus-design.md`. The language doc is
about running bash++ in the interpreter; this is about the phase *after*: lowering a
bash++ script to Go source and compiling a standalone binary.*

## The question, and the honest answer

> Can we transpile bash++ into Go and `go build` it into a self-contained binary?

**Yes — but the value is a gradient, and it is bash++ specifically that makes it real.**
Transpiling *pure bash* to native code is mostly pointless; transpiling *bash++* is
genuinely valuable, for one reason: **bash++'s extensions are already Go semantics.** A
script written in the bash++ idiom lowers to real, fast Go; a legacy-bash script lowers
to interpreter calls. So bash++ is not just "bash plus Go features in the interpreter" —
**it is the on-ramp to compilation.**

## Why transpiling *pure bash* to native is a dead end

This is why no real bash-to-native compiler exists (and why `shc` is a fake — it just
embeds the script and calls the shell):

- **Bash is dynamically typed and string-everything.** Word-splitting, `IFS`, globbing,
  dynamic scoping, `${!var}` indirection — faithfully "compiling" these means emitting Go
  that *calls a runtime library which does exactly what the interpreter does.* The
  compiled Go isn't faster, because it performs the same dynamic work.
- **`eval`, dynamic sourcing, dynamically-defined functions** are runtime code
  generation. You cannot lower them ahead of time — the code doesn't exist until runtime.
  Any program that uses them needs an interpreter embedded regardless.
- **External commands still fork.** `grep foo` transpiled is still `fork`/`exec` of
  `/usr/bin/grep`. No speedup from compilation.

So for the *dynamic* parts of bash, "transpile to Go" degenerates into "generate Go that
calls the interpreter" — which is bundling, not compilation.

## Why transpiling *bash++* is real — three reasons

1. **The bash++ extensions lower 1:1 to Go.** They *are* Go constructs wearing shell
   syntax:
   | bash++ | Go |
   |---|---|
   | `Object` ValueKind (a native `any`) | a Go `struct` / `map` |
   | typed records (L1) | Go structs |
   | `x := y` with auto-`err` (L1) | `x, err := …` |
   | `go routine { … }` (L2) | `go func() { … }()` |
   | channels, `<-`/`send`/`recv` (L2) | Go channels |
   | the reflect bridge (L3) | direct Go package calls |
   The more a script uses bash++ features, the more of it compiles to clean, native Go —
   and the reflect bridge *loses its reflection* at compile time (a direct call), which is
   a genuine optimization the interpreter cannot make.

2. **The runtime is already Go.** bashy's value model (`expand.ValueKind`, the L0 `Object`
   kind) and its Tier-1 userland (`cat`/`grep`/`sort` as pure-Go functions) are Go code.
   The transpiler emits Go that calls the *same* primitives — no impedance mismatch, no
   FFI, no re-implementation.

3. **Tier-1 commands inline — the fork disappears.** This is the bashy-specific win. Where
   a script uses the in-process coreutils, the transpiler emits a **direct Go call**
   instead of a fork. That is the compile-time realization of the 0-fork Tier-1 pipeline
   thesis: `sort | uniq -c | sort -rn` becomes three Go function calls passing values in
   memory, not three processes passing bytes through pipes.

## The spectrum (name the levels honestly)

| level | what it is | binary runs… | honest name |
|---|---|---|---|
| **0** | embed the script, call `interp.Run` | the interpreter | **bundling** (this is `shc`) |
| **1** | AST → Go source, 1:1 lowering, interp fallback for dynamic bits | compiled Go + a thin embedded interp | **transpilation** |
| **2** | + type inference, drop the `Value` union where types are known, inline Tier-1 | optimized native Go | **compilation** |

Level 0 is trivial and already exists (`bashy script.sh`). The prize is Level 1, and
bash++ is what makes Level 1 more than a rename.

## The design: a *hybrid gradient* compiler

The transpiler is an **AST → Go-source pass** over the same `sh/syntax` AST bash++ parses,
gated on the **L0 `LangBashPP` dialect seam**:

- **Lower what it can.** bash++-idiomatic, statically-analyzable constructs (typed values,
  loops, pipelines, `go routine`, channels, Tier-1 commands) emit native Go.
- **Fall back where it can't.** `eval`, dynamic indirection, an unanalyzable external
  command → emit a call into an embedded `interp` runtime. The binary is native Go for the
  analyzable core plus a thin interpreter for the dynamic tail.
- **A `bashpp/runtime` Go library** provides the shell primitives (Value ops, word-split,
  the coreutils — which already exist as Go — pipelines-as-goroutines). Transpiled Go
  imports it.
- **Compile via the self-provisioning toolchain** (`bashy go`), so nothing external is
  assumed: `bashy compile script.bpp -o mytool` = transpile → `go build` → a binary.

So the reachable target is: **fully-static bash++ → a pure native binary with no
interpreter; a mixed script → a native binary with a shrinking interpreter core.** The
more bash++ (less legacy bash), the more native and the leaner the binary.

## The correctness gate — measured, not asserted

The same discipline as bash++ supersetness: **a transpiled binary must produce
byte-identical output to the interpreted run, across the conformance corpus.** Any
divergence is a bug, not a "compilation difference." This is the integrity-of-the-waist
rule applied to the compiler: the compiled path may not lie about the semantics the
interpreted path guarantees. It is the transpiler's regression gate, and it is
automatable against the existing fixtures.

## Where it sits in the roadmap

The language phases *are* the enabling work — each typed/concurrent feature is a
transpilable construct:

- L0 `Object` + the seam → the typed Value model the transpiler emits.
- L1 typed records / `:=` → Go structs / `x, err :=`.
- L2 `go routine` / channels → goroutines / channels.
- L3 reflect bridge → direct package calls (reflection eliminated at compile time).

So the compiler is the natural **L4/L5**, and it is not a separate project bolted on — it
is the same value model and the same dialect seam, read a second way.

## Payoffs

- **Performance** for hot pipelines/loops: native Go + inlined Tier-1 = no interpreter
  walk, no forks. This is the top tier of the agentic-performance ladder.
- **Distribution**: a bash++ script becomes a single deployable binary — no bashy runtime
  required for the fully-static case.
- **A smooth migration path**, which is the strategic point: start as an interpreted bash
  script → adopt bash++ features (still interpreted, now typed/concurrent) → transpile the
  typed core to a native binary, incrementally. It is the "C → C++ → rewrite in a real
  language" arc, but **without the rewrite** — the same source compiles further as you
  make it more typed.

## Hard walls and non-goals

- **Do not claim "compile any bash to native."** That is the `shc` lie. The honest claim
  is the gradient: static bash++ compiles fully; dynamic bash keeps an interpreter core.
- **`eval` and friends always need the interpreter.** A truly interpreter-free binary
  exists only for the fully-analyzable subset. Most real programs are hybrid.
- **Correctness is non-negotiable.** The byte-identical gate is the whole trust of the
  feature; ship it before any performance claim.
- **This follows the language, not precedes it.** It needs L0–L2 landed to be more than
  Level-0 bundling. Sequence: L0 (in flight) → L1/L2 → the transpiler MVP (Level 1) →
  optimization (Level 2).

## The one-line reframing

**bash++ is the on-ramp from an interpreted shell script to a compiled Go binary — and it
is transpilable *because* its extensions are Go semantics.** The language work and the
compiler are the same investment, read twice.

---

## Appendix — the reverse direction (Go → bash++), for symmetry

*A technical thought experiment, not a roadmap item: can we go the other way — emit a
bash++ script from Go source? It sharpens the forward thesis by contrast.*

**Technically yes (both are Turing-complete), practically no for general Go — and the
asymmetry is the whole point.** Transpilation is easy *toward* the richer, more static
substrate and hard *toward* the poorer, more dynamic one. Forward (bash++ → Go) rides the
grain: shell constructs lower into a superset — types become types, goroutines become
goroutines — and you *gain* speed. Reverse (Go → bash++) fights the grain: the target
*lacks* primitives the source relies on, so you must **simulate** them, losing speed and
gaining nothing.

**What the target lacks — the walls:**
- **Pointers and the shared heap.** Go has real aliasing over a GC heap; bash has none
  (`declare -n` namerefs are single-level, not an object graph). Faithful emulation means
  building a simulated heap (an associative array as memory, integer keys as "addresses")
  — i.e. writing a little VM in shell.
- **Shared-memory concurrency.** Go goroutines share a heap under a real scheduler; shell's
  only native concurrency is *copy-on-fork subshells with no shared memory* — the opposite.
  bash++ can express `go routine`/channels **only because they are backed by the Go runtime
  under the interpreter** — so mapping Go concurrency to bash++ concurrency doesn't run it
  "in shell," it hands it back to Go. The skin is bash++; the engine is still Go.
- **The type system, `defer`/`panic`/`recover`, closures-by-reference, `select`** — erased
  or hand-simulated with tags and dispatch tables.
- **Floats and the stdlib.** bash has no native floats (needs `bc`/`awk`); `net/http`,
  `crypto`, `reflect` have no shell reimplementation — you'd shell out to external programs,
  changing semantics, dependencies, and the security surface.

**The same degenerate cheat, mirrored.** Just as the forward direction's Level 0 was
"bundling" (`shc` embeds the script and calls the interpreter), the reverse has a Level-0
cheat: emit one line of bash++ that **calls** the compiled Go (via the L3 reflect bridge or
an `exec`). Full fidelity, transpiles nothing — FFI, not translation. It is the only
*faithful* full-Go answer, and it cheats.

**The spectrum, mirrored:**
| level | reverse (Go → bash++) | verdict |
|---|---|---|
| 0 | emit bash++ that calls the compiled Go (reflect bridge / `exec`) | faithful but FFI — translates nothing |
| 1 | lower a **restricted Go subset** (no aliasing, no shared-mem goroutines, no stdlib beyond a mapped shim, scalar/struct/tree data) → readable bash++ | **the genuinely doable, interesting case** |
| 2 | simulate Go's heap + scheduler + memory model in shell | Turing-possible, absurd — a Go VM in bash |

**The one place it makes sense.** Not performance — the opposite axis: **reach**. Compile a
*small typed Go-like subset* down to a portable, dependency-free shell script that runs
anywhere a shell runs, with **no Go toolchain on the target**. That is an established niche
(tools that compile a DSL to POSIX `sh` purely for portability; `batsh` targets bash *and*
batch). Go is a poor *source* for it — too rich — but the pattern is real.

**The symmetry worth keeping:**
- **Forward — compile UP for SPEED.** Lower a dynamic script into a static substrate; win
  performance; needs a toolchain to build, then runs native.
- **Reverse — compile DOWN for REACH.** Lift a static program into a dynamic substrate; win
  ubiquity (runs wherever a shell runs, zero runtime deps); lose performance; sane only for
  a restricted subset.

The general law both illustrate: **you transpile easily toward the richer substrate and
only painfully toward the poorer one** — which is exactly why the forward direction is a
real feature and the reverse is a curiosity with one narrow, portability-shaped use.
