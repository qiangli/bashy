# Bashy Script Check and Coreutils Coverage Plan

Date: 2026-07-01

Purpose: add a ShellCheck-like static analyzer for bashy scripts, but focused on
execution predictability: syntax validity, Bash/POSIX mode compatibility,
bashy-only dependency closure, and exact reporting of commands that would fall
through to the system or to a managed GNU coreutils fallback container.

## References

- ShellCheck is the existing reference point for shell static analysis: warnings
  and suggestions for `bash`/`sh` scripts, with goals spanning syntax,
  intermediate semantic mistakes, and subtle portability pitfalls.
  <https://github.com/koalaman/shellcheck>
- GNU coreutils upstream states that coreutils is the union of fileutils,
  sh-utils, and textutils; the current upstream README lists the buildable
  program inventory and notes `v9.11` as the latest GitHub release on
  2026-04-20. <https://github.com/coreutils/coreutils>

## Proposed User Surface

Primary checker:

```text
bashy check [--mode bash53|posix|bashy] [--json] [--strict-system] SCRIPT...
```

Mode meanings:

- `bash53`: parse and validate against the Bash 5.3-compatible shell surface.
- `posix`: parse in POSIX mode and reject bashisms where the parser/interpreter
  can identify them.
- `bashy`: Bash 5.3 plus AgentOS extensions, coreutils userland, and front-door
  verbs.

Dependency policy flags:

- `--strict-system`: fail any command that would execute through host `PATH`.
- `--allow-system NAME[,NAME...]`: permit specific host commands.
- `--allow-container`: allow fallback to managed GNU coreutils container.
- `--no-source`: do not follow `source`/`.` includes; report them as unknown
  unless already loaded.
- `--source-root DIR`: resolve relative sourced files from a known project root.
- `--max-depth N`: recursion guard for sourced scripts and statically resolved
  script executions.

Managed GNU coreutils fallback:

```text
bashy coreutils doctor
bashy coreutils ensure [--image bashy/gnu-coreutils:<version>|--alpine]
bashy coreutils run -- COMMAND [ARGS...]
```

Use `ensure`, not `install`, because bashy should provision a managed fallback
without mutating the host. The first implementation can build/cache a small OCI
image through `bashy podman`:

```Dockerfile
FROM alpine:3.22
RUN apk add --no-cache coreutils
ENTRYPOINT ["/usr/bin/env"]
```

If exact GNU behavior matters more than image size, add a Debian/Ubuntu variant
as the default profile and keep Alpine as `--alpine`. Alpine's `coreutils`
package is useful and small, but musl and package layout can still differ from
the GNU/Linux behavior users expect on Debian/Fedora.

## Current Bashy Command Surface

Live inventory from `./bin/bashy commands --json`:

- Bash builtins: `.`, `:`, `[`, `alias`, `bg`, `bind`, `break`, `builtin`,
  `caller`, `cd`, `command`, `compgen`, `complete`, `compopt`, `continue`,
  `declare`, `dirs`, `disown`, `echo`, `enable`, `eval`, `exec`, `exit`,
  `export`, `false`, `fc`, `fg`, `getopts`, `hash`, `help`, `history`, `jobs`,
  `kill`, `let`, `local`, `logout`, `mapfile`, `popd`, `printf`, `pushd`,
  `pwd`, `read`, `readarray`, `readonly`, `return`, `set`, `shift`, `shopt`,
  `source`, `suspend`, `test`, `times`, `trap`, `true`, `type`, `typeset`,
  `ulimit`, `umask`, `unalias`, `unset`, `wait`.
- Bashy in-process userland: `awk`, `base32`, `base64`, `basename`, `cat`,
  `chgrp`, `chmod`, `chown`, `clip`, `cmp`, `comm`, `cp`, `cut`, `date`, `df`,
  `diff`, `dirname`, `du`, `echo`, `env`, `false`, `fetch`, `find`, `grep`,
  `gunzip`, `gzip`, `head`, `hexdump`, `hostname`, `id`, `join`, `jq`, `link`,
  `ln`, `ls`, `md5sum`, `mkdir`, `mktemp`, `mv`, `paste`, `printenv`, `pwd`,
  `readlink`, `realpath`, `rm`, `rmdir`, `sed`, `seq`, `sha1sum`, `sha224sum`,
  `sha256sum`, `sha384sum`, `sha512sum`, `shuf`, `sleep`, `sort`, `split`,
  `stat`, `strings`, `sync`, `tac`, `tail`, `tar`, `tee`, `time`, `tokens`,
  `touch`, `tr`, `tree`, `true`, `truncate`, `tsort`, `tty`, `uname`, `uniq`,
  `unlink`, `uptime`, `wc`, `which`, `whoami`, `xargs`, `yc`, `yes`, `zcat`.
