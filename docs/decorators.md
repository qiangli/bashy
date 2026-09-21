# Bash# predefined decorators — the catalog

The decorators bashy supports for Bash# (`.bsh`, or `bashy` in agentic
mode). The list is deliberately the **minimum**: what `bashy dag` needs
(`require`, `ensure`, `guard`) plus what a script author reaches for
constantly (`effects`, `trace`, `timed`, `retry`, `timeout`, `memo`, `auth`,
`confirm`). Each is a thin
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
@guard("net,write")
@timeout("30s")
@retry(n: 3, backoff: "1s")
@require('test -n "$1"')
function deploy() { ... }
```

Outermost first: `auth` wraps `timeout` wraps `retry` wraps `require` wraps
the body. Arguments are the decorator's own, keyword (`n: 3`) or positional.

## The supported set

| decorator | arguments | what it does | connects to | exit | advisable |
|---|---|---|---|---|---|
| `@require` | `'<check>', …` | precondition: every check (a shell command run in the call's frame) must exit 0, or the body does not run | dag `Require:` | 3 | no |
| `@ensure` | `'<check>', …` | postcondition: judged after the body with `$STATUS` and `$RESULT`; a failing check invalidates the result | dag `Ensure:` | 3 | no |
| `@guard` | `"read,net"` or `effects: "…"` | the effect cap for everything the call dispatches: a command whose atlas effects exceed it is denied before it runs (126 in a dag body, non-zero in the shell); nested guards only narrow | atlas effect atoms; dag `Effects:` (advisory there) | 126 | yes |
| `@trace` | — | one OTel span `call <name>` around the call; argument count only, never values | `bashy otel` spool | — | yes |
| `@timed` | — | measures the call: one `@timed: <fn>: status=N duration=D` line on stderr; engine-supplied (`sh`), usable like any other | the `sh` engine | — | no |
| `@retry` | `n: 3, backoff: "1s"` | re-runs the chain until it succeeds or `n` attempts are spent (default schedule: autoretry's) | `pkg/autoretry` | the last attempt's | never |
| `@timeout` | `"10s"` or `d: "10s"` | cancels the chain at the deadline; under `@retry` each attempt re-arms | the context deadline | 124 | never |
| `@memo` | `["1h"]` or `ttl: "1h"` | the same name + arguments within one process returns the cached `Results` and `Status` without running the body; a non-zero status is not cached; `ttl` expires an entry. Memo caches what the call **returns** — printed output is not replayed, so use it on typed (value-returning) functions | a process-wide map | the cached | never |
| `@auth` | `via: "<cmd>"`, `as: "<principal>"` | runs `via` once per process (default `bashy tessaro status`, this host's pairing); exit 0 = authenticated and its first stdout line is the principal, exported to the body as `BASHY_PRINCIPAL`; `as:` names the principal required | `bashy login` / `bashy tessaro status` | 77 | yes |
| `@effects` | `"net,write"` or `effects: "…"` | the author's side of the guard coin: declares what the function does. A `@guard` that does not allow the declaration denies the call at the boundary (126) before the body runs; inside, the declaration is the function's own cap **and** the classification for commands the atlas does not know — so a tool bashy has never heard of runs on the author's word instead of failing as `unknown` | atlas vocabulary; `@guard`; dag `Effects:` | 126 | never |
| `@confirm` | — | human-in-the-loop **allow** per operation — not a guard: the high-impact atoms (`destroy`, `cred`, `priv`, `spend`, unknown) take their answer from `--confirm=TOKEN:yes` / `--what-if`, or the call yields; `docs/effect-derived-confirmation.md` | atlas effects, `bashy ask` | 6 | no |

"Advisable" = an advice rule may apply it. `retry`, `memo`, `timeout` never:
a policy that re-executes, replaces or cuts short a body behind the author's
back is the obliviousness hazard the advice design refuses. `effects` never:
it is the author's declaration, and only the author can make it.

## The two sides of the coin

```bash
@effects("net,write")      # what deploy DOES — the callee's declaration
function deploy() { ... }

@guard("read")             # what may run under ci — the caller's cap
function ci() { deploy; }  # denied at the boundary: write is not in read
```

## The effect vocabulary (`@guard`, `@effects`, dag `Effects:`, `@confirm`)

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

### Which commands have predefined effects

**Every command bashy ships carries predefined effects** — the certified
coreutils userland, yoke's agentic tools, bashy's own verbs (`go`, `git`,
`dag`, `podman`, `kubectl`, …) and the declarative-registry CLIs. That is a
guarantee, not an intention: yoke's `atlas_coverage_test` fails by name when a
tool enters the live registry without an atlas entry or an entry goes stale.
`bashy commands --view effects` prints the table.

Everything else is `unknown`, which no cap contains: a stranger on `PATH`
(`cc`, `ssh`, a vendor CLI), a script run by path, a function nobody
described. Two ways to describe it — both the author's word, never bashy's
guess:

- **your own function** → `@effects("exec,write")` on it: the declaration is
  checked at the call boundary, is the function's cap inside, and classifies
  the unknown commands it runs;
- **a tool you install** → `bashy commands add … --effects exec,write`: a
  registered command sits in the atlas beside the shipped ones.

`bashy inspect decorators` and this page are pinned to each other by test.

## Internal

`attest` is registered too — the rung policy advice attaches to an `agentic{}`
function so its completion is attested (`docs/function-attestation.md`). It is
the mechanism's, not an author's: not in this list, not in `inspect`.

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
