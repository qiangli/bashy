# Sprint 355 base/core Profile D candidate

Story #402 (`c6d7c705b835`) tracks the build profile. The `bashy_cert_base`
tag selects the existing one-file base/core entry point while retaining the
same Bash shell, Bash# Go-source and Go fence support, Go Coreutils applets,
and registered-command CRUD. It excludes the optional AgentOS, ycode, Genie,
Filebrowser, and database import graph before Go package initialization.
`bashy_core` remains a diagnostic-only tag and release eligibility rejects it.

The Profile D harness must build with `bashy_cert,bashy_cert_base`, static
Linux CGO, and `-w` (never `-s`). Its pre-TCC gate checks both tags, CGO,
ELF64 x86_64 ET_EXEC, no interpreter, and the retained `runtime.fwdSig`
symbol. The Bashy release gate also rejects a base artifact lacking Linux
CGO certification settings. The staged executable is one physical Bashy
file; shell and applet names are links to it. The existing required-command,
provider, inode, and POSIX-mode checks still apply.

The base entry point accepts `--dry-run` and `--dryrun` as parse-only
validation. In POSIX mode, that route also selects the strict POSIX grammar;
the staging canary checks that a valid command has no side effects. This
keeps the safety check available without importing optional AgentOS startup.

The base also exposes Coreutils' `bashy schedule` daemon command. Profile D
starts it before TCC so POSIX `at`, `batch`, and `crontab` jobs can run. The
base dispatch uses the same schedule package as full Bashy and does not load
AgentOS.

This profile is a candidate for the declared POSIX base and Bash# runtime.
It does not yet supply the optional AgentOS front door or a language/toolchain
extension registry. Those are separately tracked in Sprint 355 and are not
claims of this build. A focused licensed `awk` replay must establish whether
the measured startup reduction clears the unchanged 600-second set cap before
a full 117-set Profile D diagnostic. Neither a launch benchmark nor the
previous nonshipping `bashy_core` probe is certification evidence.