- Bashy front-door verbs: `act`, `commands`, `dag`, `docker`, `doctor`, `gh`,
  `git`, `kopia`, `loom`, `mirror`, `ollama`, `podman`, `rclone`, `run`,
  `schedule`, `seaweedfs`, `secrets`, `self`, `skills`, `sprint`, `weave`,
  `zot`.

## GNU Coreutils Command-Level Gap

GNU upstream buildable program inventory:

```text
arch b2sum base32 base64 basename basenc cat chcon chgrp chmod chown chroot
cksum comm coreutils cp csplit cut date dd df dir dircolors dirname du echo env
expand expr factor false fmt fold groups head hostid hostname id install join
kill link ln logname ls md5sum mkdir mkfifo mknod mktemp mv nice nl nohup nproc
numfmt od paste pathchk pinky pr printenv printf ptx pwd readlink realpath rm
rmdir runcon seq sha1sum sha224sum sha256sum sha384sum sha512sum shred shuf
sleep sort split stat stdbuf stty sum sync tac tail tee test timeout touch tr
true truncate tsort tty uname unexpand uniq unlink uptime users vdir wc who
whoami yes
```

Missing from bashy's in-process coreutils registry:

```text
arch b2sum basenc chcon chroot cksum coreutils csplit dd dir dircolors expand
expr factor fmt fold groups hostid install kill logname mkfifo mknod nice nl
nohup nproc numfmt od pathchk pinky pr printf ptx runcon shred stdbuf stty sum
test timeout unexpand users vdir who
```

Important nuance for the script checker:

- `kill`, `printf`, and `test` are covered as Bash builtins even though they are
  absent from the in-process coreutils registry.
- `dir` and `vdir` can be compatibility aliases over `ls` behavior, but GNU
  treats them as separate commands.
- `arch` can be an alias/subset of `uname -m`, but should still be present if
  the goal is command-name closure.
- `coreutils` is the GNU multicall frontend; bashy does not currently expose an
  equivalent in-process command.

Bashy has non-GNU extras that are useful but should not count toward GNU
coreutils parity:

```text
awk clip cmp diff fetch find grep gunzip gzip hexdump jq sed strings tar time
tokens tree which xargs yc zcat
```

## Option and Argument Coverage Gap

Current state: command names are discoverable, but option/argument capability is
not. The `coreutils/tool` package has a strict GNU-style flag parser that fails
unknown flags loudly, but the registry exposes only `Name`, `Synopsis`, `Usage`,
and `Run`.

Required next step: add a structured capability manifest to coreutils:

```go
type Capability struct {
    Name        string
    Kind        string // builtin | coreutil | verb | external
    GNUName     string
    GNUVersion  string
    Flags       []FlagCapability
    Operands    OperandSpec
    Unsupported []UnsupportedCase
    Notes       []string
}
```

Each command must publish:

- supported short flags, long flags, and flag argument requirements;
- unsupported GNU flags with explicit reason;
- operand shape: min/max/count, special forms, stdin behavior;
- dangerous behavior notes (`rm -rf`, redirection, `find -exec`, `xargs`);
- platform notes where Windows/macOS/Linux differ.

The checker must never infer full GNU compatibility from command presence.
Until a command has a capability manifest, report it as:

```text
status: "implemented-command-unknown-option-coverage"
```

## Checker Architecture

Phase 1: parse and syntax validation.

- Use `mvdan.cc/sh/v3/syntax` from the bashy/sh fork.
- Parse in Bash 5.3 mode and POSIX mode where supported.
- Report parse errors with Bash-compatible locations.
- Track source span for every command word and redirection.

Phase 2: static command extraction.

- Walk the AST and collect simple commands, pipelines, command substitutions,
  process substitutions, functions, aliases where statically resolvable, and
  `source`/`.` includes.
