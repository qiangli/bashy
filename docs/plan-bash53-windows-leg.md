# Plan: the Bash 5.3 fixture suite on Windows (Sprint 216, story 543)

**Status (2026-09-20):** shipped in this repo; **no Windows measurement exists
yet.** The count for Windows is whatever `.github/workflows/conformance.yml`'s
`bash53-windows` job publishes on its first `workflow_dispatch` run — nothing
below, in `CLAUDE.md`, or in the Bash# claims table may quote a Windows number
before that artifact exists. 86/86 is a claim about the canonical hosts
(native macOS serial, the Linux container gate) and stays that way until
Windows is measured.

## What was wrong

- `tools/bash53suite` **failed closed on Windows**: `armParentDeathWatch`
  returned "unsupported", so not one fixture had ever run there. The harness
  contract — a fixture and every descendant die with the harness, and never
  outlive their iteration — was provided on Unix by `yoke/pkg/procguard`
  (process group + a helper process), for which Windows has no equivalent.
- The helper build hard-coded `cc`; a GitHub Windows runner has MinGW `gcc`
  and no `cc`.
- The fixture `PATH` was the Unix list (`/usr/bin:/bin:/usr/local/bin`), which
  names nothing on Windows. The pure `bin/bash` drop-in carries no userland, so
  every `cat`/`sed`/`awk`/`diff` in a fixture would be "not found".
- `EnsureBash53Fixtures` refused to continue when the `external/bash-5.3`
  symlink could not be created (a privilege on Windows).
- Separately, `todo:1ec1081071d7`: on Windows `echo $PATH` through a pipe
  printed `…;$HOME\s213` — Stage 0 output canonicalization rewrote the native
  home prefix to a `$HOME` token that the shell (whose `$HOME` is `/c/Users/…`)
  can never re-expand, so a PATH copied back from that display cannot find
  anything under the profile directory.

## What shipped

### Containment: Windows Job Objects (`tools/bash53suite/proc_windows.go`)

Two nested kill-on-close jobs, the platform's own process tree:

1. **The harness job.** Created once; the harness assigns *itself*. Every
   fixture is then born inside it (membership is inherited at creation), so
   there is no child-before-watcher window. Its handle is never closed: the
   implicit close at process death — normal, panic, or `TerminateProcess` —
   terminates everything left in the job. That is the kill-on-parent-exit
   contract, from the kernel.
