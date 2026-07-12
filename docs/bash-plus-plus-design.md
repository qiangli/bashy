# bashy++ — a measured superset of Bash

Status: **design of record, 2026-07-12.** The language half of the uplift; the runner half
is `plan-distributed-chunked-execution.md` (§Axis 4 there is the summary — this is the
detail). Sequencing lives in the plan; the decisions and their reasoning live here.

## Why a language extension at all

bashy is a **pure-Go** Bash 5.3 engine. That changes what is possible: there is no
transpile step, no text-emitting hack, no attempt to squeeze Go semantics into a shell's
string-only world. The interpreter's own value model, memory model, and concurrency
primitives can be exposed *directly* to the shell's AST and evaluation loop.

The forcing function is the runner. A chunked pipeline — conformance matrix, HPC sweep, ML
training, ETL — must express **typed records, fan-out, and stage-to-stage handoff**. Today
`dag.md` expresses that *declaratively*, in task headings. There is no way to say it *in the
script*, and every record that crosses a stage boundary is serialized to text and scraped
back with `awk`. That is not a cosmetic problem: it is precisely why the conformance
aggregator silently merged results across incompatible hosts — `awk` cannot tell that it is
doing it.

**`dag.md` is the declarative workflow surface; bashy++ is the in-language one.** Two faces
of one runner.

---

## The seams already exist

This is additive, not a rewrite. Three facts about the current engine:

1. **The variable table is already a union.** `expand.ValueKind` (`sh/expand/environ.go:67-125`)
   is `String | NameRef | Indexed | Associative`, with `Variable{Kind, Str, List, Map, …}`.
   A structured value is a **new Kind (`Object`)** holding a native Go `any` — not a refactor
   of `map[string]string`.
2. **The parser already has a dialect seam.** `syntax.LangVariant`
   (`sh/syntax/parser.go:29-51`) is `LangBash | LangPOSIX | LangMirBSDKorn | LangZsh`.
   `LangBashPP` slots in beside them.
3. **Subshells are already goroutines.** Concurrency costs almost nothing at the runtime
   layer — it is the *same* mechanism that produces the known signal/job-control edges.

**What does NOT exist, and is the real work:** `interp` and `expand` have **no dialect seam
at all** — only the parser does. It has to be built. Build it *generally*, because it is the
same seam a zsh mode would need.

---

## The design rule: supersetness is measured, not asserted

bashy++ is a **true superset** of Bash — every valid Bash script keeps its exact meaning.
The C++/C analogy is instructive precisely because **C++ failed this test**: it is *not* a
strict superset of C, and the reason is new reserved words. Any C program with a variable
named `class`, `new`, or `template` stops compiling.

The shell has the same exposure, and it is worse: a reserved word in command position doesn't
shadow a *variable*, it shadows a **program**. Reserve `struct` and every script that invokes
a binary called `struct` changes meaning.

> **Rule: prefer two-word keywords and new operators over new single reserved words.**

| Form | Verdict | Why |
|---|---|---|
| **`go routine { … }`** | ✅ | Two-token lookahead in command position. `routine` is not a `go` subcommand, so the collision surface is nil — and `go build ./...` (ubiquitous, and a bashy verb) keeps working untouched. Bash resolves reserved words pre-expansion and only unquoted, so `go "routine"` and `go $x` stay ordinary commands for free. |
| **`:=`** | ✅ | New operator. Today `x := y` parses as command `x` with args `:=` and `y` — nobody writes that. |
| **`<-`** | ✅ (with care) | New operator. Check against `<` redirection and `<<-` heredocs. |
| ~~bare `struct` / `chan` / `func`~~ | ❌ | **The C++ mistake.** Any script invoking a program of that name breaks. Use a two-word form or a `declare`/`typeset` extension. |

### The gate, and it is the dogfood

> Run the **entire** conformance matrix with bashy++ **ON** and require a **byte-identical
> fail-set** to mode-off — not just the 86 Bash 5.3 fixtures, but the 719-script clean-room
> differential, the 10-shell panel, oils, yash, modernish.

If the fail-set is identical with extensions live, the superset property is **measured**, not
argued. This is a permanent regression test, and it closes a loop: **the conformance suite
guards the language, and the language is what makes the conformance suite's own records
structured.** The two halves of the uplift check each other.

Consequently there is **no script pragma** and no opt-in friction — extensions are simply on
in bashy's default mode. Two gates remain, both host-level rather than script-level:

1. **`--posix` / `set -o posix` turns everything off.** This is what a certification run uses.
2. **The engine is shared.** `interp`/`expand`/`syntax` live in `../sh` and have other
   consumers, so "always on" cannot mean "always on in the engine". Gate at **`LangVariant`**:
   bashy sets `LangBashPP`; other consumers keep `LangBash`. A host-application choice.

---

## What a goroutine can actually run

A correction that shapes the whole concurrency phase, and an easy thing to get wrong:

**Subshells are already goroutines, but an external binary is still `fork`/`exec` — always.**
`go routine { curl … }` is `&` with extra syntax; the fork is still paid.

A goroutine only pays off when the body is **in-process**: shell functions, builtins, and the
Tier-1 coreutils that already run without forking. The prize is in-process fan-out where
stages hand each other **native values over channels instead of serializing through pipes** —
exactly where the 0-fork Tier-1 userland thesis cashes out, and exactly what a pipeline/HPC
workload needs.

## Types before concurrency

