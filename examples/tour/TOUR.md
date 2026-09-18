# The Bash# tour

*Bash# is alpha. Everything on this page runs today on a released `bashy`, with
no model, no API key, no network, no Go toolchain. Syntax may still change
before 1.0 — and the way to change it is an RFC in the language repo.*

**Bash# in one sentence:** the bash you already know, Go where you need types,
any fenced language where you need a library, and `agentic` where you need a
model — with contracts so a model's output is judged, never trusted.

This directory holds eleven small files. Each is a complete program, each has
a pinned transcript next to it (`NN-name.expected`), and `./check.sh` runs
them all against the `bashy` on your `PATH`. If you would rather be walked
through by a coding agent, hand it `SKILL.md` in this directory — it is the
same tour, written as a procedure.

Install first: unpack the archive for your platform from the bashy Releases
page, put `bashy` on `PATH`, and check `bashy --version`. Then `./check.sh`.

---

## 00 · It is still bash

`00-bash-is-bash.bsh` is a plain Bash 5.3 script — functions, arrays, `[[`.
Run it with the dialect on and off:

```sh
bashy --bashsharp    00-bash-is-bash.bsh
bashy --no-bashsharp 00-bash-is-bash.bsh
```

Same output. That is the first promise: *every Bash 5.3 program is a Bash#
program with the same meaning*. Bash# only adds shapes that stock bash
rejects (a call with `(`, a `:=`), or that are listed with an escape hatch —
and with the flag off, or in `--posix` mode, none of it exists.

## 01 · Go where you need types

```bash
x := 42
name := "gopher"
func twice(n int) int { return n * 2 }
y := twice(x)
echo "x=$x y=$y name=$name"
printf '%s doubled is %d\n' "$name" twice(y)
```

`:=` declares a typed value; `func` declares a typed function; a call can be
written as an ordinary *word* in a command. Shell expansion sees Go values as
text, so `"$x"` works exactly where it always did. (Go *control flow* at the
top level of shell text — `if x != 0 { … }` — is not in the tour because it
is not shipped yet; inside a `func` body it is, as sections 04 and 08 show.)

## 02 · Keyword arguments and defaults

```bash
func greet(name string, retries int = 3) {
    printf '%s:%d\n' name retries
}
greet("Ada")                    # Ada:3
greet(retries: 7, name: "Cy")   # Cy:7
```

Shell people think in named flags; Go makes you count positions. Both of
these shapes are syntax errors in stock bash, so admitting them changes no
existing script.

## 03 · Decorators

```bash
func tag(c *Call, label string, level string = "info") {
    name := c.Name
    echo "[$level] $label -> $name"
    c.Next()
}

@tag(label: "keyword", level: "debug")
func two() { echo "two:body" }
```

A decorator is just a function whose first parameter is `c *Call`; `c.Name`
is who was called and `c.Next()` runs it. Lines stack above a declaration.
The arguments are evaluated on **every call**, in the declaring scope — run
the file and watch `@tag("late-$suffix")` print `late-a` and then `late-b`
after `suffix` changes. Tracing, auth checks, retries: one line each.

## 04 · Enums that must be covered

```bash
type Color enum { Red; Green }
func label(c Color) string {
    switch c {
    case Red:   return "red"
    case Green: return "green"
    }
}
```

An enum lowers to Go constants, and a `switch` over it must name every
member. Drop a case and the file is refused:
`BASHPP-EENUM-NONEXHAUSTIVE: switch on Color is missing member Green or a default arm`.

## 05 · `readonly`, all the way down

bash already has `readonly`. Bash# makes it mean it: a struct, slice or map
marked `readonly` cannot be mutated through a field, an index, a key, an
alias, or from a subshell. Reads stay ordinary:

```bash
readonly cfg
alias := cfg
printf '%s:%d:%s\n' alias.Meta.Name alias.Ports[1] alias.Labels["tier"]
( cfg.Ports[0] = 8080 )     # BASHPP-EREADONLY-MUTATION
```

