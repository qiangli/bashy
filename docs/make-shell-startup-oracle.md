# SHELL startup oracle and the Profile B make TP97/TP99 fix

Sanitized record for the cross-platform Profile B `make:TP97` / `make:TP99`
resolution. No licensed suite content appears here — only the TP identities,
a public GNU Bash 5.3 oracle, and a public synthetic reducer.

## The public GNU Bash 5.3 oracle

Measured against GNU bash 5.3.15 (`/opt/homebrew/bin/bash`, darwin/arm64,
2026-08-22), matrix = {absent, explicitly empty, explicit nonempty} SHELL ×
{normal, `--posix`} modes. Both modes behave identically:

| inherited SHELL | shell variable                         | exported?          | child env |
|-----------------|----------------------------------------|--------------------|-----------|
| absent          | bound to the login shell (`pw_shell`)  | **no** (`declare --`) | absent |
| `SHELL=` (empty)| kept empty                             | yes (inherited)    | `SHELL=`  |
| `SHELL=value`   | kept verbatim                          | yes (inherited)    | `SHELL=value` |

This is `variables.c` `initialize_shell_variables`: bind SHELL only when no
variable was imported, value from `getpwuid(getuid())->pw_shell`, falling
back to `/bin/sh` when the lookup fails or the field is empty; the
synthesized binding is not exported. An imported SHELL — including the
explicitly empty string — is never overwritten.

## The reducer and the first observable divergence

`scripts/make-shell-var-reducer.sh` runs make(1) over a one-target Makefile
(`SHELL = sh`) under an identical provider PATH whose only difference is
which binary `sh` resolves to — the GNU oracle or bashy's `bin/bash` — across
the three inherited-SHELL states, and diffs the outputs.

First observable divergence (pre-fix): the **absent** row. GNU's recipe
shell reported `$SHELL` set (login shell, still absent from a child's
environment); bashy's recipe shell reported it unset. The empty and
nonempty rows matched byte-for-byte. That unset `$SHELL` in recipe context
is the sanitized root cause of the make TP failures.

## The fix, and why this layer

Bashy never synthesized the SHELL startup default. The `variables.c`
startup defaults already live in `internal/cli` (PATH `set_if_not`, SHLVL
increment, POSIXLY_CORRECT aliasing, and the non-exported startup bindings
of `BASH`/`BASH_VERSION`/`_` in `withBashVersionVars`) — so the root cause
is this repo's CLI startup layer, **not** sibling `sh`. No sibling change
was needed or made.

`internal/cli/shellvar.go` adds `withStartupShellVar`: applied in
`newRunner` and `NewSessionRunner`, it wraps the startup environment only
when SHELL is not inherited, overlaying a non-exported SHELL bound to
`loginShellPath()`. There is no global default table and no overwrite path:
an inherited SHELL (empty included) returns the base environment untouched.

`loginShellPath()` is bash's `get_current_user_info` over what pure Go (no
cgo) can reach: the `/etc/passwd` entry for the current uid, `/bin/sh` when
the lookup fails or the shell field is empty. Known, documented limitation:
on hosts whose users resolve only through a directory service (macOS Open
Directory), the uid is absent from `/etc/passwd` and the `/bin/sh` fallback
applies, so the *value* differs from a cgo `getpwuid` there while set-ness,
export attribute, and child-env visibility still match GNU. On Linux —
where the Profile B lanes run — the passwd file is authoritative and the
value matches GNU exactly. Windows synthesizes nothing: GNU bash has no
native Windows build to serve as an oracle.

## Preserved neighbors

Deliberately untouched and re-verified: login-`sh` startup files, `ENV`
processing, explicitly empty `PS1` (the overlay fires on *unset*, never on
*empty* — the exact distinction the PS1/PATH precedents already encode),
and `bash --posix -i` startup. Regressions:
`internal/cli/shellvar_test.go` (argv0 {sh,bash} × {normal,posix} ×
{absent,empty,nonempty}, plus passwd-parse edge cases) and the reducer
script above.

Full public gate (`make test-bash`, native host lane, 2026-08-22): 80/86.
The six failures (execscript, invocation, read, test, type, vredir) were
A/B'd against a pre-change build on the same host: verdicts AND failure
signatures are byte-identical — pre-existing host-environment effects (no
controlling TTY under the agent harness, shared `/tmp` pollution), zero
deltas from this change. The authoritative hermetic verdict remains
`make test-bash-container` per the scoreboard-reliability policy.

Pre-existing gap noted while probing the preserved neighbors (also A/B'd,
unchanged by this fix, out of scope here): `ENV` is not sourced under
`--posix -i -c` where GNU bash sources it before running the command
string.
