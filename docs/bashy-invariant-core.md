# Bashy invariant command classes

`bashy commands --view class` groups commands by the release boundary. Its JSON
form carries `class` on every command. `bashy commands NAME` reports the same
class for one command.

| Class | Meaning |
|---|---|
| `core` | The shell builtins, POSIX and GNU utilities, POSIX providers, and the 14 verbs required to run and repair Bashy |
| `skill` | Other Bashy commands, including registered commands; their source can be changed through the local source workflow |
| `external` | Pinned third-party tools that Bashy downloads and executes |

The 14 core verbs are `agent`, `model`, `tool`, `skill`, `self`, `src`, `git`,
`dag`, `commands`, `inspect`, `secret`, `ask`, `claim`, and `resource`.
Protection applies to source paths rather than whole commands. `bashy src show
NAME` reports a command's pinned source and any protected parts; `bashy src
fetch [repo]` fetches a pinned OSS checkout; `bashy src check` reports edits to
core or protected paths. `src` remains experimental until its full workflow is
verified.

The default `bashy commands` first screen has a separate **featured** list of
35 often-used commands. Featured is a presentation choice, not a class.

The `bashy-commands-v2` and `bashy-atlas-v2` JSON schemas use `class` for the
invariant class and `dispatch_class` for the earlier `builtin`, `coreutils`, or
`verb` distinction. Verbose commands JSON uses `sections.featured` in place
of `sections.core`; atlas records use `featured` in place of `core`.
