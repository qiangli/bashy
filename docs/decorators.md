# Bash# predefined decorators — the catalog

The decorators bashy predefines for Bash# (`.bsh`, or `bashy` in agentic
mode). The list is deliberately the **minimum**: what `bashy dag` needs
(`require`, `ensure`, `guard`) plus what a script author reaches for
constantly (`trace`, `retry`, `timeout`, `memo`, `auth`). Each is a thin
connection to a rod bashy already has — the atlas, the OTel spool,
`autoretry`, the context deadline, `bashy login` — never a policy of its
own. `bashy inspect decorators` prints this table from the registry; a test
pins the two to each other.

**Every one is redefinable.** A script function with the decorator's
signature and name shadows the native for that script:

```bash
func guard(c *Call) { c.Next() }   # this script's @guard does nothing
```

The one exception is policy: a decorator applied by an advice rule
(`BASHY_ADVICE`) always resolves to the native, so a script cannot switch off
what the operator imposed. See `dhnt/docs/bashpp-decorators-and-advice.md`.

## Applying one

```bash
@auth(via: "bashy tessaro status")
@timeout("30s")
@retry(n: 3, backoff: "1s")
@require('test -n "$1"')
function deploy() { ... }
```

Outermost first: `auth` wraps `timeout` wraps `retry` wraps `require` wraps
the body. Arguments are the decorator's own, keyword (`n: 3`) or positional.

## The minimum set

| decorator | arguments | what it does | connects to | exit | advisable |
|---|---|---|---|---|---|
| `@require` | `'<check>', …` | precondition: every check (a shell command run in the call's frame) must exit 0, or the body does not run | dag `Require:` | 3 | no |
| `@ensure` | `'<check>', …` | postcondition: judged after the body with `$STATUS` and `$RESULT`; a failing check invalidates the result | dag `Ensure:` | 3 | no |
| `@guard` | `effects: "read,net"` | the effect cap for everything the call dispatches; a command whose atlas effects exceed it is denied by the audit handler when `BASHY_AUDIT` is on | atlas effect atoms; dag `Effects:` (advisory there) | 126 | yes |
| `@trace` | — | one OTel span `call <name>` around the call; argument count only, never values | `bashy otel` spool | — | yes |
| `@retry` | `n: 3, backoff: "1s"` | re-runs the chain until it succeeds or `n` attempts are spent (default schedule: autoretry's) | `pkg/autoretry` | the last attempt's | never |
| `@timeout` | `"10s"` or `d: "10s"` | cancels the chain at the deadline; under `@retry` each attempt re-arms | the context deadline | 124 | never |
| `@memo` | `["1h"]` or `ttl: "1h"` | the same name + arguments within one process returns the cached `Results` and `Status` without running the body; a non-zero status is not cached; `ttl` expires an entry. Memo caches what the call **returns** — printed output is not replayed, so use it on typed (value-returning) functions | a process-wide map | the cached | never |
| `@auth` | `via: "<cmd>"`, `as: "<principal>"` | runs `via` once per process (default `bashy tessaro status`, this host's pairing); exit 0 = authenticated and its first stdout line is the principal, exported to the body as `BASHY_PRINCIPAL`; `as:` names the principal required | `bashy login` / `bashy tessaro status` | 77 | yes |

"Advisable" = an advice rule may apply it. `retry`, `memo`, `timeout` never:
a policy that re-executes, replaces or cuts short a body behind the author's
back is the obliviousness hazard the advice design refuses.

## The effect vocabulary (`@guard`, dag `Effects:`, `@confirm`)

Eleven atoms, closed, defined in `yoke/pkg/atlas` — the grammar every cap is
written in. A cap is a set of them; a command is allowed when its atoms are a
subset of the cap. `pure` is never checked.

| atom | means | e.g. |
|---|---|---|
| `pure` | no governed side effect — always allowed | `echo`, `true`, `dirname` |
| `read` | reads files / host state / input | `cat`, `pwd`, `grep` |
| `write` | mutates files or host state | `mkdir`, `tar`, `sed` |
| `destroy` | can irreversibly lose data | `rm`, `dd` |
| `net` | opens a network connection | `curl`, `git`, `go` |
| `exec` | spawns a process bashy no longer governs | `go`, `awk`, `make` |
| `cred` | reads or writes credentials | `git`, `gh` |
| `priv` | changes privilege, ownership or a label | `chmod`, `sudo` |
| `remote` | runs on another host | `kubectl`, `dag` |
| `persist` | leaves something that outlives the session | `podman`, `self` |
| `spend` | incurs metered cost | paid inference |

Which atoms a *command* carries comes from the atlas (`bashy commands --view
effects`), a table bashy curates and cannot complete — a command it does not
know is `unknown`, which no cap contains. A registered command
(`bashy commands add`) carries the effects its author declared.

## Present, not in the minimum set

| decorator | note |
|---|---|
| `@confirm` | effect-derived `--what-if` / `--confirm` per operation (exit 6 on an unanswered high-impact op); `docs/effect-derived-confirmation.md` |
| `@attest` | the pass-through rung advice puts on `agentic{}` functions so the call is attested; not for authors |
| `@timed` | the engine's own observation decorator (`sh`): one `@timed:` line with status and duration |

## Making `@guard` say more

Two escape hatches, neither a new mechanism: declare what your own command
does (`bashy commands add --effects …` today; an `@effects("net,write")`
declaration on a function is the proposed next step), or redefine `guard` in
the script with the rule you actually want — `func guard(c *Call)` sees the
call's name, arguments and principal and decides `Next()` itself.

## Writing your own

A decorator is a function whose first parameter is `*Call`:

```bash
func tag(c *Call) {
    echo "in $c.Name"
    c.Next()                 # the rest of the chain; skip it to deny
    echo "out status=$c.Status"
}
@tag()
function f() { echo body; }
```

`Call` carries `Name`, `Site`, `Caller`, `Args`, `Results`, `Status`,
`Agentic`, `Advised`; `Next()` may be called more than once (retry does).