- Recursively analyze scripts reached from the entry scripts:
  - `source file` and `. file`;
  - statically resolved shell script executions such as `./scripts/build.sh`,
    `bash scripts/build.sh`, `bashy scripts/build.sh`, and `sh scripts/build.sh`;
  - executable files with a shell shebang when the path is static and readable.
- Maintain shell state for function definitions and local aliases within the
  same file.
- Track an analyzed-file set by canonical path to avoid loops, and stop at
  `--max-depth` with a warning instead of recursing forever.
- Preserve the caller/callee edge so reports can explain where a command came
  from: entry script -> sourced file -> nested script.
- Mark dynamically computed command names separately:

```text
"$(choose_tool)" "$arg"     -> dynamic-command
"$cmd" --flag              -> dynamic-command
command "$tool"            -> dynamic-command
```

Phase 3: resolution model.

Resolve every command in this order:

1. shell functions/aliases defined by the script;
2. Bash builtins;
3. bashy in-process coreutils/userland;
4. bashy front-door verbs and bare-name shims;
5. managed externals (`gh`, `act`, `rclone`, etc.);
6. managed GNU coreutils container, if `--allow-container`;
7. host `PATH`, unless `--strict-system`.

Phase 4: flag/operand validation.

- If the command has a capability manifest, parse its static arguments against
  that manifest.
- If arguments include expansions, globbing, arrays, or command substitutions,
  validate only the static prefix and mark dynamic segments as unknown.
- If no manifest exists, report command-level coverage but unknown option-level
  coverage.

Phase 5: execution expectation report.

Plain output should read like a dry-run:

```text
script.sh:12:3 ok      cp -a src dst        bashy coreutils/cp
script.sh:15:3 warn    dd if=a of=b bs=1M   missing in bashy; would use host PATH
script.sh:20:3 error   timeout 5 cmd        missing in bashy; strict-system forbids host PATH
script.sh:31:3 info    $tool --version      dynamic command name; cannot prove closure
```

At the end, always print a closure inventory:

```text
command inventory:
  bashy native:
    cp        coreutils/cp
    echo      builtin
    git       bashy verb
  system PATH:
    python3   /usr/bin/python3
    perl      /usr/bin/perl
  container fallback:
    timeout   gnu-coreutils:9.11
  not found:
    deployctl
```

The `system PATH` section must include the full resolved path for each command
because this is the evidence that the script is not self-contained. The
`not found` section is the set of commands that would fail at runtime with
`command not found` under the selected policy and environment.

JSON output should be stable for agents:

```json
{
  "schema_version": "bashy-check-v1",
  "mode": "bashy",
  "summary": {
    "commands": 12,
    "files_analyzed": 3,
    "bashy_native": 8,
    "container": 1,
    "system": 2,
    "not_found": 1,
    "dynamic": 1,
    "errors": 1,
    "warnings": 2
  },
  "files": [
    {"path": "script.sh", "role": "entry"},
    {"path": "lib/common.sh", "role": "source", "from": "script.sh"}
  ],
  "inventory": {
    "bashy_native": [
      {"name": "cp", "kind": "coreutil"},
      {"name": "echo", "kind": "builtin"}
    ],
    "system": [
      {"name": "python3", "path": "/usr/bin/python3"}
    ],
    "container": [
      {"name": "timeout", "image": "gnu-coreutils:9.11"}
    ],
    "not_found": [
      {"name": "deployctl"}
    ],
    "dynamic": [
      {"span": "script.sh:31:3", "text": "$tool"}
    ]
  },
  "diagnostics": []
}
```

## Diagnostic Codes

Initial code family:

- `BASHY0001`: syntax error.
- `BASHY0100`: Bashism rejected in POSIX mode.
- `BASHY0200`: command resolves to bashy builtin.
- `BASHY0201`: command resolves to bashy in-process userland.
- `BASHY0202`: command resolves to bashy front-door verb.
- `BASHY0300`: command not covered by bashy, would use host PATH.
- `BASHY0301`: command not covered by bashy, strict-system failure.
- `BASHY0302`: command not covered by bashy, container fallback available.
- `BASHY0400`: option unsupported by bashy implementation.
- `BASHY0401`: option coverage unknown because no manifest exists.
- `BASHY0500`: dynamic command name cannot be proven.
- `BASHY0600`: sourced file not found or not analyzed.
- `BASHY0601`: recursive script analysis exceeded `--max-depth`.
- `BASHY0700`: command found on system PATH; report full path.
- `BASHY0701`: command not found in bashy, container fallback, or system PATH.

