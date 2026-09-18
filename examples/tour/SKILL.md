---
name: bashsharp-tour
description: >-
  Walk the Bash# language tour on an installed bashy: eleven runnable
  sections (bash-is-bash, Go values, keyword/default args, decorators, enums,
  deep readonly, contracts, agentic + yield, null-safety check, a Python
  island, transpile to Go), each with a pinned transcript, and one gate
  (check.sh) that proves every section on the binary in front of you. Use
  when asked to learn, demonstrate, verify or teach Bash#, or before writing
  any Bash# — the sections are the only syntax you may copy from. NOT a
  reference: a construct that is not in a section is not promised here.
metadata:
  check-tour: ./check.sh
  check-one: bashy --bashsharp NN-name.bsh
  step-lowered: BASHSHARP_SH_ROOT=/path/to/sh ./check.sh
---

# Bash# tour — agent procedure

Bash# is **alpha**. Every section below runs on a released `bashy` with no
model, no API key, no network. The transcripts are the contract: a section is
understood when your run matches its `.expected` byte for byte.

## Preconditions

1. `bashy --version` prints a version. If not, install from the bashy
   Releases page (unpack, put `bashy` on `PATH`). Do not build from source
   for this tour.
2. `cd` to this directory. Every command below is relative to it.
3. If your `bashy` predates the rename it may not know `--bashsharp`; then
   export `BASHSHARP_FLAG=--bashpp` and read `--bashsharp` as that flag
   throughout.

## Procedure

Run the gate first. It tells you what this binary can show:

```sh
./check.sh
```

Expected: `tour: N passed, 0 failed, M skipped` and exit 0. A `SKIP` line
names what is missing (python3; go >= 1.27; an `sh` checkout for the
lowered build). A `FAIL` prints a unified diff against the pinned transcript
— stop and report it with the diff; do not edit an `.expected` file to make
it pass.

Then take the sections in order. For each: read the `.bsh` (the comment
block at the top says what to notice), run it, diff against `.expected`:

| # | run | expect |
|---|---|---|
| 00 | `bashy --bashsharp 00-bash-is-bash.bsh` and `bashy --no-bashsharp 00-bash-is-bash.bsh` | identical output both ways — a Bash 5.3 script means the same thing with the dialect on or off |
| 01 | `bashy --bashsharp 01-go-values.bsh` | `:=`, a typed `func`, a call written as a word (`twice(y)`); `"$x"` expands a Go value as text |
| 02 | `bashy --bashsharp 02-kwargs-defaults.bsh` | `greet("Ada")` applies the default; `greet(retries: 7, name: "Cy")` binds by name |
| 03 | `bashy --bashsharp 03-decorators.bsh` | `@tag(...)` above a `func`; the decorator takes `c *Call`, calls `c.Next()`; its arguments are re-evaluated on every call (`late-a` then `late-b`) |
| 04 | `bashy --bashsharp 04-enums.bsh` | `type Color enum { Red; Green }`; the `switch` covers every member |
| 05 | `bashy --bashsharp 05-readonly.bsh` | reads through an alias work; the write in the subshell is refused with `BASHPP-EREADONLY-MUTATION`; the value is unchanged after |
| 06 | `bashy --bashsharp 06-contracts.bsh` | `@require` runs before the body (a failure exits **3**, the body never runs); `@ensure` runs after (exits **3** on disagreement); `$RESULT` is a typed return; a check's own variables never leak (`probe=unset`) |
| 07 | `bashy --bashsharp 07-agentic-yield.bsh` | six calls, exit codes **0 3 3 1 1 6**: ok · require-fail · ensure-fail · guard-denied write · fail · **yield** — `return 6` inside an `agentic` body means *input required* and reaches the caller with no ensure run |
| 08 | `bashy check --bashsharp 08-null-safety.bsh` | exit **2** and `BASHPP-ENULL-DEREF: p may be nil when dereferenced`; the file is checked, not run |
| 09 | `bashy --bashsharp 09-python-island.bsh` | a tilde-fenced Python block named `py`; `py.major()` and `py.shout("islands")` are ordinary calls from shell text (needs python3) |
| 10 | `bashy --bashsharp 10-transpile.bsh`; then `bashy transpile --bashsharp 10-transpile.bsh -o t.go` | the direct run matches the transcript; the emitted `t.go` is ordinary Go that builds inside a module which `replace`s `mvdan.cc/sh/v3` with a `github.com/qiangli/sh` checkout (`check.sh` does this when `BASHSHARP_SH_ROOT` is set) |

## Rules when you write Bash# afterwards

- **Copy shapes from these sections.** Bash# admits a construct only at a
  measured *start site*; a shape that is not here may parse as plain bash and
  do something else silently. Two you will reach for that are NOT here yet:
  a top-level Go `if x != 0 { … }` block in shell text (planned), and a Go
  call inside keyword-argument position (`greet(retries: twice(2))` — bind it
  to a name first, as `10-transpile.bsh` does).
- **Never write a tilde fence inside a comment**; the island scanner takes it
  for a real fence.
- **Every claim about Bash# names its corpus.** The numbers are in the
  language repo's `docs/claims.md`; do not quote others.
- With the dialect **off** (`--no-bashsharp`, `--posix`, or the `bash`
  binary) none of the above exists — that is the compatibility guarantee, and
  section 00 is its proof.

## Reporting

When asked to report on this tour, give: the gate's summary line, each SKIP
with its reason, and — for any FAIL — the diff verbatim. Say which `bashy
--version` you ran.
