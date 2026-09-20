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

**What this does not settle.** The tour's `commands/register` case also failed
on GitHub's Windows leg, where the checkout is on `D:\a\…` — *not* under the
profile directory — with `bashy: command not found` from inside a script. That
cannot be the display seam, and nothing in the interpreter's PATH lookup
(`interp.lookPathDirMode`, case-insensitive env, `PATHEXT`) explains it from
source reading on macOS. It needs a Windows run with the lookup instrumented.
Keep the tour's `windows=todo:1ec1081071d7` marker until that run passes.

## How to get the first number

```
gh workflow run conformance.yml        # then download the bash53-windows artifact
```

Then, and only then, add the measured line (`passed / runnable`, runner, date,
testee version) to the Bash# claims table and to `CLAUDE.md` §Conformance-suite
host safety.