## Implementation Sprint Plan

### Sprint 1: Static Foundation

- Add `bashy check` front-door command.
- Parse files and report Bash/POSIX syntax errors.
- Walk AST and collect command invocations with source locations.
- Recursively analyze sourced scripts and statically resolved shell-script
  executions.
- Resolve against live builtins/coreutils/verbs from existing registries.
- Emit final command inventory grouped by bashy-native, container fallback,
  system PATH with full paths, dynamic, and not found.
- Emit plain and JSON reports.
- Add fixtures for valid bash, invalid syntax, POSIX mode rejection, dynamic
  command names, functions, aliases, and sourced files.

Exit criteria:

- `bashy check --json script.sh` reports stable `bashy-check-v1`.
- `--strict-system` fails scripts with unknown external commands.

### Sprint 2: Coreutils Capability Manifest

- Extend `coreutils/tool.Tool` with optional capability metadata.
- Add manifests for the highest-use commands first: `cat`, `cp`, `mv`, `rm`,
  `mkdir`, `ls`, `grep`, `sed`, `find`, `xargs`, `tar`, `sort`, `head`,
  `tail`, `wc`, `tr`, `cut`, `date`, `env`.
- Add a generated inventory command:

```text
bashy commands --capabilities --json
```

- Add `bashy check` option/operand validation against the manifest.

Exit criteria:

- The checker reports unsupported flags before runtime.
- Unknown command vs unknown option vs dynamic option are distinct outcomes.

### Sprint 3: GNU Coreutils Gap Database

- Vendor or generate a GNU coreutils command/flag inventory from upstream
  `--help` output for a pinned GNU coreutils release.
- Generate command-level and flag-level diff reports:

```text
bashy coreutils coverage --gnu-version 9.11 --json
```

- Store generated snapshots under `docs/generated/` or `internal/coverage/testdata/`.

Exit criteria:

- Complete command-level gap is machine-generated.
- Option-level gaps are machine-generated for every command that has a bashy
  implementation.

### Sprint 4: Managed GNU Coreutils Fallback

- Add `bashy coreutils ensure`.
- Build/cache a small OCI image with GNU coreutils.
- Add `bashy coreutils run -- COMMAND ARGS...`.
- Teach `bashy check --allow-container` to classify missing commands as
  container-resolvable.

Exit criteria:

- A script with `dd`, `timeout`, or `numfmt` can be reported as:
  "not bashy-native; available through managed GNU coreutils container".
- No host mutation is required.

### Sprint 5: Runtime Integration

- Add optional runtime policy:

```text
BASHY_SYSTEM_POLICY=strict|warn|allow
BASHY_COREUTILS_FALLBACK=off|container
```

- Wire the same resolver into dry-run/advisor so runtime and static check agree.
- Add a `bashy run --policy strict` mode that fails before host PATH fallback.

Exit criteria:

- `bashy check` predicts what `bashy run --dry-run` reports for static command
  invocations.

## Future Coreutils Conformance Roadmap

Goal: make bashy's in-process coreutils command set approach GNU coreutils
compatibility without compromising the no-host-dependency bootstrap goal.

Order of work:

1. Add missing easy aliases/wrappers: `arch`, `dir`, `vdir`, maybe `groups`.
2. Add common standalone commands with limited platform risk: `b2sum`, `basenc`,
   `cksum`, `expand`, `expr`, `factor`, `fmt`, `fold`, `nl`, `nproc`, `od`,
   `pathchk`, `sum`, `timeout`, `unexpand`, `users`, `who`.
3. Add filesystem/special-device commands behind platform-specific support:
   `chroot`, `dd`, `install`, `mkfifo`, `mknod`, `shred`, `stdbuf`, `stty`.
4. Add SELinux/security-context commands as explicit unsupported/platform-gated
   commands first: `chcon`, `runcon`.
5. Add GNU test fixtures per command and flag. Every unsupported flag should
   have a contract test proving it fails loudly.

Success metrics:

- command coverage: implemented GNU command count / GNU command count;
- option coverage: supported GNU option count / GNU option count per command;
- script closure: percentage of real bashy project scripts that pass
  `bashy check --strict-system`;
- fallback reduction: number of script commands requiring container/host PATH.
