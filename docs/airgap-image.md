# The offline bashy image — what works inside it

**Bashy is all you need.** With nothing but the release download — no git, go,
podman or docker on the host — build the image and run your Bash# script with
no network:

```sh
bashy self image
bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<ver>-linux-<arch> --bashsharp ./script.bsh
```

The image is `FROM scratch` plus one static binary: bashy as it is — Bash 5.3,
`--posix`, Bash#, the builtin coreutils, and the yoke verbs that make sense
with no network. Nothing else is in it: no libc, no shell but bashy, no
external tools. Every row of the table below was **measured** by
`scripts/airgap-container-smoke.sh` (`make smoke-airgap-container`,
`bashy dag smoke-airgap`) running that exact probe under
`--network=none --read-only --cap-drop=ALL`; the table is generated from the
script's output and CI (`.github/workflows/airgap-image.yml`, linux amd64 +
arm64) fails when the doc and the measurement disagree. Sprint 227.

## How it is built, and by what

`bashy self image` fetches the release's static `bashy-scratch-linux-<arch>`
artifact (the `bashy_scratch` profile: static, purego-free — `make
verify-bashy-scratch` proves it) through the same checksum→cache path as every
managed download, writes the five-line Containerfile around it and builds
through `bashy podman`. From a source checkout, `bashy dag build-image` images
the candidate instead.

`bashy podman` is the engine bashy provisions for itself — pinned upstream
releases, sha256 committed in the source, fetched into `$BASHY_BIN_CACHE` and
exec'd as separate processes, never linked or bundled
(`docs/licensing-supply-chain-policy.md`):

| host | what bashy fetches (pinned) | runs the image |
|---|---|---|
| Linux amd64 / arm64 | a complete static podman (podman + conmon + crun/runc + netavark/aardvark-dns, from `mgoltzsche/podman-static`) | natively, rootless or rootful |
| macOS arm64 (amd64: podman 5.8) | the podman machine client + gvproxy + vfkit | inside the podman machine (a Linux VM podman initialises) |
| Windows amd64 / arm64 | the podman machine client (gvproxy + win-sshproxy ship with it) | inside the podman machine on WSL2 — every Windows edition |

A host podman already on `$PATH` is used instead (tier 3); `BASHY_OCI` names
another engine outright.

## What the host needs

Measured by `scripts/self-contained-image-smoke.sh` from the published release
archive on a host with `$PATH` scrubbed of git/go/cc/podman/docker and an empty
`$BASHY_BIN_CACHE`; the provision inventory it prints is this list.

- **Linux:** nothing bashy does not fetch, with three host facts stated: rootless
  podman needs `newuidmap`/`newgidmap` on `$PATH` and a `/etc/subuid` entry for
  the user (shadow-utils — present on every mainstream distro; as root none of
  this applies); the kernel's overlay/user-namespace support (any kernel ≥ 5.11);
  and **Ubuntu 24.04+ with `kernel.apparmor_restrict_unprivileged_userns=1`**:
  a rootless podman from an unpackaged path (ours) is moved into the
  `unprivileged_userns` AppArmor profile when it creates its user namespace,
  and that profile denies exec of `/proc/self/exe` — every rootless command
  dies with "failed to reexec: Permission denied" (audit-log proven on a
  GitHub `ubuntu-24.04` runner; the distro's `/usr/bin/podman` escapes through
  its packaged profile). bashy prints the way out before exec: run as root, or
  once `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0`
  (persist it in `/etc/sysctl.d/`). Some 24.04 kernels do not enforce it
  (rootless build + run passed untouched on a 6.8 server kernel); the sysctl
  makes it moot everywhere.