2. **A per-fixture job**, assigned right after `Start`, carrying the memory cap
   (`JOB_OBJECT_LIMIT_JOB_MEMORY`, `BASH53_MEM_KB`; a commit past the cap fails
   instead of wedging the host — the Unix RSS poller's role) and terminated on
   reap, so background descendants of a finished fixture never outlive the
   iteration. If this assignment fails the run continues (containment still
   holds through the harness job) and says so **once** on stderr — a cap that
   silently stopped binding is the absence-of-evidence shape this repo hunts.

Fails closed: if the harness cannot contain itself, no fixture launches.
Reviewed against the abandoned run-38 commit `6ecbc2b5fbab`: its two-job design
was kept; added are the kernel-side proof (`IsProcessInJob`), the read-back of
the job limits, the 32-bit saturation of the byte limit, the once-only warning,
and the parent-death test that actually kills the guarding process.

### Tests (`proc_windows_test.go`, Windows only; run on every push by test.yml)

- a guarded child is in a job and dies when the guard is released;
- the harness itself is in a job after the first arm (kernel's answer);
- the per-fixture job carries `KILL_ON_JOB_CLOSE` and the exact memory cap,
  and carries **no** memory limit when the cap is disabled;
- a failed start releases the job handle;
- **parent death**: the test binary re-executes itself as a helper that arms a
  guard around `ping -n 60`, the parent kills the helper abruptly, and the
  child must be gone within 10 s.

Cross-OS (`main_test.go`): the fixture PATH per host, compiler resolution
(`$CC` → `cc` → `gcc` → `clang`), the binary-mode unit only on Windows. Every
push also cross-vets and cross-compiles the harness **and its test binary** for
`windows/amd64`.

### Portability of the runner

- `helperCompiler()`: `$CC`, `cc`, `gcc`, `clang`.
- `fixturePath()`: Unix list unchanged; on Windows `BASH53_TOOLS_PATH`
  (`;`-separated) or, unset, the harness's own PATH — and the list is printed
  in the run header, because a count whose PATH is unknown is not a measurement.
- Helpers built with a generated `bashy-binmode.c` (`int _CRT_fmode =
  _O_BINARY;`) so mingw's text-mode stdout does not turn every `recho` line into
  CRLF. The measured bytes are **never** normalized; instead the harness probes
  `recho a` once after the build and prints a note if a CR is still present, so
  a low count is attributed to the helpers' line discipline, not the shell.
- The private run tree also sets `TMP`/`TEMP` on Windows (what `os.TempDir`
  and native children actually read).
- `EnsureBash53Fixtures` returns the verified cache directory when the symlink
  cannot be made on Windows; the CI script passes it with `-tests-dir`.

### The measurement job

`scripts/ci-bash53-windows.sh` + `conformance.yml` job `bash53-windows`
(tags and `workflow_dispatch`, consistent with the 2026-09-17 decision that
push CI is unit/mock only). It builds `bin/bash.exe` (`CGO_ENABLED=0`) and the
runner, fetches the pinned corpus, points the fixture userland at Git for
Windows' `usr\bin`, runs the whole suite, and publishes:

- `bash53-windows.log` — the transcript;
- `bash53-windows-summary.md` — also the step summary;
- `bash53-windows-counts.json` — `listed / runnable / passed / failed /
  timed_out / skipped`, the testee version, the tools path, the timeout.

Exit 2 only when the suite could not run (build, fetch, no `Results:` line, no
verdicts). A low pass count exits 0: it is the answer, not a failure of the job.

### `todo:1ec1081071d7` at the display boundary

`internal/agentos/output_reduce.go`: `displayHome()` hands Stage 0 an empty
home on Windows, so the native home prefix is never rewritten to `$HOME` there
(on Unix `$HOME/bin` round-trips and nothing changes). Table test plus a replay
of the recorded PATH line through the same canonicalizer.

### `commands/register` PATH lookup on Windows (Sprint 216, story 543)

The tour's `commands/register` case also failed on GitHub's Windows leg, where
the checkout is on `D:\a\…` — *not* under the profile directory — with
`command not found` from inside a script. It is not the display seam, and the
interpreter's own lookup (`interp.LookPathDir` → `lookPathDirMode` → `checkStat`
→ `pathconv.JoinAbs`) converts every drive spelling correctly, which is why
source reading on macOS could not explain it.

The failing lookup was **bashy's own** `resolveCmd` (`internal/agentos/dryrun.go`),
which the commands/dry-run/verify surface uses and which re-implemented the PATH
scan. On Windows it got three details wrong at once:

- it tested the POSIX `0o111` execute bit, which `os.Stat` never reports for a
  Windows regular file, so **every** PATH hit looked non-executable;
- it never appended `PATHEXT`, so `bashy` could not match `bashy.exe`;
- it fed `os.Stat` an MSYS-form element (`/d/a/…`), which Windows reads as a
  path on the *current* drive, not drive D.

Fixed by delegating `resolveCmd`'s PATH branch to `interp.LookPathDir` — the
same lookup the shell uses, which carries the pathconv conversion, `PATHEXT`,
and the no-exec-bit Windows rule. Unix behaviour is unchanged.

The `windows=todo:1ec1081071d7` marker is replaced by an instrumented,
focused Windows gate — `TestResolveCmdWindowsPathSpellings`
(`internal/agentos/register_pathlookup_windows_test.go`, Windows-only, run on
every push by `test.yml`). It proves both `interp.LookPathDir` and `resolveCmd`
resolve a program across native-backslash, native-forward-slash, and MSYS
(`/c/…`) PATH spellings, logging each result so a residual failure localizes to
a spelling instead of leaving a bare todo. The conversion is drive-agnostic, so
the runner's C: temp dir exercises the exact code path the D:\ checkout hit.

## How to get the first number

```
gh workflow run conformance.yml        # then download the bash53-windows artifact
```

Then, and only then, add the measured line (`passed / runnable`, runner, date,
testee version) to the Bash# claims table and to `CLAUDE.md` §Conformance-suite
host safety.

## Sprint 245 (2026-09-22): the leg measures against bashy's own userland

The first two measurements (runs 35503573404 and 35659867244, both 23/86) ran
against Git for Windows' MSYS `usr\bin`, and 26 of the 63 non-passing fixtures
were the mingw-built `recho`/`zecho` writing CRLF — the C runtime's byte, not
the shell's. The leg now:

- builds the pure-Go **yoke** multicall binary (coreutils + yoke applets) from
  the pinned sibling and lays it out as a POSIX root under the private run
  tree (`-userland bin/yoke.exe`): `root/usr/bin/<applet>.exe` hard links,
  `root/usr/bin/{bash,sh}.exe` = the testee, `root/etc/passwd`; exports
  `BASHY_ROOT` for the shell's mounts (Sprint 245 story S245.1) and puts that
  `usr/bin` on the fixture PATH. `BASH53_TOOLS_PATH` still overrides it.
- serves `recho`/`zecho`/`xcase` from the harness binary itself by argv[0]
  (`tools/bash53suite/helpers.go`), hard-linked into the tests tree; no C
  compiler is consulted on Windows and stdout is binary.
- hands the fixtures `THIS_SH`, `_`, `BUILD_DIR` in the shell's POSIX spelling
  (`/d/a/.../bash.exe`) and `TMPDIR=/tmp` (TMP/TEMP stay native and are what
  `/tmp` maps to), because the corpus post-processes them as POSIX text.

The run header prints `Fixture PATH:` and `Fixture root:`; the counts JSON
carries `userland`, `tools_path` and `fixture_root`. The plan and the
per-fixture accounting live in the umbrella's
`docs/sprint-245-bash53-windows-parity-plan.md`.
