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

## Manager rework: option boundary and F3 self-signal

GNU Bash 5.3.15 and dash were compared with the seven requested `sh` forms.
All agreed on `-ec`, options between `-c` and its command, `--` after its
command, `-s` on either side of `-c`, and `+o errexit` before `-c`.
`sh -c '' a` exposed a second CLI defect: the frozen CLI tried to open `a`
as a script (127), while both references exit 0 with empty output. The
process-level table test was red only on that case; it and the prior
`-c ... -s` regression are green after recording the presence of `-c`
independently of its string value. A script operand followed by `-c` remains
a script argument, verified by the same test.

F3: `kill -KILL $$` and `kill -TERM $$` on the pinned interpreter returned
normal exit status 137/143. Both reference shells terminate by signal 9/15.
The new sh regression test observed ordinary exit in both cases (red); after
relaying the default signal in a standalone runner it observes `WIFSIGNALED`
with the exact signal in both cases (green). Focused command:
`GOWORK=off go test ./interp -run 'TestBuiltinSelfSignal|TestTrapResetRestoresOSDefault|TestRunPreservesSignaledForegroundTermination' -count=1`
passed in `/tmp/s561-a3b-f3-codex`. Review commit `74586fde` there before
merging. The bashy `sh` pin is intentionally unchanged pending review.
With the CLI rework in this branch and the frozen sh pin, `GOWORK=off make
test-bash` completed: 86 passed, 0 failed, 0 skipped, 0 timed out.

### sh diff for manager review

```diff
diff --git a/interp/builtin.go b/interp/builtin.go
index 4e841bad..50961ce0 100644
--- a/interp/builtin.go
+++ b/interp/builtin.go
@@ -2018,6 +2018,14 @@ func (r *Runner) builtin(ctx context.Context, pos syntax.Pos, name string, args
                             // would suspend all in-process runners.
                             continue
                         }
+                        // A standalone shell owns its process signal disposition.
+                        // Relay the default action so its parent observes WIFSIGNALED,
+                        // rather than a normal exit with status 128+signal.
+                        if _, standalone := r.sigReset.(OSSignalResetter); standalone {
+                            if err := relayAsyncOwnerSignal(sig); err == nil {
+                                continue
+                            }
+                        }
                         exit.code = uint8(128 + sigNum(sig))
                         exit.exiting = true
                         return exit
diff --git a/interp/self_signal_unix_test.go b/interp/self_signal_unix_test.go
new file mode 100644
index 00000000..da5de1dd
--- /dev/null
+++ b/interp/self_signal_unix_test.go
@@ -0,0 +1,59 @@
+//go:build unix
+
+package interp_test
+
+import (
+    "context"
+    "errors"
+    "os"
+    "os/exec"
+    "strings"
+    "syscall"
+    "testing"
+
+    "mvdan.cc/sh/v3/interp"
+    "mvdan.cc/sh/v3/syntax"
+)
+
+func TestBuiltinSelfSignalTerminatesStandaloneShell(t *testing.T) {
+    for _, tc := range []struct {
+        name string
+        sig  syscall.Signal
+    }{
+        {"KILL", syscall.SIGKILL},
+        {"TERM", syscall.SIGTERM},
+    } {
+        t.Run(tc.name, func(t *testing.T) {
+            cmd := exec.Command(os.Args[0], "-test.run=^TestBuiltinSelfSignalChild$")
+            cmd.Env = append(os.Environ(), "GOSH_PROG=", "SH_SELF_SIGNAL="+tc.name)
+            out, err := cmd.CombinedOutput()
+            var exit *exec.ExitError
+            if !errors.As(err, &exit) {
+                t.Fatalf("child exit = %v, output = %q, want signal %v", err, out, tc.sig)
+            }
+            status, ok := exit.Sys().(syscall.WaitStatus)
+            if !ok || !status.Signaled() || status.Signal() != tc.sig {
+                t.Fatalf("child wait status = %v, output = %q, want signal %v", exit.Sys(), out, tc.sig)
+            }
+        })
+    }
+}
+
+func TestBuiltinSelfSignalChild(t *testing.T) {
+    name := os.Getenv("SH_SELF_SIGNAL")
+    if name == "" {
+        return
+    }
+    file, err := syntax.NewParser().Parse(strings.NewReader("kill -"+name+" $$"), "")
+    if err != nil {
+        t.Fatal(err)
+    }
+    runner, err := interp.New(interp.WithSignalResetter(interp.OSSignalResetter{}))
+    if err != nil {
+        t.Fatal(err)
+    }
+    if err := runner.Run(context.Background(), file); err != nil {
+        t.Fatalf("shell returned normally: %v", err)
+    }
+    t.Fatal("shell returned normally")
+}
```
