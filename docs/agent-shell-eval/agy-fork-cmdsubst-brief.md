# Agy Brief: Exec-Backed Command Substitution Prototype

## Goal

Make bashy pass the ShellBench timed smoke where GNU Bash 5.3 succeeds:

```sh
/usr/bin/timeout 30 setsid /bench/shellbench -e -s /usr/local/bin/bashy -t 1 -w 1 sample/count.sh sample/output.sh
```

The immediate target is not a broad rewrite of command substitution. It is a narrowly gated prototype that gives signal-sensitive command substitutions a real OS process boundary when explicitly enabled.

## Background

ShellBench runs the timed benchmark inside command substitution:

```sh
count=$(bench "$shell" "$code" "$BENCHMARK_TIME")
```

Inside `bench`, ShellBench coordinates readiness and timing with traps and process-group signals:

```sh
MAIN_PID=$(exec sh -c 'echo $PPID')
export MAIN_PID
trap 'ready=$(($ready + 1))' HUP
"$1" -c "$2" 3>&1 >/dev/null &
stopper "$3" "$!" &
while [ "$ready" -lt 2 ]; do dummy=; done
sleep "$WARMUP_TIME" &
wait "$!" || exit 1
kill -HUP "-$$"
wait || exit 1
```

GNU Bash uses a forked command-substitution subshell. External children therefore see a distinct command-substitution process as `PPID`, and `kill -HUP "$MAIN_PID"` can trigger the trap in that subshell.

Bashy currently runs ordinary command substitutions in-process as a goroutine. External children see the top-level bashy OS process as their parent, so the signal does not target the command-substitution runner that owns the trap. The benchmark times out.

Several adjacent issues are already fixed:

- inherited fd discovery/export in bashy CLI startup;
- write-only inherited fd export from `../sh`;
- `$!` under `set -u`;
- inherited `$!` visibility in background subshells without making parent jobs waitable;
- non-file command-substitution writers bridged through `os.Pipe` for external children.

The remaining ShellBench blocker is OS process identity and signal routing for command substitution.

## Proposed Implementation

Add an explicit opt-in mode first:

```sh
BASHY_FORK_CMD_SUBST=always
```

Optional later mode:

```sh
BASHY_FORK_CMD_SUBST=auto
```

For this task, implement `always` if feasible. If `always` is too large, produce a tested design doc and a minimal failing regression that proves the exact gap.

### Expected Behavior

When `BASHY_FORK_CMD_SUBST=always` is set, regular `$(...)` command substitutions should execute in a child bashy process with:

- stdout captured through an OS pipe;
- stderr behavior matching current command substitution behavior;
- stdin/working directory inherited as bash would;
- shell options that matter for the body preserved enough for ShellBench and basic bash tests;
- exported variables/functions available to the child;
- exit status propagated to `r.lastExpandExit`;
- no effect on `${ cmd; }` funsub or `${| cmd; }` valsub, because those intentionally run in caller scope.

Start narrow. It is acceptable for non-exported local variables/functions to remain unsupported in the first prototype if this is clearly documented and the mode is explicitly opt-in.

### Suggested Approach

Work in `../sh`, primarily in `interp/runner.go` command-substitution handling inside `fillExpandConfig`.

1. Add an internal helper like `forkCommandSubst(ctx, w, cs)` or equivalent.
2. Render `cs.Stmts` to shell source using `syntax.Printer` with existing bash-compatible printer options where appropriate.
3. Spawn the current shell binary as a child process with `-c <rendered-source>`.
   - Prefer an existing argv0/executable path if the runner already tracks one.
   - If there is no robust current-binary path in `../sh`, add the minimal plumbing in bashy CLI to set it through a runner option.
4. Capture stdout with an OS pipe and copy it into the command-substitution writer.
5. Set the child process group/session behavior only if necessary for ShellBench. First try ordinary child execution; ShellBench itself is launched under `setsid`.
6. Preserve/propagate `BASHY_INHERITED_FDS` behavior if the child needs fd 3 forwarding.
7. Gate the path behind `BASHY_FORK_CMD_SUBST=always`; default behavior must remain unchanged.

## Acceptance Tests

Add focused tests where they naturally belong. At minimum:

```sh
echo "$(bashy -c 'echo hi >&3' 3>&1 >/dev/null)"
```

already passes after the fd bridge; keep it from regressing if you touch that path.

Add a signal-sensitive command-substitution regression. A direct test can be adapted from ShellBench:

```sh
BASHY_FORK_CMD_SUBST=always bashy -c '
  out=$(
    MAIN_PID=$(exec sh -c "echo $PPID")
    export MAIN_PID
    ready=0
    trap "ready=$((ready + 1))" HUP
    bashy -c "kill -HUP \"\$MAIN_PID\"" &
    while [ "$ready" -lt 1 ]; do :; done
    echo ready
  )
  test "$out" = ready
'
```

If this exact script needs adjustment for quoting, keep the same semantics: an external child must signal the command-substitution process and trigger a trap in that command-substitution execution context.

Then verify the actual ShellBench smoke manually:

```sh
make build-bashy BASHY_ENGINES=1 VERSION=eval-fork-cmdsubst
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
  -ldflags '-s -w -X github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-eval-fork-cmdsubst' \
  -o "$HOME/tests/bashy-eval/bin/bashy-linux-arm64" ./cmd/bashy
./eval/agent-shell/container-preflight.sh "$HOME/tests/bashy-eval"
bin/bashy podman run --rm \
  -e BASHY_FORK_CMD_SUBST=always \
  -v "$HOME/tests/bashy-standard-benchmarks/shellbench:/bench:ro" \
  -w /bench bashy-agent-shell:bashy-current \
  -c '/usr/bin/timeout 30 setsid /bench/shellbench -e -s /usr/local/bin/bashy -t 1 -w 1 sample/count.sh sample/output.sh'
```

## Required Gates

Run at least:

```sh
env GOCACHE=/private/tmp/bashy-gocache go test ./...        # in ../sh
env GOCACHE=/private/tmp/bashy-gocache go test ./...        # in bashy
make test-bash-parallel                                    # in bashy
bin/bashy dag DAG.md yash                                  # in bashy, if runtime allows
```

Report the scores:

- Bash 5.3 fixture score, expected `86 passed, 0 failed, 0 skipped, 0 timed out`.
- Yash POSIX Alpine/Debian scores, latest baseline before this task:
  - Alpine: bashy `1762 OK / 64 ERROR`, 96%.
  - Debian: bashy `1776 OK / 62 ERROR`, 96%.

## Constraints

- Do not change default command-substitution behavior unless tests prove it is safe. The first useful mode is opt-in.
- Do not break funsub/valsub caller-scope semantics.
- Do not vendor GPL benchmark/test suites.
- Keep changes scoped; if the robust solution is larger than expected, stop with a concrete design and a minimal regression test.
- Commit work in the isolated weave workspace only after local verification.
