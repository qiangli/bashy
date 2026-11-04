# Sprint 561 A3b: public shell reproductions

Story `ddf0079eb063`. Baseline: frozen bashy tree, `GOWORK=off`, pinned
`mvdan.cc/sh/v3` fork at `6663c93a2d3c`. Probes used standalone `cmd/bash`
builds invoked through a `sh` symlink, the fixed `cmd/bashy` multicall `sh`
alias, GNU Bash 5.3.15, and dash on Darwin.
The public commands below require no licensed test suite. Outcomes describe
this candidate only; the older VSC assertions still need a final approved run.

| A2 item | Public reproduction | Frozen / GNU Bash 5.3 / dash | Classification |
| --- | --- | --- | --- |
| I12, `$!` after `( ... ) &` | `sh -c '(:) & p=$!; case $p in *[!0-9]*|"") echo bad;; *) kill -0 "$p" 2>/dev/null && echo live-pid;; esac; wait "$p"; echo wait:$?'` | All `live-pid`, `wait:0` | Historical defect already absent at frozen pin; no current sh repair. |
| I11 #5, GA26 | `sh -c 'printf "ok\n"; exit 37'` | All print `ok`, exit 37 | Environment/evidence gap: the available A2 record says only “GA26”, with no assertion text or inputs. This public utility smoke cannot dispose of that exact TP. No speculative fix. |
| I11 #46, `sh -s` | `printf 'printf "one:%%s\n" "$1"\n' | sh -s first` | All print `one:first`, exit 0 | No current defect in the public form; historical TP needs its original inputs for disposition. |
| I11 #59, `PATH_MAX` | Create a script path 928 bytes long where `getconf PATH_MAX` reports 1024; invoke `sh "$path"`, then invoke it with 400 more `/.` bytes. | All run the shorter path; all reject the overlong path (bashy/GNU exit 126, dash exit 2). | Environment-specific boundary: Darwin's 1024-byte limit differs from the historical Linux harness's 4096-byte setting. No current product defect shown. |
| I11 #67/#68, `-c`/`-s` stdin | `printf 'DATA\n' | sh -c 'IFS= read -r x; printf "zero:%s one:%s stdin:%s\n" "$0" "$1" "$x"' -s first` | Frozen `zero:first one: stdin:DATA`; GNU/dash `zero:-s one:first stdin:DATA` | **Confirmed CLI defect.** The stdin bytes agree, but `-s` after the command is consumed as a shell flag instead of `$0`. After the fix all three agree. `sh -s -c 'read ...'` also agrees before and after. |
| I11 #244, subshell syntax status | `sh -c '(:; if ) ; printf after'` | All exit 2, empty stdout, diagnostic on stderr | No current defect in the public syntax-error case; original TP input is unavailable. |
| I11 #801, async events | `sh -c 'trap "echo chld" CHLD; (exit 7) & p=$!; wait "$p"; echo status:$?'` | All print `chld` and `status:7`, exit 0 | No current defect in the public asynchronous event case; original TP input is unavailable. |

## Fix and verification

`internal/cli` owns the confirmed `-c` argument-scan error. Go's flag parser
kept interpreting option-looking words after the command operand. The fix
ends option parsing at that boundary. The `sh` interpreter pin is unchanged,
so there is no `umbrella/sh` diff to merge or pin bump to review.

- Red: `GOWORK=off go test ./internal/cli -run '^TestShCommandOperandEndsOptionParsing$' -count=1 -v` failed: got `zero=operand one= stdin=data`, wanted `zero=-s one=operand stdin=data`.
- Green: same command passed after the CLI fix. The existing Profile B sh entrypoint and command-flag relocation tests also passed in the focused run.
- `GOWORK=off go build -o /tmp/s561-a3b-bashy ./cmd/bashy`, symlinked as `sh`: the I12 PID, `-s`, `-c ... -s`, syntax-error, and CHLD cases all matched GNU Bash and dash after the fix.
- `GOWORK=off go test ./interp -run 'TestJobCarrierBuiltinPidReadyBug|TestTP462DollarBangAsyncLists|TestJobCarrierSingleCommandJobsPIDMatchesDollarBang' -count=1 -v` at the frozen sh pin: 3/3 pass. The TP462 test printed two distinct numeric PIDs.
- `GOWORK=off make test-bash`: exit 0, `Results: 86 passed, 0 failed, 0 skipped, 0 timed out`.

GA26 and the original inputs for #46, #59, #244, and #801 remain evidence
gaps, not claims that the licensed TPs now pass. A4 should compare the final
candidate under the approved harness and retain any differing inputs and
diagnostics for further causal triage.