## 06 · Contracts

```bash
@require('test -d "$work" && test -n "$1"')
@ensure('probe=leaked; test -f "$work/$1"')
function produce() { : > "$work/$1"; }

@ensure('test "$RESULT" -gt 0')
func twice(n int) int { return n * 2 }
```

`@require` runs before the body; if it fails the body never runs and the
call exits **3**, naming the clause. `@ensure` runs after, and can see
`$RESULT` for a typed function. Each check is a shell command run in the
call's own frame — `$1..$n` are the arguments — and a check's own variables
never leak back (`probe` stays unset). Single-quote a check: double quotes
would expand at the decorator line, not at the call.

## 07 · `agentic` — judged, never trusted

This is the reason Bash# exists. `07-agentic-yield.bsh`:

```bash
@guard(effects: "read")
@require('test -n "$1"')
@ensure('test "$1" != lie')
agentic function summarize() { ... }
```

Six calls, six exit codes:

| call | | exit |
|---|---|---|
| `summarize ok` | require → body → ensure, all fine | 0 |
| `summarize ""` | require fails; body never runs | 3 |
| `summarize lie` | body says fine; ensure disagrees | 3 |
| `summarize write` | body writes under a read-only guard | 1 (denied) |
| `summarize fail` | body returns 1 | 1 |
| `summarize yield` | body returns 6: *input required* | 6 |

The interpreter never calls a model. `agentic` marks the one place where
determinism moves from the program to the **judge** — the contracts around
it — and `return 6` is a *yield*: the body needs something it does not have,
and that fact reaches whoever ran the script (your shell, or the coding agent
that ran `bashy -c …`) so it can ask, and retry. No ensure runs on a yield.

## 08 · Null safety is a check, not a keyword

```sh
bashy check --bashsharp 08-null-safety.bsh
# BASHPP-ENULL-DEREF: p may be nil when dereferenced   (exit 2)
```

No `?.`, no `??`. A pointer dereferenced without a nil test is reported by
`bashy check`; narrow it with an ordinary `if p == nil { return 0 }` and the
report goes away.

## 09 · Any language where you need a library

```
~~~py as py
def shout(s: str) -> str:
    return s.upper() + "!"
~~~
s := py.shout("islands")
echo "$s"                      # ISLANDS!
```

A tilde-fenced block is an *island*: its functions become ordinary callables
in shell text and typed values cross the boundary. The same shape exists for
TypeScript, Rust, C/C++, Go and bash. Bash# does not embed a Python — it
finds yours on `PATH` (the tour skips this section if there is none).

## 10 · And back out as Go

```sh
bashy transpile --bashsharp 10-transpile.bsh -o t.go
```

Every construct in this tour lowers to ordinary Go, so a Bash# file can be
handed to the Go compiler. The emitted file imports the small `shellrt`
runtime from the engine module, so building it today needs a checkout of
`github.com/qiangli/sh` next to it (`check.sh` builds and runs it when
`BASHSHARP_SH_ROOT` points there, and proves the transcript is the same);
running the `.bsh` directly needs nothing.

---

## What is not here, on purpose

Goroutines and `select` in shell text, whole multi-package Go programs, and
the constructs Bash# **refuses** — list comprehensions, the ternary,
`match`, try/catch, operator and method overloading, inheritance,
async/await. Each refusal has a reason on record in the language repo (the
short version: it must lower to plain Go, and it must not collide with a
shape bash already accepts). Whole Go programs run through `--source=go`,
which is where the Go-corpus numbers in `docs/claims.md` are measured.

## Where to go next

- `../quickstart/` — the ten-minute version (just the judge demo).
- The language repo: the five clauses, `ROADMAP.md` (what is alpha, beta,
  1.0), `docs/claims.md` (every number and its corpus), and `rfcs/` — where
  the syntax you wish you had is decided.
