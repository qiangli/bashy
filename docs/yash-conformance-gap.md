# yash POSIX-suite conformance gap — triage worklist

> **Update 2026-07-02 — the "gap" is largely a MEASUREMENT ARTIFACT; bashy is at bash-parity.**
> Re-measured every `-p` cluster against a *genuine* bash oracle (macOS bash
> 5.3.15 + Linux bash 5.2) using yash's own `run-test.sh` (drive with
> `bash -O expand_aliases`; the `.tst` sets `posix=` so the testee runs as
> `sh`). Computing the true delta — cases where **bashy FAILS but bash PASSES**
> (`comm -23` of each shell's failing set) — collapses the headline "112-case
> gap" to **essentially one real bug**:
> - **error-p (36)**: bash fails all 36 *identically* to bashy — the "assignment
>   error … kills non-interactive shell" cases are input-mode dependent; the
>   harness feeds via **stdin** where bash (ksh93-emulation) *continues*. NOT a
>   delta. Do not "fix" these — it would diverge bashy FROM bash.
> - **alias-p / trap-p**: bashy fails *fewer* cases than bash → at/above parity.
> - **The lone genuine delta was `redir-p:246`** (`exec 3<&-` then `<&3`),
>   **Linux-only** (the Go runtime holds OS fd 3, so the redirect re-dupped a
>   live runtime fd instead of erroring). **FIXED 2026-07-02** in `../sh`
>   (`interp` now tracks logically-closed inherited fds in `r.fdClosedTable`);
>   gated serial `make test-bash` 86/86 + `redir-p` cluster 0 deltas on Linux.
>
> **Method rule:** always classify a yash "delta" as bashy-FAILS-but-bash-PASSES
> against a real bash oracle before treating it as a bug — and measure on
> **Linux** (a macOS-only run misses Linux-Go-runtime-fd bugs like redir-p:246).
> The cluster table below is a stale 2026-06-27 snapshot kept for history.

> **Update 2026-06-29 — re-measured at 96%.** A fresh `scripts/yash-posix-suite.sh`
> run puts **bashy at 96% on both panels** (alpine **1763 OK / 63 ERROR of 1826**,
> debian **1777 / 61 of 1838**) — pass rate now **≥ bash** (alpine 95%, debian
> 94%) and tied with mksh for best. The ERROR tail fell **160 → ~61** since the
> 2026-06-27 table below. The cluster worklist below remains the map for the
> residual tail; re-run the suite to regenerate exact per-case verdicts before
> picking the next cluster (the table/counts here are the 2026-06-27 snapshot).

Status: **measured 2026-06-27** (alpine panel, `scripts/yash-posix-suite.sh`).
This is the concrete long-tail worklist behind the conformance statement's "yash
row." It converts the headline "bashy 90% vs bash 5.3 95%" into the exact cases
where **bashy errors but bash 5.3 passes**, clustered by root cause.

## The number

On yash's own `-p` POSIX suite (the strictest POSIX shell's adversarial suite),
run in one container with all shells under the identical yash test framework:

| shell | pass | of | rate |
|---|---|---|---|
| yash | 1835 | 1840 | 99% |
| mksh | 1763 | 1835 | 96% |
| **bash 5.3** | **1758** | **1835** | **95%** |
| dash | 1758 | 1835 | 95% |
| ash | 1745 | 1835 | 95% |
| loksh | 1738 | 1835 | 94% |
| zsh | 1672 | 1835 | 91% |
| **bashy** | **1653** | **1835** | **90%** |
| ksh93 | 1675 | 1852 | 90% |

bashy sits mid-pack (≈ zsh / ksh93), ~5 points behind bash/dash/mksh. The
**bashy-errors-but-bash53-passes delta = 112 cases** (alpine panel). Closing it
is the difference between "clean on sampled corpora" and "matches bash on the
strict suite." Note: the absolute ceiling is ~95% even for bash — the residual
~5% are yash-specific or interactive assertions no mainstream shell passes; the
target is **parity with bash 5.3 (close the 112-case delta), not 100% of yash.**

## Clusters (112 cases)

| cluster | n | nature | leverage |
|---|---|---|---|
| `error-p` | 39 | mostly one pattern (below) | **high — likely 1–2 root causes** |
| `alias-p` | 30 | alias-substitution edge positions | **high — one subsystem** |
| `quote-p` | 6 | line-continuation + single-quote-in-expansion | medium |
| `trap-p` | 4 | `trap` default-reset + `-p` printing | medium |
| `startup-p` | 4 | first-operand-is-`-`/`--` handling, `$0` with `-s` | medium |
| `redir-p` | 4 | redirection order, fd closing, quote removal | medium |
| `exit-p`/`errexit-p` | 4 | exit-status edge cases, negated pipeline errexit | low |
| `exec-p` | 3 | exec-in-grouping, `$$` of exec'd proc | low |
| others | 18 | dot/declutil/cmdsub/shift/return/readonly/read/pipeline/param/option/function/fsplit/export/arith (1–2 each) | tail |

