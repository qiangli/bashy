# bashy — a pure-Go Bash 5.3 drop-in

`bashy` is a single static binary that runs Bash scripts and interactive
sessions. It is written entirely in Go (no CGo, no system Bash required) and
aims to be a faithful **drop-in replacement for `bash` 5.3** — same command
line flags, same script semantics, same `$BASH_VERSION`.

It is built on the [`qiangli/sh`](https://github.com/qiangli/sh) fork of
[`mvdan.cc/sh`](https://github.com/mvdan/sh), which carries the Bash 5.3
interpreter work. `bashy` is the user-facing shell; `sh` is the library.

> **Status:** active development. Compliance against Bash's own 5.3 test suite
> is currently **72 passing / 4 failing / 11 skipped** (see `docs/TODO.md`).
> The 11 skipped fixtures depend on kernel job control / coprocesses, which a
> pure-Go goroutine-based runner cannot fully emulate.

## Why

- **No dependencies.** One binary. No `bash`, no shared libraries, no package
  manager. Drop it on any host (including minimal containers and Windows) and
  run your scripts.
- **Cross-platform.** The same shell semantics on Linux, macOS, and Windows.
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

```sh
git clone https://github.com/qiangli/bashy
cd bashy
# bashy resolves the sh engine as a flat sibling. This clones it next door
# (../sh) at the SHA pinned in .sibling-pins:
./scripts/bootstrap-siblings.sh
make build          # -> bin/bashy
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

`bashy` is a pure-Go runner: subshells are goroutines, not `fork()`. As a
result, features that require a real kernel job-control table or coprocess
pipes (`jobs`/`fg`/`bg` semantics, `coproc`, some signal-trap edge cases) are
stubbed or unsupported and report a clear hint. Everything else — parameter
expansion, arrays and associative arrays, namerefs, `[[ ]]`, arithmetic, here
documents, brace/tilde/glob expansion, traps, `printf`, `read`, prompt escapes
— targets Bash 5.3 behaviour and is verified against Bash's own test suite.

## Development

See [`CLAUDE.md`](CLAUDE.md) for the development workflow and
[`docs/`](docs/) for the compliance roadmap and per-fixture analyses. The
compliance suite is driven by `make test-bash` (requires a local
`external/bash-5.3` symlink into a Bash source tree — see CLAUDE.md).

## License

BSD 3-Clause (inherited from `mvdan.cc/sh`). See [`LICENSE`](LICENSE).
