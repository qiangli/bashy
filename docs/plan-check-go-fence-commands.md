# Check Go fence commands

Sprint 562, story `7da98b722aca`. The checker parses Bash# source blocks and
records exported top-level Go function declarations under each block's alias.
For shell command calls in that same file, `alias.Function` resolves as a Go
fence command only when the declaration exists. All other commands use the
existing builtin, userland, and PATH resolution. Typed calls retain their
current syntax path and are not classified as shell commands.

Verification: focused `TestCheckGoFence*` and `TestCheckBashPP*` regressions,
including malformed Go and two files reusing alias `g`. The parent runs the
repository gate and `docs/s562/checker-remote-gate.sh` on a remote candidate.
The CLI packet pins the three unchanged shell-command corpus fixtures by SHA256
and checks their `g.Main "$@"` resolutions, negative names, typed calls, and
ordinary PATH behavior. The check reads and parses Go source; it never runs
the user's Go program.