Parallelism already exists — `dag -j N`, chunks, the fleet, goroutine subshells. **Structured
values do not.** The runner consumes structured records everywhere (run records, host facts,
the chunk manifest, the merged scoreboard) and each is currently text. A channel is also just
a `Value` of native kind, so the union must land first regardless.

| Phase | Ships | Notes |
|---|---|---|
| **L0** | `Object` ValueKind; the `interp`/`expand` dialect seam + `LangBashPP`; **auto-JSON at the OS boundary**; **the superset gate** | The only language phase the runner's MVP needs |
| **L1** | typed records via a two-word / `declare` form; `:=` tuple-return with an auto-bound `err` | The pipeline/HPC record type |
| **L2** | **`go routine { … }`** + channels (`<-` / `send` / `recv`) | In-process bodies only (see above) |
| **L3** | the reflect **Go bridge** + type registry (`json`, `http`, `math`) | Most power, most risk — see below |

### Auto-JSON at the OS boundary (the L0 payoff)

A native value crossing into an external binary (`grep`, `awk`, a Python script) **serializes
to JSON**; crossing back, it parses. Inside the shell it stays a fast native object. This is
the whole UX: a structured record behaves like structured text when it talks to the outside
world, and like memory when it doesn't.

### L3 is where the danger is

An unbounded reflective bridge into arbitrary Go packages is an **unbounded effect surface**.
It must answer to the existing `Effects`/`EffectCap` lattice — a script that can call
`http.Get` through the bridge has a `net` effect whether or not it declared one. This is also
where the licensing rule bites: *download+exec ≠ bundle* holds for separate programs, and a
linked-in package registry is not a separate program.

---

## Open questions (do not close silently)

- **Error model.** `content, err := readFile "config.json"` maps Go's `(val, err)` onto the
  shell's `$?`. Does `err` shadow `$?`, coexist, or set both? Bash scripts branch on `$?`;
  bashy++ scripts would branch on `$err`. Both must remain true simultaneously.
- **Where `Object` values may flow.** Arrays of objects? Objects in associative arrays?
  Exported to the environment (which is `[]string` at the syscall boundary — an object cannot
  survive `execve` except as JSON)?
- **`<-` vs `<<-`.** Lexer disambiguation must be proven against the corpus, not reasoned about.
- **Does `declare -T`-style typing carry into `local`/`export`/`readonly`?**

---

## Appendix — the source proposal (preserved verbatim in substance)

The design brief that opened this line of work. Kept because the decisions above are best read
against what they accepted, amended, and rejected.

> **Knowing that the shell engine is already natively implemented in Go changes the entire
> game.** You no longer need to emit text-based transpile hacks or squeeze Go semantics into
> rigid Bash limits. Instead you can directly expose Go's native runtime, memory model, and
> concurrency primitives into the AST and evaluation loop.
>
> **1. Evolution of the variable engine (from strings to `any`).** Refactor environment storage
> to handle structured data natively — a `Value` union wrapping Bash strings alongside complex
> Go types (`Kind reflect.Kind; Value any`). When a standard Bash command runs, coerce to the
> string representation; when a bashy++ construct runs, retain the native Go type.
>
> **2. Native concurrency (true goroutines vs OS forks).** Intercept `go` to launch green
> threads (`case *ast.GoStatement: go interpreter.Execute(node.Body, env)`) rather than a heavy
> OS process. Treat channels as first-class references in the variable map — a `chan` builtin
> initializes a native Go channel and `<-`/`send`/`recv` pipes data through memory without
> serializing over stdout/stdin pipes.
>
> **3. Reflective interop (the "Go bridge").** A global type registry (`json`, `http`, `math`)
> lets shell scripts instantiate any Go type or call any registered Go function via `reflect`.
> An annotated `struct User { Name, Age }` declaration allocates a native map/reflected struct.
>
> **4. Dual-mode error handling.** Go returns `(val, err)`; Bash returns `$?`. A dedicated
> assignment syntax bridges them: `content, err := readFile "config.json"`, with `err` bound
> automatically when the underlying function returns a secondary error interface.
>
> **5. Implementation strategy.** Not a separate language — a **strict superset** as an
> extended grammar profile in the existing parser. Keep the Bash-compliant lexer; recognize
> extended tokens (`:=`, `<-`, `struct`) under a pragma. **Context-aware evaluation:** passing a
> bashy++ value into an external binary auto-serializes the Go struct/map to JSON — structured
> JSON text to the outside world, fast native memory object inside the shell loop.

**What we accepted:** the whole thesis (native runtime exposure, the `Value` union, channels
in the variable map, the Go bridge, auto-JSON at the boundary, superset-not-fork).

**What we amended:** (a) `go` alone cannot be the keyword — it collides with the Go toolchain,
which bashy itself ships as a verb; `go routine` is the collision-safe form. (b) `struct`/`chan`
as bare reserved words is the C++ mistake; use two-word forms or `declare` extensions.
(c) A goroutine only pays off for in-process bodies — over an external binary it is `&` with
extra steps. (d) The pragma is unnecessary and undesirable: gate at `LangVariant` (host-level)
and *measure* supersetness against the conformance matrix instead. (e) The `Value` union is not
a refactor — `expand.ValueKind` already exists; add a Kind.

**What we deferred:** the reflective bridge (L3) until the effect-cap containment story is
settled — an unbounded bridge is an unbounded effect surface.

**Sequencing answer to the brief's closing question** ("concurrency first, or types first?"):
**types first.** Parallelism already exists; structured values do not.
