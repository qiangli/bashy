# Using bashy

Invocation, flags, shell modes, Bash 5.3 compatibility notes and development. Back to the [README](../README.md).
## Usage

```sh
bashy script.sh arg1 arg2      # run a script
bashy -c 'echo "$BASH_VERSION"'# run a command string
bashy                          # interactive shell
echo 'echo hi' | bashy         # read a script from stdin
```

### Supported flags

`bashy` accepts the common Bash invocation flags:

| Flag | Meaning |
| --- | --- |
| `-c <string>` | run `<string>` as a command |
| `-i` | force interactive mode |
| `-l`, `--login` | act as a login shell |
| `--posix` | POSIX mode |
| `--norc` | do not read `~/.bashyrc` |
| `--noprofile` | do not read profile files |
| `--rcfile`, `--init-file <f>` | use `<f>` as the interactive startup file |
| `-o <opt>` | enable a `set` option (e.g. `errexit`, `xtrace`) |
| `-O <opt>` | enable a `shopt` option |
| `--pretty-print` | pretty-print the parsed input |
| `--version` | print version and exit |

### Shell mode selection

Invoked as `bash` or `bashy`, the shell starts in GNU Bash 5.3-compatible
mode. Invoked with basename `sh`, it starts in POSIX `sh` mode. POSIX mode can
also be requested with `--posix`, `-o posix`, `SHELLOPTS=posix`, or by the
presence of `POSIXLY_CORRECT`/`POSIX_PEDANTIC` (including empty values).
Command-line `-o/+o posix` is last-wins unless one of the environment or `sh`
startup conditions forces POSIX mode, matching GNU Bash 5.3.

The complete contract, including strict `sh` semantics and certification
wiring, is documented in [Shell mode selection](shell-mode-selection.md).

Startup files: interactive shells read `~/.bashyrc` (or `--rcfile`); login
shells read `/etc/profile` and `~/.bashy_profile`; `$BASH_ENV` is honoured for
non-interactive shells.

## Compatibility notes

`bashy` is a pure-Go runner: subshells are goroutines rather than `fork()`,
and process substitutions use real named pipes. Job control
(`jobs`/`fg`/`bg`/`kill %n`/`suspend` with stopped-state tracking),
coprocesses, and signal traps are implemented and pass Bash's test suite on
Unix. Mirroring Bash's own design (`jobs.c` on Unix, `nojobs.c` elsewhere),
Unix process groups and controlling-terminal handoff are implemented. Windows
does not provide Unix process-group/TTY job control; its basic job carrier
provides process identity but no live signal proxy (`os/exec` rejects
`ExtraFiles`). See [job-carrier.md](job-carrier.md).

Arithmetic currently uses the native int width (64-bit on 64-bit platforms),
so very large values on 32-bit builds truncate — a tracked int64 migration.
Unix job control has shipped; the remaining interactive boundaries are listed
in [the conformance statement](conformance-statement.md#declared-limitations).
The final Sprint 253 production gate measured the Bash 5.3 fixtures at
**86/86 on Windows and Linux** (run 35812307698), and the Sprint 257 timezone
follow-up measured **86/86** on two native Windows builds, native macOS, and
Ubuntu 24.04 test droplets. These are fixture results, not a claim of full
Bash compatibility; see the umbrella's `docs/sprint-253-delivery-evidence.md`.
Everything else — parameter expansion, arrays and associative arrays,
namerefs, `[[ ]]`, arithmetic, here documents, brace/tilde/glob expansion
(locale-aware, including non-UTF-8 charsets such as Big5/Shift-JIS), traps,
`printf`, `read`, prompt escapes — matches Bash 5.3 and is verified against
Bash's own test suite.

## Development

Builds require `go 1.27` and select `toolchain go1.27.2`, as recorded in
[`go.mod`](../go.mod). Product versions come from the release tag; use
`bashy --version` to identify the installed build.

See [`CLAUDE.md`](../CLAUDE.md) for the development workflow and [`docs/`]()
for the compliance roadmap and per-fixture analyses. The Bash 5.3 suite is
driven by `make test-bash` (serial; needs a controlling terminal; `make
test-bash-fixtures` fetches the pinned fixture tree). The language, its
roadmap and RFCs live in [bashsharp/bashsharp](https://github.com/bashsharp/bashsharp).
