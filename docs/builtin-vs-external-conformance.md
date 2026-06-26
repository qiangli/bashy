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

**Result (2026-06-25): 55/57 names classify identically to bash 5.3.** The only
two differences are the additive fork builtins below.

| Outcome | Names |
|---|---|
| **Identical to bash 5.3** (`builtin`/`file`/`keyword`) | all POSIX special + regular built-ins, all bash extensions, and every external sampled (`ls`,`grep`,`sed`,`awk`,`cat`,`env`,`which`,`sort`,`head`,`tail` → `file`) |
| **Intentional, additive delta** | `nohup`, `setsid` — **builtin** in `bin/bash`, **file** in stock bash |

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

## Fork additions — the 2 documented deltas

`nohup`, `setsid` are **builtins in `bin/bash`** (and the matrix shell / `outpost`
SSH) where stock bash leaves them external. This is **additive**: it lets
`nohup foo &` / `setsid foo` survive a closed SSH session in the in-process
interpreter (which has no child shell process to outlive the connection). The
qiangli/sh fork also promotes `disown`/`kill` to first-class builtins for the
same reason. A script that relied on `type nohup` reporting `file` would see
`builtin` — the only observable difference, and intentional.

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
# → "55+ names checked; 2 disagreement(s) (expected: nohup, setsid)"
```
