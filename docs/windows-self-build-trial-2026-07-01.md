# Windows Self-Build Trial - 2026-07-01

Purpose: validate the bashy self-build/bootstrap path on real Windows hosts,
using one host for development iteration and one cleaner host for proof/QA.

## Host Roles

- `lj2ivy`: development/iteration host.
  - `outpost v0.12.27` present.
  - `go version go1.26.4 windows/amd64` present.
  - `bashy` present, but older than the current command surface: it has `dag`
    and `go`, but not `git` or `commands`.
- `puppy`: proof/QA host.
  - `outpost v0.12.27` present.
  - No `bashy` on PATH.
  - No `go` on PATH.
  - No host `git` on PATH.
  - Has `winget` and `curl.exe`, but the desired clean path is for outpost to
    seed bashy directly, not for operators to hand-drive Windows tooling.

## Bootstrap Substrate

Both hosts successfully used outpost's existing embedded git client:

```text
outpost git clone https://github.com/qiangli/bashy.git <user>/tests/bashy-self/bashy
outpost git clone https://github.com/qiangli/sh.git <user>/tests/bashy-self/sh
outpost git clone https://github.com/qiangli/coreutils.git <user>/tests/bashy-self/coreutils
outpost git clone https://github.com/qiangli/readline.git <user>/tests/bashy-self/readline
```

The checked-out bashy commit was `6be4de5`.

Pinned sibling SHAs from `.sibling-pins`:

```text
sh=f03c2e39e91da8f658a9446c84968dcc75a01288
coreutils=dfe1bbcfee2d7a065d2171d33c5332a6d5b9c039
readline=d29dca5d747044b347dd31b7eb32cb9835af96d6
```

Observed outpost git gaps:

- `outpost git` does not support `git -C`.
- `outpost git checkout <sha>` only switches branches; it does not detach to
  an arbitrary commit SHA.
- `outpost git reset --hard <sha>` can move to an existing commit and was used
  on `lj2ivy` to align `coreutils` with the pinned SHA.

## lj2ivy Result

First local bashy build through the allowed Go dependency:

```text
cd C:/Users/Lern/tests/bashy-self/bashy
go build -trimpath -ldflags "-s -w -X github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-windows-dev" -o bin/bashy.exe ./cmd/bashy
```

Result: passed.

Bashy-owned rebuild:

```text
./bin/bashy.exe dag build
```

Result: passed (`dag: 1 target(s) ok`).

New `self` command:

```text
./bin/bashy.exe self fetch
```

Result: passed, cached the released v0.4.0 Windows binary:

```text
C:\Users\Lern\AppData\Local\bashy\bin\bashy\v0.4.0\bashy.exe
```

Command surface check:

```text
./bin/bashy.exe commands --plain --all
```

Result: passed. `self` appeared in the visible verbs; hidden aliases
`bootstrap` and `upgrade` appeared only with `--all`.

## lj2ivy Test Failure

`./bin/bashy.exe dag test` failed, but after the build had already succeeded.
The failures were caused by Go tests spawning host commands that were missing
from the clean Windows PATH:

```text
bash: line 1: env: command not found
heredoc.tests: line 1: cat: command not found
```

This is a test-environment dependency leak, not a bashy compile failure. Bashy
itself exposes `env` and `cat` through the coreutils userland, but `go test`
subprocesses do not automatically resolve those in-process tools through PATH.

Follow-up options:

- Provide test-time shims for coreutils names when `bashy dag test` runs on
  Windows.
- Make the affected Go tests invoke bashy/coreutils explicitly where the test
  intent is not host-PATH coverage.
- Keep full conformance/compatibility tests on the established harnesses and
  treat clean-Windows `dag test` as a separate bootstrap hardening target.

## puppy Result

`puppy` is the better proof/QA host because it starts without bashy and without
Go. Current state proves the bootstrap gap:

- outpost can clone bashy and siblings.
- there is no initial bashy command to run `bashy go` or `bashy dag`.
- there is no host Go toolchain to build the first local bashy.

This is exactly the use case for a small outpost-side seed command:

