# Shell mode selection

Bashy uses the name and startup options of the shell invocation to choose one
of two compatibility modes. This is part of the public shell contract, not a
test-harness convention.

## Modes

| Invocation | Initial mode |
| --- | --- |
| `bash`, `bashy`, or another name | GNU Bash 5.3-compatible mode |
| `sh`, `/path/to/sh`, or login `-sh` | POSIX `sh` mode |

Plain `bash`/`bashy` keeps GNU Bash syntax and behavior enabled. Invoking the
same executable with basename `sh` selects the historical POSIX shell
interface, as GNU Bash does.

The following startup requests also enable Bash-compatible POSIX mode:

- `--posix`;
- `-o posix`;
- `SHELLOPTS` containing `posix`;
- the presence of `POSIXLY_CORRECT`, even when its value is empty; or
- the presence of the legacy `POSIX_PEDANTIC`, even when its value is empty.

`POSIX_PEDANTIC` synthesizes `POSIXLY_CORRECT=y` when
`POSIXLY_CORRECT` was not inherited. An inherited `POSIXLY_CORRECT` value is
preserved verbatim, including an empty value. These are GNU Bash 5.3 startup
semantics.

## Precedence

For command-line-only requests, the last `-o posix` or `+o posix` wins. The
startup conditions `sh`, `SHELLOPTS=posix`, `POSIXLY_CORRECT`, and
`POSIX_PEDANTIC` force POSIX mode after command-line set-option processing, as
GNU Bash 5.3 does. Consequently, `+o posix` does not cancel one of those
startup conditions.

Invoked-as-`sh` mode additionally enables the strict semantics associated with
choosing the POSIX shell interface. Explicit `bash --posix` remains the GNU
Bash-compatible POSIX mode; it is not silently promoted to every stricter
`argv[0]=sh` rule. This distinction is tested because GNU Bash itself exposes
it in several error and builtin behaviors.

## One decision across every execution path

The effective startup mode is shared by:

- non-interactive and interactive parsing;
- interpreter set options and builtin behavior;
- prompt and history behavior;
- job-carrier selection;
- AgentOS extension gating; and
- persistent/warm session parsing and execution.

This prevents a command from being parsed in one mode and executed in another,
or from behaving differently only because it entered through a warm session.
AgentOS extensions that are documented as inert in POSIX mode use this same
decision.

## Certification wiring

The VSC/PCTS campaign installs the shell-under-test as `/vsc/sut/sh` and sets
that exact path as `TET_EXEC_TOOL`. The native preflight executes the wired
tool and refuses licensed dispatch unless `POSIXLY_CORRECT` is present and
`set -o` reports `posix on`. A version string or a POSIX-capable binary alone
is insufficient.

See also [POSIX-mode behaviors](posix-mode-behaviors.md) for the behavioral
parity checklist and [the conformance statement](conformance-statement.md) for
the scope and evidence boundaries.
