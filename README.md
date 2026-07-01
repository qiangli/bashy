# bashy — a pure-Go Bash 5.3 drop-in

`bashy` is a single static binary that runs Bash scripts and interactive
sessions. It is written entirely in Go (no CGo, no system Bash required) and
is a **drop-in replacement for `bash` 5.3** — same command-line flags, same
script semantics, same `$BASH_VERSION` — that **passes GNU Bash's own 5.3 test
suite** (every runnable fixture; see Status below).

It is built on the [`qiangli/sh`](https://github.com/qiangli/sh) fork of
[`mvdan.cc/sh`](https://github.com/mvdan/sh), which carries the Bash 5.3
interpreter work. `bashy` is the user-facing shell; `sh` is the library.

> **Status:** `bashy` passes **100% of GNU Bash's own 5.3 test suite** — every
> measured fixture (86/86: 0 failing, 0 skipped) on Linux and macOS (see
> [`docs/TODO.md`](docs/TODO.md)). That includes job control, coprocesses,
> signal traps, and locale-aware (non-UTF-8) globbing — features the early
> goroutine-based runner couldn't do, now implemented.
>
> **Known limitations:** arithmetic uses the native int width, so 64-bit
> values on 32-bit builds (`GOARCH=386`) truncate (a 64-bit-int migration is
> tracked); and Windows builds and runs but its full test-suite run is still
> being verified. As in Bash itself (`jobs.c` vs `nojobs.c`), OS-level job
> control is a Unix feature.

## Why

- **No dependencies.** One binary. No `bash`, no shared libraries, no package
  manager. Drop it on any host (including minimal containers and Windows) and
  run your scripts.
- **Cross-platform.** The same shell semantics on Linux and macOS (verified
  against Bash's test suite); Windows builds and runs, with full verification
  in progress.
- **Embeddable lineage.** The engine underneath (`mvdan.cc/sh`) is a mature,
  widely-used Go shell library, so behaviour is well-tested and hackable.

## Install

### Download a release binary

Grab the archive for your platform from the
[Releases](https://github.com/qiangli/bashy/releases) page and put `bashy` on
your `PATH`:

| Platform | Asset |
| --- | --- |
| Linux x86-64 | `bashy-linux-amd64.tar.gz` |
| Linux arm64 | `bashy-linux-arm64.tar.gz` |
| macOS Intel | `bashy-darwin-amd64.tar.gz` |
| macOS Apple Silicon | `bashy-darwin-arm64.tar.gz` |
| Windows x86-64 | `bashy-windows-amd64.zip` |
| Windows arm64 | `bashy-windows-arm64.zip` |

```sh
# Linux/macOS example
tar -xzf bashy-linux-amd64.tar.gz
sudo install bashy /usr/local/bin/bashy
bashy --version
```

### With Go

```sh
go install github.com/qiangli/bashy@latest
```

### From source

If `bashy` is already installed on the outpost, the source build can dogfood
bashy's own tool surface: no host `git`, `curl`, `wget`, or `make` required.
`bashy git` gets the sources, `scripts/bootstrap-siblings.sh` uses `bashy git`
for sibling checkouts, and `./bashy dag build` runs the build through `bashy go`
(bashy's self-provisioning Go front end):

```sh
bashy git clone https://github.com/qiangli/bashy
cd bashy
./scripts/bootstrap-siblings.sh
./bashy dag build          # -> bin/bash and bin/bashy
./bashy dag install        # optional: install into GOBIN
```

On a host build of bashy with the container engine enabled, the same checkout can
also be built inside a container with the host filesystem mounted through
`bashy podman`; use that lane when the outpost should not depend on host build
packages beyond the already-installed bashy.

The traditional host-tool path also works:

```sh
git clone https://github.com/qiangli/bashy
cd bashy
# bashy resolves the sh engine as a flat sibling. This clones it next door
# (../sh) at the SHA pinned in .sibling-pins:
./scripts/bootstrap-siblings.sh
make build          # -> bin/bashy
```

For a fresh checkout that wants to dogfood the DAG runner before this checkout's
new `bin/bashy` exists, use the repo-local bootstrap launcher:

```sh
./bashy dag build
./bashy dag install   # installs bash/bashy into GOBIN; then `bashy dag ...` works
make dag ARGS=build   # equivalent bootstrap path if you prefer make
```

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

Startup files: interactive shells read `~/.bashyrc` (or `--rcfile`); login
shells read `/etc/profile` and `~/.bashy_profile`; `$BASH_ENV` is honoured for
non-interactive shells.

## Compatibility notes

`bashy` is a pure-Go runner: subshells are goroutines rather than `fork()`,
and process substitutions use real named pipes. Job control
(`jobs`/`fg`/`bg`/`kill %n`/`suspend` with stopped-state tracking),
coprocesses, and signal traps are implemented and pass Bash's test suite on
Unix. Mirroring Bash's own design (`jobs.c` on Unix, `nojobs.c` elsewhere),
the OS-level job-control machinery is Unix-only; on other platforms it
degrades exactly as a no-job-control Bash does.

Two known gaps: arithmetic currently uses the native int width (64-bit on
64-bit platforms), so very large values on 32-bit builds truncate — a tracked
int64 migration; and the Windows test-suite run is still being verified.
Everything else — parameter expansion, arrays and associative arrays,
namerefs, `[[ ]]`, arithmetic, here documents, brace/tilde/glob expansion
(locale-aware, including non-UTF-8 charsets such as Big5/Shift-JIS), traps,
`printf`, `read`, prompt escapes — matches Bash 5.3 and is verified against
Bash's own test suite.

## Development

See [`CLAUDE.md`](CLAUDE.md) for the development workflow and
[`docs/`](docs/) for the compliance roadmap and per-fixture analyses. The
compliance suite is driven by `make test-bash` (requires a local
`external/bash-5.3` symlink into a Bash source tree — see CLAUDE.md).

## License

BSD 3-Clause (inherited from `mvdan.cc/sh`). See [`LICENSE`](LICENSE).