```text
outpost bashy
```

Expected behavior for that future command:

- Resolve latest `qiangli/bashy` release for the current OS/arch.
- Download and verify the archive using `checksums.txt`.
- Extract/cache `bashy.exe`.
- Print the installed/cached path, and optionally install to a PATH directory.

After `outpost bashy` exists and is released, `puppy` should be used as the QA
host for the full clean flow:

```text
outpost bashy
bashy git clone https://github.com/qiangli/bashy.git
cd bashy
bashy dag build
bashy self fetch
```

## Conclusion

- Current bashy `main` self-build works on Windows when the allowed Go
  dependency is present (`lj2ivy`).
- `bashy self fetch` works on Windows and correctly resolves/extracts the
  released v0.4.0 `bashy.exe`.
- Clean Windows hosts still need a seed path. The right next product step is an
  outpost `bashy` subcommand and a new outpost release, then QA that release on
  `puppy`.

## v0.4.1 Seed Release Validation

After releasing outpost `v0.12.28` and bashy `v0.4.1`, both Windows hosts were
used to validate the clean seed/self-build path.

Release seed check:

```text
outpost bashy --install-dir <test>/bin
<test>/bin/bashy.exe --version
<test>/bin/bashy.exe go version
```

Results:

```text
GNU bash, version 5.3.0(1)-bashy-0.4.1
go version go1.26.4 windows/amd64
```

Fresh-source proof used bashy's own git command to clone the required sibling
repositories:

```text
bashy.exe git clone https://github.com/qiangli/bashy.git <test>/src/bashy
bashy.exe git clone https://github.com/qiangli/sh.git <test>/src/sh
bashy.exe git clone https://github.com/qiangli/coreutils.git <test>/src/coreutils
bashy.exe git clone https://github.com/qiangli/readline.git <test>/src/readline
```

Build proof:

```text
cd <test>/src/bashy
BASHY=<test>/bin/bashy.exe <test>/bin/bashy.exe dag build -B --plain
```

Results on `puppy` and `lj2ivy`:

```text
==> build
dag: 1 target(s) ok
```

Generated binary checks:

```text
bin/bashy.exe --version
bin/bash.exe --version
bin/bashy.exe -c "echo built-bashy-ok"
bin/bash.exe -c "echo built-bash-ok"
```

Results on both hosts:

```text
GNU bash, version 5.3.0(1)-bashy-dev
GNU bash, version 5.3.0(1)-bashy-dev
built-bashy-ok
built-bash-ok
```

Unit-test proof:

```text
BASHY=<test>/bin/bashy.exe <test>/bin/bashy.exe dag test -B --plain
```

`puppy` result:

```text
?    github.com/qiangli/bashy/cmd/bash    [no test files]
?    github.com/qiangli/bashy/cmd/bashy   [no test files]
ok   github.com/qiangli/bashy/internal/agentos  3.485s
ok   github.com/qiangli/bashy/internal/cli      1.218s
?    github.com/qiangli/bashy/skills      [no test files]
dag: 1 target(s) ok
```

`lj2ivy` result:

```text
?    github.com/qiangli/bashy/cmd/bash    [no test files]
?    github.com/qiangli/bashy/cmd/bashy   [no test files]
ok   github.com/qiangli/bashy/internal/agentos  6.942s
ok   github.com/qiangli/bashy/internal/cli      2.727s
?    github.com/qiangli/bashy/skills      [no test files]
dag: 1 target(s) ok
```

Default latest resolution was also checked:

```text
outpost bashy --install-dir <test>/bin
<test>/bin/bashy.exe --version
```

Result:

```text
GNU bash, version 5.3.0(1)-bashy-0.4.1
```

Compatibility score from the release gate:

```text
86 passed, 0 failed, 0 skipped, 0 timed out
```

Conclusion: the seed path is now proven on both Windows hosts. A current
outpost can fetch the latest bashy patch release, and that released bashy can
clone, build, and test bashy without requiring host git, host bash, host sh, or
host coreutils. The only intended external dependency remains Go, supplied
through bashy.
