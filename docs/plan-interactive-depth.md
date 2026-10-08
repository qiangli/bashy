# Interactive depth: Sprint 379, Story 1511

Plan: inspect the live interpreter/readline wiring, add focused assertions using
an isolated controlling PTY, share those assertions with a native Windows ConPTY
runner, and declare the remaining interactive limits. Run the focused tests plus
module build/vet; retain the Windows CLI CI exclusion. The 86 Bash fixtures are a
separate gate and do not establish interactive completeness.

## Probe contract

`go test ./internal/cli -run '^TestInteractiveDepthPTY$' -count=1 -v` builds the
real `cmd/bash` and exercises ten cases. `go run ./tools/interactive-depth
/path/to/bash` drives the same cases independently. Both use the repository's
`go-pty` dependency: a controlling PTY on Unix and ConPTY on Windows. Each case
has a private home, working directory and history file. The runner answers cursor
position queries and requires an executed status marker plus successful shell
exit. A marker in echoed input cannot satisfy an assertion. Timeouts fail.

Windows, from a headless session:

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o bash.exe ./cmd/bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o interactive-depth.exe ./tools/interactive-depth
# Copy both executables to the Windows test host, then run there:
interactive-depth.exe C:\path\to\bash.exe
```

These are explicit expected-behavior assertions, not proof of every flag
combination. All ten cases were also run against GNU Bash 5.3.15 on macOS. No full fixture suite is run on the development host.

## Coverage and declared limitations (2026-10-08)

| Surface | Assertion or declared limitation |
|---|---|
| `compgen` | `-W` prefix selection and no-match status 1 tested. Other actions/options and combinations remain unproven by this PTY lane. |
| `complete` | `-W` registration, direct `-p` output, `-r` removal tested. Registry state is lost inside command substitution (`complete -W 'alpha beta' x; v=$(complete -p x)` yields empty `v`): declared engine limitation. |
| TAB completion / `compopt` | Declared limitation: the interactive loop supplies no `AutoComplete` callback to readline, so registration does not establish programmable TAB completion. `-F/-C`, `COMP_*`, and `compopt` behavior during completion are not claimed. |
| `bind` | Declared limitation: interpreter dispatch is a no-op. `-p/-l/-x/-r/-q/-u/-m/-f` have no working readline integration; status 0 is not evidence of binding support. |
| `READLINE_LINE/POINT/MARK` | Declared limitation: no `bind -x` callback integration publishes or applies these values. |
| `history` | `-c/-s/-d`, `-w/-r/-a`, `-n`, `-p` tested with list contents and persistence assertions. Negative/range deletion, timestamps, filtering, size limits, concurrent sessions, and arbitrary option combinations are not proven here. |
| `fc` | `-l/-n/-r`, `-s old=new`, `-e -` tested. External editor/TTY handoff, editor failures, and arbitrary selectors are not proven by this lane (separate Unix editor tests exist). No Windows external-editor claim. Observed engine limitation: with history disabled, after `history -c`, `history -s DEPTH_VALUE=old`, a combined `fc -s old=new 1; history -s DEPTH_VALUE=edited; fc -e - -1` reports no command found rather than executing the latest entry. |
| `checkjobs` | Declared limitation: accepted shopt setting is not consulted by the interactive exit loop to warn/refuse exit with active jobs. |
| `histappend` | Declared limitation: readline persists history independently of this shopt setting. Bash-compatible append-versus-replace-on-exit and concurrent-session merge behavior are not claimed. |
| Readline | Ctrl-U discards typed input before an interactive/TTY assertion. This does not claim the full Readline keymap. |

The scope above is mirrored in `TODO.md` and `conformance-statement.md`.

## Evidence

See the delivery commit for the exact candidate. No CI run is created by this
worker; it does not push. Windows `internal/cli` remains excluded from CI: these
ten focused probes do not prove the entire package safe to run headless.

Verified 2026-10-08, using the umbrella Go workspace with
`sh` at `2a6d42599019f6cb0c60ad9ca34dcc315252d884` and
`readline` at `38ad08e836761205d820a51da9b70100c5ec72e1`:

| Host class | Exact command | Result |
|---|---|---|
| macOS development host | `go build ./...` | exit 0 |
| macOS development host | `go vet ./...` | exit 0 |
| macOS development host | `go test ./internal/interactiveprobe ./internal/cli -run 'Test(StatusRequiresExecutedMarker\|ConPTY.*\|InteractiveDepthPTY)$' -count=1 -v` | exit 0; 3 harness tests and all 10 PTY cases pass |
| macOS development host | `go run ./tools/interactive-depth /tmp/story1511-bash` | exit 0; 10/10 |
| macOS development host, GNU Bash 5.3.15 oracle | `go run ./tools/interactive-depth /opt/homebrew/bin/bash` | exit 0; 10/10 |
| macOS cross-build | `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/story1511-bash.exe ./cmd/bash` | exit 0 |
| macOS cross-build | `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/story1511-depth.exe ./tools/interactive-depth` | exit 0 |
| Native Windows, headless SSH, ConPTY | `story1511-depth.exe PATH_TO/story1511-bash.exe` | exit 0; 10/10, including Ctrl-U |

The local testee was built with `go build -o /tmp/story1511-bash ./cmd/bash`.
No Linux run or full macOS/Windows CLI suite is claimed. No release-candidate
86-fixture result is refreshed by this work.

Red-to-green harness evidence: the initial Windows run timed out on all nine
then-existing cases because trailing prompt spaces became cursor movements.
After prompt recognition was corrected, six of nine still timed out despite
status-0 output because ConPTY encoded output line starts as cursor positioning.
`TestConPTYPromptAndIncompleteMarker` and
`TestConPTYCursorPositionStartsOutputLine` reproduced those failures locally
before the fixes; the final ten-case native run passed. The initial Unix runner
also treated PTY EOF racing the child wait as failure; it now waits for the child
exit. The first one-line history setup failed against GNU Bash; explicit input
lines and disabled automatic history recording make the final cases pass both
shells. `fc -s` and `fc -e -` have independent positive-selector cases; the
combined negative-selector discrepancy remains declared above, not fixed here.