- **macOS:** nothing but macOS. The first `bashy podman machine init` downloads
  the machine OS image (podman's own fetch, upstream terms) and creates the VM —
  measured 28 s init + 12 s start on an Apple-silicon box, once. Mount paths must
  be under what the machine shares (`/Users`, `/private`, `/var/folders`): `/tmp`
  is a symlink to `/private/tmp` on the host and does not exist inside the VM —
  `-v "$(pwd -P):/work"` if you work under `/tmp`.
- **Windows:** WSL2 enabled (`wsl --install`, any edition). No managed Windows
  podman in this release: a host podman on `$PATH` is what Windows uses today.

## Toolchains for fences: on demand or preloaded

Bash# fences (`~~~py`, `embed python "./bench.py" as bm` …) and bashy's own
rebuild need toolchains bashy provisions. Two ways to get them into the image:

- **On demand (the lean image).** The first fence run fetches what it needs,
  exactly as on any host. The image has no libc and no CA bundle, so bashy
  carries its own TLS roots (exported as `SSL_CERT_FILE` for the toolchains it
  runs), picks uv's static musl build, installs musl's loader (Alpine
  v3.24 `musl-1.2.6-r2`, MIT, sha256-pinned) at
  `/lib/ld-musl-<arch>.so.1`, and lets uv install a musl CPython. Needs network
  on first use and a writable root; a read-only root gets an error naming the
  variant below. Measured: first `embed python` run ~4 s; `bashy go build
  ./cmd/bashy` inside the image rebuilds bashy in ~40 s.
- **Preloaded (a variant).** `bashy self image --with python` (and/or `go`)
  runs that same provisioning at build time into its own layer under
  `/opt/bashy` — outside `/tmp`, so `--read-only --tmpfs /tmp` still sees it.
  Fences then run offline with no first-use download. Measured on arm64: lean
  104 MB, `--with python` 234 MB, `--with go` 356 MB; the `embed python`
  script runs in 0.54 s under `--network=none --read-only`. Rust is not a
  variant yet.

## The table

`in the image` reads: **works** — the probe ran and exited 0; **present** — the
name resolved and ran in-process but its `--version`/`--help` exits non-zero
(the note has the first line); **stated** — it works, with a behaviour you
should know; **not usable offline** — the verb is
present but is an engine, remote-by-design or a bin-managed external (it would
download, and `--network=none` refuses); **absent** — not in the image at all.

Two rows to know about: `echo $HOME` prints the literal `$HOME` on a non-tty
sink (a container's stdout always is) — bashy's Stage 0 output canonicalization
(`internal/agentos/output_reduce.go`), which a terminal bypasses; and islands
that need a toolchain (`~~~py`, `~~~go`, …) are not in this image — an
externals variant is a separate story.

<!-- airgap-table:begin (generated by scripts/airgap-container-smoke.sh — do not edit) -->
| section | name | in the image | note |
|---|---|---|---|
| builtins | `.` | works | bash builtin |
| builtins | `:` | works | bash builtin |
| builtins | `[` | works | bash builtin |
| builtins | `alias` | works | bash builtin |
| builtins | `bg` | works | bash builtin |
| builtins | `bind` | works | bash builtin |
| builtins | `break` | works | bash builtin |
| builtins | `builtin` | works | bash builtin |
| builtins | `caller` | works | bash builtin |
| builtins | `cd` | works | bash builtin |
| builtins | `command` | works | bash builtin |
| builtins | `compgen` | works | bash builtin |
| builtins | `complete` | works | bash builtin |
| builtins | `compopt` | works | bash builtin |
| builtins | `continue` | works | bash builtin |
| builtins | `declare` | works | bash builtin |
| builtins | `dirs` | works | bash builtin |
| builtins | `disown` | works | bash builtin |
| builtins | `echo` | works | bash builtin |
| builtins | `enable` | works | bash builtin |
| builtins | `eval` | works | bash builtin |
| builtins | `exec` | works | bash builtin |
| builtins | `exit` | works | bash builtin |
| builtins | `export` | works | bash builtin |
| builtins | `false` | works | bash builtin |
| builtins | `fc` | works | bash builtin |
| builtins | `fg` | works | bash builtin |
| builtins | `getopts` | works | bash builtin |
| builtins | `hash` | works | bash builtin |
| builtins | `help` | works | bash builtin |
| builtins | `history` | works | bash builtin |
| builtins | `jobs` | works | bash builtin |
| builtins | `kill` | works | bash builtin |
| builtins | `let` | works | bash builtin |
| builtins | `local` | works | bash builtin |
| builtins | `logout` | works | bash builtin |
| builtins | `mapfile` | works | bash builtin |
| builtins | `popd` | works | bash builtin |
| builtins | `printf` | works | bash builtin |
| builtins | `pushd` | works | bash builtin |
| builtins | `pwd` | works | bash builtin |
| builtins | `read` | works | bash builtin |
| builtins | `readarray` | works | bash builtin |
| builtins | `readonly` | works | bash builtin |
| builtins | `return` | works | bash builtin |
| builtins | `set` | works | bash builtin |
| builtins | `shift` | works | bash builtin |
| builtins | `shopt` | works | bash builtin |
| builtins | `source` | works | bash builtin |
| builtins | `suspend` | works | bash builtin |
| builtins | `test` | works | bash builtin |
| builtins | `times` | works | bash builtin |
| builtins | `trap` | works | bash builtin |
| builtins | `true` | works | bash builtin |
| builtins | `type` | works | bash builtin |
| builtins | `typeset` | works | bash builtin |
| builtins | `ulimit` | works | bash builtin |
| builtins | `umask` | works | bash builtin |
| builtins | `unalias` | works | bash builtin |
| builtins | `unset` | works | bash builtin |
| builtins | `wait` | works | bash builtin |
| coreutils | `ar` | not usable offline | bin-managed external: would download - ar: posix provider cache root:  |
| coreutils | `arch` | works | arch (qiangli/coreutils) <ver> |
| coreutils | `ast` | works | Usage: ast <subcommand> [args] |
| coreutils | `at` | works | at (qiangli/coreutils) <ver> |
| coreutils | `atq` | works | atq (qiangli/coreutils) <ver> |
| coreutils | `atrm` | works | atrm (qiangli/coreutils) <ver> |
| coreutils | `awk` | works | awk (qiangli/coreutils) <ver> |
| coreutils | `b2sum` | works | b2sum (qiangli/coreutils) <ver> |
| coreutils | `base32` | works | base32 (qiangli/coreutils) <ver> |
| coreutils | `base64` | works | base64 (qiangli/coreutils) <ver> |
| coreutils | `basename` | works | basename (qiangli/coreutils) <ver> |
| coreutils | `basenc` | works | basenc (qiangli/coreutils) <ver> |
| coreutils | `batch` | works | batch (qiangli/coreutils) <ver> |
| coreutils | `bc` | works | bc (qiangli/coreutils) <ver> |
| coreutils | `browser` | works | browser (qiangli/coreutils) <ver> |
| coreutils | `cal` | works | cal (qiangli/coreutils) <ver> |
| coreutils | `cat` | works | cat (qiangli/coreutils) <ver> |
| coreutils | `chcon` | works | chcon (qiangli/coreutils) <ver> |
| coreutils | `chgrp` | works | chgrp (qiangli/coreutils) <ver> |
| coreutils | `chmod` | works | chmod (qiangli/coreutils) <ver> |
| coreutils | `chown` | works | chown (qiangli/coreutils) <ver> |
| coreutils | `cksum` | works | cksum (qiangli/coreutils) <ver> |
| coreutils | `clip` | works | clip (qiangli/coreutils) <ver> |
| coreutils | `cmp` | works | cmp (qiangli/coreutils) <ver> |
| coreutils | `comm` | works | comm (qiangli/coreutils) <ver> |
| coreutils | `cp` | works | cp (qiangli/coreutils) <ver> |
| coreutils | `crontab` | works | crontab (qiangli/coreutils) <ver> |
| coreutils | `csplit` | works | csplit (qiangli/coreutils) <ver> |
| coreutils | `ctags` | not usable offline | bin-managed external: would download - ctags: posix provider cache roo |
| coreutils | `cut` | works | cut (qiangli/coreutils) <ver> |
| coreutils | `cygpath` | works | cygpath (qiangli/coreutils) <ver> |
| coreutils | `date` | works | date (qiangli/coreutils) <ver> |
| coreutils | `dd` | works | dd (qiangli/coreutils) <ver> |
| coreutils | `df` | works | df (qiangli/coreutils) <ver> |
| coreutils | `diff` | works | diff (qiangli/coreutils) <ver> |
| coreutils | `dir` | works | dir (qiangli/coreutils) <ver> |
| coreutils | `dircolors` | works | dircolors (qiangli/coreutils) <ver> |
| coreutils | `dirname` | works | dirname (qiangli/coreutils) <ver> |
| coreutils | `du` | works | du (qiangli/coreutils) <ver> |
| coreutils | `duration` | works | duration (qiangli/coreutils) <ver> |
| coreutils | `ed` | works | ed (qiangli/coreutils) <ver> |
| coreutils | `env` | works | env (qiangli/coreutils) <ver> |
| coreutils | `ex` | not usable offline | bin-managed external: would download - ex: posix provider cache root:  |
| coreutils | `expand` | works | expand (qiangli/coreutils) <ver> |
| coreutils | `expr` | works | expr (qiangli/coreutils) <ver> |
| coreutils | `factor` | works | factor (qiangli/coreutils) <ver> |
| coreutils | `fetch` | works | fetch (qiangli/coreutils) <ver> |
| coreutils | `file` | works | file (qiangli/coreutils) <ver> |
| coreutils | `find` | works | find (qiangli/coreutils) <ver> |
| coreutils | `fmt` | works | fmt (qiangli/coreutils) <ver> |
| coreutils | `fold` | works | fold (qiangli/coreutils) <ver> |
| coreutils | `foreman` | works | foreman <ver> |
| coreutils | `getconf` | works | getconf (qiangli/coreutils) <ver> |
| coreutils | `graph` | works | Usage: graph <subcommand> [args] |
| coreutils | `grep` | works | grep (qiangli/coreutils) <ver> |
| coreutils | `groups` | works | groups (qiangli/coreutils) <ver> |
| coreutils | `gunzip` | works | gunzip (qiangli/coreutils) <ver> |
| coreutils | `gzip` | works | gzip (qiangli/coreutils) <ver> |
| coreutils | `head` | works | head (qiangli/coreutils) <ver> |
| coreutils | `hexdump` | works | hexdump (qiangli/coreutils) <ver> |
| coreutils | `hostid` | works | hostid (qiangli/coreutils) <ver> |
| coreutils | `hostname` | works | hostname (qiangli/coreutils) <ver> |
| coreutils | `iconv` | works | iconv (qiangli/coreutils) <ver> |
| coreutils | `id` | works | id (qiangli/coreutils) <ver> |
| coreutils | `install` | works | install (qiangli/coreutils) <ver> |
| coreutils | `join` | works | join (qiangli/coreutils) <ver> |
| coreutils | `jq` | works | jq (qiangli/coreutils) <ver> |
| coreutils | `link` | works | link (qiangli/coreutils) <ver> |
| coreutils | `ln` | works | ln (qiangli/coreutils) <ver> |
| coreutils | `locale` | works | locale (qiangli/coreutils) <ver> |
| coreutils | `localedef` | not usable offline | bin-managed external: would download - localedef: posix provider cache |
| coreutils | `logger` | works | logger (qiangli/coreutils) <ver> |
| coreutils | `logname` | works | logname (qiangli/coreutils) <ver> |
| coreutils | `lp` | not usable offline | bin-managed external: would download - lp: posix provider cache root:  |
| coreutils | `ls` | works | ls (qiangli/coreutils) <ver> |
| coreutils | `m4` | not usable offline | bin-managed external: would download - m4: posix provider cache root:  |
| coreutils | `mail` | works | mail (qiangli/coreutils) <ver> |
| coreutils | `mailx` | works | mailx (qiangli/coreutils) <ver> |
| coreutils | `make` | works | make (qiangli/coreutils) <ver> |
| coreutils | `man` | present | runs in-process; --version/--help rc=2: man: unknown option --help |
| coreutils | `md5sum` | works | md5sum (qiangli/coreutils) <ver> |
| coreutils | `mesg` | works | mesg (qiangli/coreutils) <ver> |
| coreutils | `mkdir` | works | mkdir (qiangli/coreutils) <ver> |
| coreutils | `mkfifo` | works | mkfifo (qiangli/coreutils) <ver> |
| coreutils | `mknod` | works | mknod (qiangli/coreutils) <ver> |
| coreutils | `mktemp` | works | mktemp (qiangli/coreutils) <ver> |
| coreutils | `more` | works | more (qiangli/coreutils) <ver> |
| coreutils | `mv` | works | mv (qiangli/coreutils) <ver> |
| coreutils | `ncal` | works | cal (qiangli/coreutils) <ver> |
| coreutils | `newgrp` | works | bash builtin |
| coreutils | `nice` | works | nice (qiangli/coreutils) <ver> |
| coreutils | `nl` | works | nl (qiangli/coreutils) <ver> |
| coreutils | `nm` | not usable offline | bin-managed external: would download - nm: posix provider cache root:  |
| coreutils | `nohup` | works | bash builtin |
| coreutils | `nproc` | works | nproc (qiangli/coreutils) <ver> |
| coreutils | `ntp` | works | ntp (qiangli/coreutils) <ver> |
| coreutils | `numfmt` | works | numfmt (qiangli/coreutils) <ver> |
| coreutils | `od` | works | od (qiangli/coreutils) <ver> |
| coreutils | `paste` | works | paste (qiangli/coreutils) <ver> |
| coreutils | `patch` | works | patch (qiangli/coreutils) <ver> |
| coreutils | `pathchk` | works | pathchk (qiangli/coreutils) <ver> |
| coreutils | `pax` | works | pax (qiangli/coreutils) <ver> |
| coreutils | `pinky` | works | pinky (qiangli/coreutils) <ver> |
| coreutils | `posix-providers` | works | posix-providers <subcommand> |
| coreutils | `pr` | works | pr (qiangli/coreutils) <ver> |
| coreutils | `printenv` | works | printenv (qiangli/coreutils) <ver> |
| coreutils | `ps` | works | ps (qiangli/coreutils) <ver> |
| coreutils | `ptx` | works | ptx (qiangli/coreutils) <ver> |
| coreutils | `readlink` | works | readlink (qiangli/coreutils) <ver> |
| coreutils | `realpath` | works | realpath (qiangli/coreutils) <ver> |
| coreutils | `renice` | works | renice (qiangli/coreutils) <ver> |
| coreutils | `rm` | works | rm (qiangli/coreutils) <ver> |
| coreutils | `rmdir` | works | rmdir (qiangli/coreutils) <ver> |
| coreutils | `sed` | works | sed (qiangli/coreutils) <ver> |
| coreutils | `seq` | works | seq (qiangli/coreutils) <ver> |
| coreutils | `sha1sum` | works | sha1sum (qiangli/coreutils) <ver> |
| coreutils | `sha224sum` | works | sha224sum (qiangli/coreutils) <ver> |
| coreutils | `sha256sum` | works | sha256sum (qiangli/coreutils) <ver> |
| coreutils | `sha384sum` | works | sha384sum (qiangli/coreutils) <ver> |
| coreutils | `sha512sum` | works | sha512sum (qiangli/coreutils) <ver> |
| coreutils | `shred` | works | shred (qiangli/coreutils) <ver> |
| coreutils | `shuf` | works | shuf (qiangli/coreutils) <ver> |
| coreutils | `sleep` | works | sleep (qiangli/coreutils) <ver> |
| coreutils | `sntp` | works | sntp (qiangli/coreutils) <ver> |
| coreutils | `sort` | works | sort (qiangli/coreutils) <ver> |
| coreutils | `split` | works | split (qiangli/coreutils) <ver> |
| coreutils | `stat` | works | stat (qiangli/coreutils) <ver> |
| coreutils | `stdbuf` | works | stdbuf (qiangli/coreutils) <ver> |
| coreutils | `strings` | works | strings (qiangli/coreutils) <ver> |
| coreutils | `strip` | not usable offline | bin-managed external: would download - strip: posix provider cache roo |
| coreutils | `stty` | works | stty (qiangli/coreutils) <ver> |
| coreutils | `sum` | works | sum (qiangli/coreutils) <ver> |
| coreutils | `sync` | works | sync (qiangli/coreutils) <ver> |
| coreutils | `tabs` | works | tabs (qiangli/coreutils) <ver> |
| coreutils | `tac` | works | tac (qiangli/coreutils) <ver> |
| coreutils | `tail` | works | tail (qiangli/coreutils) <ver> |
| coreutils | `talk` | works | talk (qiangli/coreutils) <ver> |
| coreutils | `tar` | works | tar (qiangli/coreutils) <ver> |
| coreutils | `tee` | works | tee (qiangli/coreutils) <ver> |
| coreutils | `time` | present | runs in-process; --version/--help rc=2: time: unknown option "--help" |
| coreutils | `timeout` | works | timeout (qiangli/coreutils) <ver> |
| coreutils | `touch` | works | touch (qiangli/coreutils) <ver> |
| coreutils | `tput` | works | tput (qiangli/coreutils) <ver> |
| coreutils | `tr` | works | tr (qiangli/coreutils) <ver> |
| coreutils | `tree` | works | tree (qiangli/coreutils) <ver> |
| coreutils | `truncate` | works | truncate (qiangli/coreutils) <ver> |
| coreutils | `tsort` | works | tsort (qiangli/coreutils) <ver> |
| coreutils | `tty` | works | tty (qiangli/coreutils) <ver> |
| coreutils | `tz` | works | tz (qiangli/coreutils) <ver> |
| coreutils | `uname` | works | uname (qiangli/coreutils) <ver> |
| coreutils | `unexpand` | works | unexpand (qiangli/coreutils) <ver> |
| coreutils | `uniq` | works | uniq (qiangli/coreutils) <ver> |
| coreutils | `unlink` | works | unlink (qiangli/coreutils) <ver> |
| coreutils | `uptime` | works | uptime (qiangli/coreutils) <ver> |
| coreutils | `users` | works | users (qiangli/coreutils) <ver> |
| coreutils | `uudecode` | works | uudecode (qiangli/coreutils) <ver> |
| coreutils | `uuencode` | works | uuencode (qiangli/coreutils) <ver> |
| coreutils | `vdir` | works | vdir (qiangli/coreutils) <ver> |
| coreutils | `vi` | not usable offline | bin-managed external: would download - vi: posix provider cache root:  |
| coreutils | `watch` | works | watch (qiangli/coreutils) <ver> |
| coreutils | `wc` | works | wc (qiangli/coreutils) <ver> |
| coreutils | `which` | works | which (qiangli/coreutils) <ver> |
| coreutils | `who` | works | who (qiangli/coreutils) <ver> |
| coreutils | `whoami` | works | whoami (qiangli/coreutils) <ver> |
| coreutils | `why` | not usable offline | bin-managed external: would download - why: resolve witr: binmgr: fetc |
| coreutils | `write` | works | write (qiangli/coreutils) <ver> |
| coreutils | `wslpath` | works | wslpath (qiangli/coreutils) <ver> |
| coreutils | `xargs` | present | runs in-process; --version/--help rc=2: xargs: unknown option "--help" |
| coreutils | `yes` | works | yes (qiangli/coreutils) <ver> |
| coreutils | `zcat` | works | zcat (qiangli/coreutils) <ver> |
| shell | `--bashsharp examples/quickstart/enums.bsh` | works | output matches enums.expected |
| shell | `--bashsharp examples/quickstart/hello.bsh` | works | output matches hello.expected |
| shell | `--bashsharp examples/quickstart/kwargs.bsh` | works | output matches kwargs.expected |
| shell | `--posix` | works | 10 |
| shell | `--version` | works | bashy, GNU Bash <ver> compatible, version <ver>(1)-bashy-<ver> (<sha>) |
| shell | `-c (arithmetic, printf)` | works | 42 |
| shell | `echo $HOME (non-tty stdout)` | stated | prints $HOME: Stage 0 output canonicalization (output_reduce.go); a tt |
| shell | `network (curl)` | absent | no network tools in the image; --network=none besides |
| verbs | `act` | not usable offline | bin-managed external: would download - Error: act: resolve: binmgr: fe |
| verbs | `act-runner` | not usable offline | bin-managed external: would download - act-runner runs Gitea's act_run |
| verbs | `activity` | works | usage: bashy activity {status\|subscribe\|unsubscribe\|interests\|tail |
| verbs | `agent` | works | Show all live named and ad-hoc work reconciled from sprint leases, wea |
| verbs | `agentic` | works | usage: bashy agentic [--] ACTION [ARG...] |
| verbs | `agents` | works | Show all live named and ad-hoc work reconciled from sprint leases, wea |
| verbs | `app` | works | app serves bashy's Apps  its surfaces in a browser at one address. |
| verbs | `apps` | works | app serves bashy's Apps  its surfaces in a browser at one address. |
| verbs | `ask` | works | ask requests a value from the person at the keyboard, over a channel t |
| verbs | `audit` | works | usage: bashy inspect audit {status\|tail [N]\|verify\|export\|path} |
| verbs | `awd` | works | bash builtin |
| verbs | `bootstrap` | works | bashy self fetches and caches a released bashy binary using the same |
| verbs | `bus` | works | bus is how agents on this host tell each other something changed. |
| verbs | `capability` | works | The routing table behind capability-routed delegation: which agent |
| verbs | `cargo` | not usable offline | engine / remote-by-design (by design): Error: rust: fetch sha256 sidec |
| verbs | `chat` | works | talk to an agent  a live governed session (no instruction) or a one-sh |
| verbs | `check` | works | usage: bashy check [--bashsharp] [--mode bash53\|posix\|bashy] [--json |
| verbs | `claim` | works | claim stops two agents from writing the same project at the same time. |
| verbs | `clang` | not usable offline | engine / remote-by-design (by design): Error: clang: no system clang o |
| verbs | `cmake` | not usable offline | engine / remote-by-design (by design): Error: cmake: fetch checksums:  |
| verbs | `coach` | works | Coach starts a steerable session, watches for a doomed loop, and when  |
| verbs | `command` | works | bash builtin |
| verbs | `commands` | works | usage: commands [COMMAND] [-v] [--json\|--plain\|--agentic\|--all\|--g |
| verbs | `conform` | works | verify runs bashy's formal test batteries. The four suites are a preci |
| verbs | `context` | works | usage: bashy inspect context [--json\|--plain] |
| verbs | `craft` | works | craft is the accumulated body of practical skill on this host. |
| verbs | `curl` | not usable offline | engine / remote-by-design (by design): Error: curl: not found on PATH  |
| verbs | `dag` | works | dag runs targets defined as headings in a markdown file (DAG.md) as a |
| verbs | `define` | works | define answers "what is this word, here?" for any token. |
| verbs | `delegate` | works | delegate hands a task to an agent and returns its result. |
| verbs | `dhnt` | works | usage: bashy dhnt COMMAND |
| verbs | `dks` | not usable offline | engine / remote-by-design (by design): bashy dks: not available in thi |
| verbs | `docker` | not usable offline | engine / remote-by-design (by design): bashy podman: fetching podman < |
| verbs | `doctl` | not usable offline | bin-managed external: would download - Error: doctl: binmgr: fetch rel |
| verbs | `doctor` | works | usage: doctor [--json] |
| verbs | `foreman` | works | foreman <ver> |
| verbs | `full` | present | runs in-process; --version/--help rc=2: bashy full: line 1: command: - |
| verbs | `gate` | works | gate runs the project's gate  the command that decides pass/fail  and |
| verbs | `gcloud` | not usable offline | engine / remote-by-design (by design): Error: gcloud: gcloud not found |
| verbs | `gh` | not usable offline | bin-managed external: would download - Error: gh: resolve: binmgr: fet |
| verbs | `git` | not usable offline | bin-managed external: would download - Error: gitscm: no system git on |
| verbs | `git-scm` | not usable offline | bin-managed external: would download - Error: gitscm: no system git on |
| verbs | `go` | not usable offline | engine / remote-by-design (by design): Error: gotoolchain: fetch relea |
| verbs | `handoff` | works | handoff captures everything a successor needs and passes the work on. |
| verbs | `helm` | not usable offline | engine / remote-by-design (by design): Error: helm: resolve latest ver |
| verbs | `herald` | works | herald reaches agents that are not on this host. |
| verbs | `inbox` | works | inbox is one read-through view over the existing message board, Meet |
| verbs | `inspect` | works | usage: bashy inspect [ASPECT] [--json\|--plain] |
| verbs | `invoke` | works | talk to an agent  a live governed session (no instruction) or a one-sh |
| verbs | `issue` | works | todo tracks work as simple items (todo -> doing -> done, or blocked).  |
| verbs | `judge` | works | judge reads a piece of work and renders an opinion on it. |
| verbs | `kb` | works | kb is agent memory as a wiki of small markdown pages (YAML frontmatter |
| verbs | `kopia` | not usable offline | bin-managed external: would download - kopia runs the Kopia repository |
| verbs | `kubectl` | not usable offline | engine / remote-by-design (by design): Error: kubectl: resolve stable  |
| verbs | `lexicon` | works | lexicon is the project's jargon, projected from the registries that al |
| verbs | `login` | not usable offline | engine / remote-by-design (by design): tessaro: this machine isn't con |
| verbs | `loom` | not usable offline | bin-managed external: would download - loom runs Gitea  downloaded, sh |
| verbs | `mb` | works | mb is the host's message board  one shared, append-only board every ag |
| verbs | `meet` | works | Run a turn-taking planning meeting across agentic CLIs and a human. |
| verbs | `messages` | works | mb is the host's message board  one shared, append-only board every ag |
| verbs | `mirror` | works | mirror keeps a destination in sync with a source directory: an initial |
| verbs | `mise` | not usable offline | bin-managed external: would download - Error: mise: resolve: binmgr: f |
| verbs | `model` | works | List inference backends. |
| verbs | `models` | works | List inference backends. |
| verbs | `node` | not usable offline | engine / remote-by-design (by design): Error: Get "https://nodejs.org/ |
| verbs | `notify` | works | notify sends one subject-only notification through the existing bus. |
| verbs | `npm` | not usable offline | engine / remote-by-design (by design): Error: Get "https://nodejs.org/ |
| verbs | `npx` | not usable offline | engine / remote-by-design (by design): Error: Get "https://nodejs.org/ |
| verbs | `oci` | not usable offline | engine / remote-by-design (by design): bashy podman: fetching podman < |
| verbs | `ollama` | not usable offline | engine / remote-by-design (by design): bashy ollama: no ollama found o |
| verbs | `otel` | works | Query OTEL telemetry with bounded agent-readable summaries |
| verbs | `out` | works | out reprints the full, un-reduced bytes that an elision marker spilled |
| verbs | `pair` | works | Run work through two agents in different roles, optionally followed by |
| verbs | `peer` | works | sphere is dhnt execution tier 4: multi-node, PEER-DIRECT pooled infere |
| verbs | `people` | works | Human principals  who the names in prose refer to |
| verbs | `person` | works | Human principals  who the names in prose refer to |
| verbs | `ping` | works | ping is the front door to this host's message board, and to the classi |
| verbs | `pip` | not usable offline | engine / remote-by-design (by design): Error: python/uv: fetch sha256  |
| verbs | `pnpm` | not usable offline | engine / remote-by-design (by design): Error: Get "https://nodejs.org/ |
| verbs | `podman` | not usable offline | engine / remote-by-design (by design): bashy podman: fetching podman < |
| verbs | `posix-gate` | works | posix-gate <subcommand> |
| verbs | `python` | not usable offline | engine / remote-by-design (by design): Error: python/uv: fetch sha256  |
| verbs | `rclone` | not usable offline | bin-managed external: would download - Error: rclone: resolve: binmgr: |
| verbs | `release` | works | bashy release turns a .goreleaser.yaml into named, checksummed artifac |
| verbs | `resource` | works | Report host resource utilization |
| verbs | `resources` | works | Report host resource utilization |
| verbs | `resume` | works | resume reads a handoff record and continues the work. |
| verbs | `rg` | not usable offline | bin-managed external: would download - Error: rg: binmgr: fetch releas |
| verbs | `run` | works | usage: bashy run [--capture] [--check] [--target NAME] -- command [arg |
| verbs | `rust` | not usable offline | engine / remote-by-design (by design): Error: rust: fetch sha256 sidec |
| verbs | `rustc` | not usable offline | engine / remote-by-design (by design): Error: rust: fetch sha256 sidec |
| verbs | `rustup` | not usable offline | engine / remote-by-design (by design): Error: rust: fetch sha256 sidec |
| verbs | `sandbox` | not usable offline | engine / remote-by-design (by design): bashy podman: fetching podman < |
| verbs | `schedule` | works | Modern cron: run commands on a cron/interval/at schedule, with an agen |
| verbs | `sdlc` | works | bashy sdlc is a local-first SDLC coordinator. It accepts an issue/requ |
| verbs | `search` | works | Search the web through a provider ladder (auto by available key, or -- |
| verbs | `seaweedfs` | not usable offline | bin-managed external: would download - seaweedfs runs SeaweedFS  downl |
| verbs | `secret` | works | secrets fetches your API keys/tokens from cloudbox's encrypted vault |
| verbs | `secrets` | works | secrets fetches your API keys/tokens from cloudbox's encrypted vault |
| verbs | `self` | works | bashy self fetches and caches a released bashy binary using the same |
| verbs | `skill` | works | skills lists, inspects, and probes the tier-2 workspace skills availab |
| verbs | `skills` | works | skills lists, inspects, and probes the tier-2 workspace skills availab |
| verbs | `sota` | works | sota grounds a synthesis agent in REAL web-search results (`bashy sear |
| verbs | `sphere` | works | sphere is dhnt execution tier 4: multi-node, PEER-DIRECT pooled infere |
| verbs | `sprint` | works | sprint is the conductor's PLAN/HANDOFF layer  the cross-repo kanban |
| verbs | `supervise` | works | Drive worker agents against a goal decomposed into tasks, IN the curre |
| verbs | `supervisord` | works | usage: bashy supervisord [flags] DAG.md TARGET |
| verbs | `tessaro` | works | Tessaro is the front door to your dhnt mesh  pooled LLMs + durable age |
| verbs | `todo` | works | todo tracks work as simple items (todo -> doing -> done, or blocked).  |
| verbs | `tofu` | not usable offline | bin-managed external: would download - Error: tofu: binmgr: fetch rele |
| verbs | `tokens` | works | tokens (qiangli/coreutils) <ver> |
| verbs | `tool` | works | List agentic CLI tools. |
| verbs | `tools` | works | List agentic CLI tools. |
| verbs | `transpile` | present | runs in-process; --version/--help rc=2: transpile: unknown flag: --hel |
| verbs | `upgrade` | works | bashy self fetches and caches a released bashy binary using the same |
| verbs | `uv` | not usable offline | engine / remote-by-design (by design): Error: python/uv: fetch sha256  |
| verbs | `verify` | works | verify runs bashy's formal test batteries. The four suites are a preci |
| verbs | `weave` | works | weave is the per-repo EXECUTION engine: a local, filesystem-based |
| verbs | `web` | works | web inspection helpers |
| verbs | `whois` | works | Resolve a name to a principal and say how to reach it. |
| verbs | `yarn` | not usable offline | engine / remote-by-design (by design): Error: Get "https://nodejs.org/ |
| verbs | `zig` | present | runs in-process; --version/--help rc=1: Error: Get "https://ziglang.or |
| verbs | `zot` | not usable offline | bin-managed external: would download - zot runs the Zot OCI registry   |
<!-- airgap-table:end -->
