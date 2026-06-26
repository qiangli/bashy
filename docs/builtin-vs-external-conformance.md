# Builtin vs external command table — bash 5.3 + POSIX conformance

**Why this is a conformance item.** Whether a name runs as a *builtin* or an
*external* command is part of shell behavior, not an implementation detail. POSIX
splits utilities into **special built-ins** (distinct semantics — a redirection
or assignment error is *fatal* to a non-interactive shell, and a preceding
variable assignment *persists* in the current environment) and **regular
built-ins** (may be implemented in-process but behave like external utilities).
A faithful bash drop-in must agree with bash on which names are built in, or
constructs like `name=val special_builtin` and error-exit behavior diverge.

## Verification (automated, reproducible)

`scripts/verify-builtins.sh` compares `type -t <name>` between `bin/bash` (the
pure drop-in) and real **bash 5.3** in the same container PATH, across every
POSIX special + regular built-in, the bash 5.3 extensions, and a sample of real
externals.

**Result (2026-06-25): 72/72 names classify identically to bash 5.3 — ZERO
disagreements.** Every POSIX special + regular built-in, every bash extension,
and every external sampled (`ls`,`grep`,`sed`,`awk`,`cat`,`env`,`which`,`sort`,
`head`,`tail` → `file`) matches bash 5.3 exactly — including `nohup`/`setsid`,
which the pure `bin/bash` drop-in disables (see below) so they resolve to the
external command like bash.

## POSIX special built-ins (special semantics) — all present, all `builtin`

`:`  `.`  `break`  `continue`  `eval`  `exec`  `exit`  `export`  `readonly`
`return`  `set`  `shift`  `times`  `trap`  `unset`

(15 of 15. These carry the special-built-in rules: assignment errors / redirection
errors are fatal in non-interactive mode; a variable assignment in the same simple
command persists after the builtin returns.)

## POSIX regular built-ins (intrinsic utilities) — all present

`alias`  `bg`  `cd`  `command`  `false`  `fc`  `fg`  `getopts`  `hash`  `jobs`
`kill`  `newgrp`*  `pwd`  `read`  `true`  `type`  `ulimit`  `umask`  `unalias`
`wait`

(*`newgrp` is recognized but prints an unsupported-hint — it requires a real
process-group/credential change the in-process model can't perform; bash needs a
suid helper too. All others are functional.)

## bash 5.3 extensions (beyond POSIX) — all present, classified as bash does

`bind`  `builtin`  `caller`  `compgen`  `complete`  `compopt`  `declare`/`typeset`
`dirs`  `disown`  `echo`  `enable`  `help`  `history`  `let`  `local`  `logout`
`mapfile`/`readarray`  `popd`  `printf`  `pushd`  `shopt`  `suspend`  `test`/`[`

(Completion-programming — `compgen`/`complete`/`compopt` — and interactive
job-display refinements are recognized; see `vsc-pcts-readiness.md` for the
interactive job-control limitation. Everything scriptable behaves as bash.)

## Full shadow audit — the 4 fork builtins gated in the strict drop-in

A complete audit of all 65 engine builtins (every `IsBuiltin` name, `type -t` vs
bash 5.3) found **exactly 4** that bash 5.3 does NOT have as builtins, so the
pure `bin/bash` drop-in disables them via `cli.SuppressedForkBuiltins`
(`interp.WithDisabledBuiltins`, the programmatic `enable -n`). `cmd/bashy` clears
the list to KEEP them. Every other builtin that shadows a standard external
(`echo`,`printf`,`test`,`[`,`kill`,`pwd`,`true`,`false`,`command`,`type`,`umask`,
`read`,`getopts`,…) is **bash-faithful** — bash builtins them too, so the shadow
matches bash and needs no gating.

| Gated builtin | Why the fork added it | What `bin/bash` does instead |
|---|---|---|
| `nohup` | in-process detach over a closed SSH session | runs `/usr/bin/nohup` (the real one) |
| `setsid` | same (new session for a goroutine "job") | runs `/usr/bin/setsid`; "not found" on macOS — **matching macOS bash** |
| `newgrp` | a non-functional stub (`not supported`) — can't switch group in-process | runs `/usr/bin/newgrp`, which **actually works**, like bash |
| `strmatch` | an engine-only pattern-match builtin bash never had | "not found" — the drop-in carries no non-bash command |

Notes:
- **`newgrp` was the real find**: ours's stub *shadowed the working external*, so
  `bin/bash newgrp staff` errored where bash switches group. Gating fixes both the
  classification AND the functionality.
- **`strmatch`** doesn't shadow any external (no `/usr/bin/strmatch`); disabling
  the user-facing builtin does NOT affect the internal `[[ ]]` matcher (a direct
  Go call, verified).
- **`disown`/`kill`** are fork builtins too, but **bash 5.3 has them as builtins
  as well**, so they match in both binaries and need no suppression.
- **Users of `bin/bashy` opt out** per-command with the bash-native
  `enable -n <name>` — restoring the external command.

Tested both paths: `sh/interp` `TestDisabledBuiltinNohupFalls
ThroughToExternal` (external/disabled + Reset-survival + it runs the command) and
`TestNohupNoTTYInheritsStdio` / `TestNohupChildIsInNewSession` (builtin behavior +
the new-session hangup-survival mechanism).

## Everything else is external

Every name *not* in the tables above (`ls`, `grep`, `sed`, `awk`, `cat`, `find`,
`env`, `which`, `sort`, …) is resolved through `PATH` exactly as bash does —
`type -t` returns `file`. **Verified in source:** `ls` is not a builtin in bash
5.3 (`builtins/*.def` has no `ls.def`) nor in Oils/OSH (`frontend/builtin_def.py`
has no `ls`); all three shells fork it. (Note: the *other* binary, `bin/bashy`,
adds an in-process coreutils fallback so `ls`/`cat`/… resolve without PATH on
minimal hosts — that is an AgentOS feature of `bashy`, NOT the pure `bin/bash`
drop-in, and the conformance harness only measures `bin/bash`.)

## Run it

```sh
cd bashy && make build && scripts/verify-builtins.sh
# → "72 names checked; 0 disagreement(s)"
```