### error-p (39) — dominated by ONE pattern

36 of 39 are literally **"assignment error on command `<X>` in subshell"**
repeated across ~30 command names (`[`, `alias`, `array`, `bg`, `cat`, `cd`,
`command`, `echo`, `false`, `fc`, `fg`, `getopts`, `hash`, …). That repetition
across unrelated commands means it is almost certainly **a single root cause**
in how `sh` handles a variable-assignment error (e.g. assigning to a readonly
var) in the prefix of a command **inside a subshell** — fix once, flip ~30
cases. The remaining 3 are the sibling "…spares interactive shell" assertions
(#119 no-command, #331 in a `for` loop, #89 expansion error) — verify bashy's
interactive-vs-non-interactive exit discipline matches POSIX (XCU §2.8.1).

### alias-p (30) — alias substitution in syntactic positions

Basic alias expansion works (cross-line define→use, alias-to-blank chaining were
re-verified by hand). The failures are **edge positions** where POSIX defines
whether alias substitution applies:

```
alias substitution to: here-document / here-document operand / ! / parenthesis
  / if-then-elif-else-fi / while-until-do-done / for / do-done(for) / case-esac
  / case pattern / ( | ) ;; (case) / function definition / line continuation
alias ending with blank ; alias as part of an operator ; printing all aliases
using alias after assignment (complex) ; using aliases in compound commands
```

This is one subsystem in `sh` (the alias-expansion pass in `syntax`/lexer):
when the alias value contains/abuts a reserved word, operator, or compound-command
boundary. Grind these together against the in-container oracle.

## How to reproduce / regenerate the delta

```sh
# Re-run the suite with a per-case verdict dump:
OCI="bashy podman" scripts/yash-posix-suite.sh /tmp/yv

# Cases where bashy ERRORs but bash 5.3 passes (alpine panel):
norm() { sed -E 's/^%%% (OK|ERROR)\[[^]]*\]: ([^ ]+): .*/\2 \1/' "$1" | grep -E ' (OK|ERROR)$'; }
join <(norm /tmp/yv/alpine.bash53.verdicts | sort) \
     <(norm /tmp/yv/alpine.bashy.verdicts  | sort) \
  | awk '$2=="OK" && $3=="ERROR"{print $1}'        # → the 112-case worklist
```

The yash checkout is a gitignored runtime clone (`.yash-tests/`, GPL — never
vendored). The individual `*-p.tst` case bodies are the spec for each assertion;
read the failing case there, reproduce it against `docker/bashy podman run
bash:5.3 bash --posix`, then fix in `../sh`.

## Triage discipline (per case)

Each delta case → one bucket, same gate as every conformance fix:

1. **Real bug** → fix in `../sh` (`interp`/`expand`/`syntax`), gated:
   `cd ../sh && go test ./...` green **and** `cd bashy && make test-bash` still
   **86/86** (no regression). Re-run `yash-posix-suite.sh` to confirm the flip
   and watch for collateral regressions in other clusters. Then bump pins.
2. **Wording/format artifact** → yash's framework can assert exact messages POSIX
   does not mandate. If bashy is semantically correct (exits/continues per spec)
   and only the message differs, record it as a yash-framework artifact, not a
   gap — but confirm against bash 5.3 first (bash passing the case means bash
   produces what yash expects, so the bar is bash-parity, not just "POSIX-ish").
3. **Out of scope** → utility/interactive assertion not in bashy's conformance
   scope (`conformance-statement.md` §Scope). Record the exclusion.

## Suggested fleet plan (WS3)

Two disjoint high-leverage workers first (≈60% of the gap), then the medium
clusters in parallel:

- **Worker A — error-p subshell assignment-error** (~36 cases, likely 1 root
  cause in the subshell command-prefix assignment path).
- **Worker B — alias-p substitution positions** (~30 cases, the alias-expansion
  pass when the value hits reserved words / operators / compound boundaries).
- Then **quote-p / trap-p / startup-p / redir-p** as separate small workers.

Re-measure the full yash suite after each merge; the goal is bashy's pass count
reaching bash 5.3's (≈1758/1835) with no bash-5.3-fixture regression.
