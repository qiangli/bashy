# Sprint 246 · S246.3 / todo:77bf637a2545 · lane L6 — status & the locale blocker

Goal: make the three Windows bash-5.3 fixtures `glob-bracket`, `glob-test`, `intl`
pass. Two of the three (the locales) turn on a change that lives in a **different
git repository than the one this workspace commits to**, so this file records the
precise finding for the conductor/operator, per the task's "a precise finding is a
genuine deliverable" clause.

## DELIVERED here (in scope, verified): the compiler — `glob-bracket`

`glob-bracket` compiles and *runs* a `fnmatch` program with `$CC` and links a
`strmatch` loadable. On the runner `cc` was "command not found", and mingw — the
obvious `cc` — is exactly what corrupted the Sprint 245 baseline (its C runtime
writes CRLF), so it is not an option.

Fix (all committed): the harness provisions the pinned, digest-verified **`zig cc`**
the dag island languages already use (yoke `external/zigcc`) and puts a `cc` on the
fixture PATH — the way bashy provisions every toolchain, not the way a user installs
one. Two gaps in mingw are filled by the provisioned `cc`, touching **no corpus file**:

* **`<fnmatch.h>` is absent on mingw.** The provisioned `cc` injects, via
  `-isystem`, a self-contained POSIX `fnmatch.h` (musl's implementation, MIT, every
  function `static` so a single translation unit needs no extra link input) —
  `tools/bash53suite/compat/fnmatch.h`, embedded into the harness.
* **`strmatch.so` cannot link** (a Windows shared object may carry no unresolved
  import; the fixture leaves `strmatch` for bash to export at load). A stub object,
  handed to the generated Makefile via `SHOBJ_LIBS`, lets the marker `.so` be
  produced. Its contents are irrelevant on **every** platform: bashy implements
  `strmatch` as an internal builtin and `enable -f` never loads the file.

Verified on this macOS host by cross-compiling to `x86_64-windows-gnu`
(`BASH53_CC_PROVISION_TEST=1 go test ./tools/bash53suite`):
`TestCompatFnmatchHeaderBuildsAndMatches` (fnmatch.exe builds **and** its results
are byte-identical to `glob.right` on all 18 cases the fixture actually feeds the
binary) and `TestStrmatchStubLinksSharedObject` (a PE DLL links). `make
test-bash-run TESTS="glob-bracket glob-test intl"` stays 3/3 on macOS (no
regression to the native path).

Files: `tools/bash53suite/cc_provision.go`, `tools/bash53suite/compat/fnmatch.h`,
`tools/bash53suite/cc_provision_test.go`, `tools/bash53suite/main.go` (wiring),
`scripts/ci-bash53-windows.sh` (header).

## BLOCKED here (out of this workspace): the locales — `glob-test`, `intl`

### The finding

Both fixtures gate on a locale being *installed*, then do locale-sensitive work:

* `intl` → `intl2.sub`: `if locale -a | grep -i '^de_DE\.UTF.*8'` then
  `LANG=de_DE.UTF-8 printf '%.4f\n' 1` must print `1,0000` (German decimal comma).
* `glob-test` → `glob2.sub`: `if locale -a | grep -i '^zh_TW\.big5'` then multibyte
  `read`/`printf`/`[[ ]]` under `LC_ALL=zh_TW.big5`.

When the `grep` fails, the sub prints a `warning: you do not have the … locale
installed` line to stderr, which the harness folds into the compared output — so
the fixture diverges from its `.right` at the warning, exactly as measured at M13.

**The locale BEHAVIOUR is already 100% bashy-engine-side (pure Go), not the OS's.**
Proven on this macOS host with **no OS locale involved**, using the freshly built
pure drop-in:

```
$ LANG=de_DE.UTF-8 ./bin/bash -c "printf '%.4f\n' 1"        # -> 1,0000
$ LC_ALL=zh_TW.big5 ./bin/bash -c $'[[ \u3b1 = \u3b1 ]] && echo ok7'   # -> ok7
$ LC_ALL=zh_TW.big5 ./bin/bash -c $'a=$\'\\u3b1\'; [[ $a = $a ]] && echo ok6'  # -> ok6
```

So on Windows the shell would compute the right answers. The **only** gap is the
`locale -a` guard. On the canonical hosts the guard is answered by the *OS* `locale`
(macOS and the Linux container image both ship these locales); on Windows the
fixtures resolve `locale` from bashy's own userland, and **bashy's `locale -a`
does not list them**:

* `../coreutils/cmds/locale/available.go` → `availableLocales()` returns a
  hardcoded `{ "C", "POSIX", "de_DE.ISO-8859-1" }`. Its own comment sets the rule:
  advertise only what the built-in database can serve.

The truthful fix is to add `de_DE.UTF-8` and `zh_TW.big5` to that list, because
bashy's engine genuinely serves them (evidence above). That is **not a hack, not a
warning-suppression, and not a corpus edit** — it makes bashy's `locale` tell the
truth about the locales bashy implements, which also serves a real Windows user who
sets `LANG=de_DE.UTF-8` and gets German formatting from bashy.

### Why it is not committed from here

`availableLocales()` is in **`../coreutils`**, a *separate git repository*
(`…/weave/bashy-6497d06f/workspaces/coreutils`). This workspace is the **bashy**
repo on branch `agent/weave-issue-7`; the weave contract has the conductor pull
*this* branch only. A change to the coreutils working tree here would neither be
committed by this task nor pulled — it would silently vanish. The workspace contract
("stay inside your workspace; never follow git remotes") forbids reaching over to
commit/push the sibling.

### Alternatives investigated and rejected

* **Windows NLS / ICU providing the locale.** Even if the runner's OS carries
  zh-TW/Big5 or de-DE, the fixture's `locale -a` is **bashy's** applet, which does
  not consult Windows NLS. So an OS-level locale changes nothing here. (`localedef`
  is also absent on Windows, as the task notes — a POSIX charmap cannot be compiled
  in on the runner.)
* **A harness-provided `locale` shim on the fixture PATH.** Technically in-scope and
  would flip both fixtures, but it serves *only the CI runner* — the exact thing the
  task warns against ("must serve a real Windows user"). The honest home for the
  truth "bashy serves de_DE.UTF-8 and zh_TW.big5" is bashy's `locale` applet, i.e.
  coreutils. Deliberately not done.

### The exact change the operator/conductor should apply in `../coreutils`

`../coreutils/cmds/locale/available.go`, `availableLocales()` — add the two names
bashy already serves (gate `zh_TW.big5` on whatever the built-in ctype database can
actually carry; `de_DE.UTF-8` pairs with the German data already used by the `date`
/ `printf` paths and the existing `de_DE.ISO-8859-1` entry):

```go
return []string{"C", "POSIX", "de_DE.ISO-8859-1", "de_DE.UTF-8", "zh_TW.big5"}
```

Then re-measure via `scripts/ci-bash53-windows.sh` (or the umbrella's Windows leg).
With the guard satisfied, both subs run the engine paths already proven above and
should reach their `.right`. (If `zh_TW.big5`'s BIG5 collation is *not* fully carried
by the coreutils database, advertise only `de_DE.UTF-8` for `intl` and keep
`glob-test` as a documented residual — but the engine `[[ ]]`/`read`/`printf` probes
that `glob2.sub` performs are ctype, not collation, and pass in pure Go above.)
