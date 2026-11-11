# examples/cast — `cast record`: a terminal scene as asciicast v2

`cast record OUT.cast -- CMD [ARGS...]` runs CMD under a real pseudo-terminal
(a pty on Unix, ConPTY on Windows) and writes asciicast v2, the file every
player and `examples/video` already read. The same bytes on macOS, Linux and
Windows; the exit status is CMD's. It replaces asciinema (GPL, no Windows
build) for the one job the video pipeline needs: recording.

It is a skill-tier command, like `examples/record`: one Bash# file that is
both the skill an agent reads and the program bashy runs, registered once,
never a core change.

```bash
bashy dag examples/cast test        # package tests + the script end to end
bashy dag examples/cast register    # register it as the command `cast`
cast record scene.cast --cols 100 --rows 30 --idle-limit 2 -- bashy todo list
cast record - -- printf '\033[31mred\033[0m\n'       # "-" is stdout
```

`bashy-defaults apply` (examples/defaults) registers it from the source pin
as well, so a fresh install has it with the other defaults.

## Anatomy

| Piece | Job |
|---|---|
| `cast.bsh` | the command: a `~~~skill` fence (what agents read), a `~~~go` fence (the call into the recorder), and the shell half (flags, `--`, exit status) |
| `cast/` | the recorder package: `Record` (header, events, idle limit, UTF-8 carry across reads) with `pty_unix.go` (creack/pty) and `pty_windows.go` (aymanbagabas/go-pty ConPTY, drained after the child exits); `Parse` reads a cast back |
| `go.mod` | this example's own module: the two pty libraries; the Go fence builds against it (the nearest `go.mod` to the script) |
| `cast_bsh_test.go` | drives `cast.bsh` through a built bashy: colour codes and a rune split across reads, `-`, cwd, idle limit, usage errors |

Everything after `--` belongs to CMD. `--cols`/`--rows` default to 80x24
and never follow the recording window, so a scene has one shape.
`--idle-limit S` caps a pause between events, as asciinema's
`idle_time_limit` does. A recorder failure (no such program, no pty) exits
127 with the reason on stderr; CMD's own status is otherwise returned as is.

## Verification

```sh
GOWORK=off go -C examples/cast test ./...                    # package + script tests
GOOS=windows GOARCH=amd64 go -C examples/cast build ./...    # the ConPTY side compiles
bash examples/video/test.sh                                  # the video pipeline records its steps with cast
```
