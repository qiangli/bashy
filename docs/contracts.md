# Bash++ contracts (design by contract)

bashy lets you write a **contract** on the two things it already runs for you —
a `bashy dag` target and a Bash++ function — using the traditional vocabulary
of *design by contract* (Meyer, 1986): a **`require`** clause (precondition),
an **`ensure`** clause (postcondition), and the **effect cap** the tree already
had (`Effects:` / `@guard`). A contract is judged deterministically at the
process boundary: every check is a shell command, exit 0 means it holds.

Nothing about a contract is generative. There is no expression language, no
predicate id, no model in the loop — a check is the same thing you would type
at a prompt to see whether the world is the way you meant it to be.

## Surface

| clause | `dag` target | Bash++ function (shell or typed, `agentic` or not) |
|---|---|---|
| require | `Require: <shell check>` (repeatable, one check per line) | `@require('<shell check>', ...)` (repeatable) |
| ensure | `Ensure: <shell check>` (repeatable) | `@ensure('<shell check>', ...)` (repeatable) |
| effects | `Effects: read,write,…` (declared, recorded) | `@guard(effects: "read,…")` (enforced) |

Evaluation order on both surfaces: **require → body → ensure**, with the cap
in force throughout. A failed `require` means the body **does not run** (and is
never retried); a failed `ensure` means the result is **not valid** even
though the body exited 0. Both exit **3** (`ExitPrecondFail`) and print a
message naming the clause and the check:

```
precondition failed: <check>
postcondition failed: <check>
```

(a function prefixes its own name: `greet: precondition failed: test -n "$1"`).
Checks within one clause are an unordered set: adding one **tightens** the
contract, order carries no meaning.

`Require:` (singular) is not `Requires:` — the plural is make's prerequisite
list: a prerequisite is a precondition the engine can satisfy by running
another target; a `Require:` is one it cannot.

## What a check sees

A check runs as a shell command through bashy's in-process shell and
userland, **at the process boundary**:

- **dag target** — in the target's directory, with the target's environment,
  exactly as `Ensure:` always has. `Require:` accepts the same predicate
  forms (`file-exists <path>`, `file-absent <path>`, `http-ok <url>`,
  `cmd <shell…>`, or any bare shell command).
- **function** — with the call's arguments bound as `$1..$n`, the working
  directory and the environment bashy was started with. `@ensure`
  additionally sees `STATUS` (the body's exit status) and, for a typed
  function, `RESULT` (the first result value). A check does **not** see the
  script's own variables, exported or not — what a check needs is passed to
  the call. The effect cap on the call (`@guard`) applies to the check's
  commands exactly as it applies to the body's.

Write function checks in **single quotes**: a double-quoted decorator argument
is expanded where the decorator line is evaluated, so `"$1"` would already be
empty by the time the check runs.

## The `agentic` boundary

An `agentic function` takes the same three decorators. `@ensure` judges a
**completed** body only: a body that failed keeps its own status, and in
particular a **yield** — exit 6, *input required* — propagates untouched,
with no postcondition run. A harness therefore never sees "postcondition
failed" masking "input required". `test/contracts/agentic-boundary.bpp` is
the fixture that pins this (and the order of the three clauses); its
transcript is `agentic-boundary.expected`, and `test/contracts/run.sh
[bashy]` replays it against any binary.

`@require`/`@ensure` are **source-only**: policy advice cannot add them (an
advised postcondition is the tighten-only ratchet, and it needs its own gate
first). `@retry` has the same rule.

## Examples

A dag target with a full contract:

```markdown
### build
Require: test -n "$VERSION"
Require: file-absent dist/app
Ensure: file-exists dist/app
Ensure: test -s dist/app && ./dist/app --version
Effects: read,write

~~~bash
go build -ldflags "-X main.version=$VERSION" -o dist/app .
~~~
```

Run with `VERSION` unset and the body never runs:

```
==> build FAILED (exit 3)
    precondition failed: test -n "$VERSION" (exit 1)
```

A function with a full contract, inside an `agentic` scope:

```bash
@guard(effects: "read")
@require('test -d "$1"')
@ensure('test -f "$1/summary.txt"')
agentic function summarize() {
    bashy chat --read-only -m "summarize $1" > "$1/summary.txt"
}

agentic {
    summarize ./notes || echo "no summary: $?"
}
```

- `./notes` missing → `summarize: precondition failed: test -d "$1"`, exit 3,
  nothing ran.
- The body wrote nothing → `summarize: postcondition failed: test -f
  "$1/summary.txt"`, exit 3.
- The body yielded (exit 6) → exit 6, no postcondition, the harness asks for
  input.

## Not in this release

- `invariant` — Meyer's is a *class* invariant and Bash++ has no class model.
- Advised (policy-applied) `@require`/`@ensure`.
- Enforcing a dag `Effects:` cap (declared and recorded; the sandbox layer's
  job).
- A `~~~ensure` fence in `SKILL.md`, and a `dag --probe` that reports a
  postcondition that already holds before the body (it cannot discriminate).
