# Plan: punted builtins (coproc / fg / umask / logout)

After the merge of bashy's Phase 3-8 work onto the `sh` fork's master, the
explicit implementations for `fg`/`bg`/`jobs`/`fc`/`umask`/`logout`/etc.
took precedence over the earlier "unsupported in this shell — …" hints. The merged spot-check tests and `TestUnsupportedHints` were trimmed
accordingly. This document captures the design work needed to bring four
of those builtins from "merged but minimal" to "actually correct".

Order of implementation:

1. **logout** + **exit** correctness (smallest, paves the way) ← this batch
2. **umask** (independent, small-medium) ← this batch
3. **fg** (reuses `wait`'s PID lookup work we already merged)
4. **coproc** (depends on numbered-fd support — biggest piece)

---

## 1. `logout` — gate `exit` on a login-shell flag

### Current state

`interp/builtin.go` `case "logout":` just sets `r.exit.exiting = true`
with no gating and no exit code propagation. Bash errors with `not login
shell: use 'exit'` if invoked from a non-login shell, otherwise behaves
like `exit`.

### Design

- Add `loginShell bool` field to `Runner` (`interp/api.go`).
- Add `WithLoginShell(bool)` `RunnerOption` so embedders (cmd/bashy
  interactive mode, outpost SSH session attach) can opt in.
- `case "logout":` becomes:
  - If `!r.loginShell`: return failure with the bash-compatible message.
  - Otherwise: reuse the same code path as `case "exit":` (accept 0 or 1
    arg, propagate `r.lastExit` if 0, parse code if 1).
- Wire Ctrl-D in `cmd/bashy/interactive.go` to the same logout-aware exit
  path when the runner was created with `WithLoginShell(true)`.

### Tradeoff

The flag is a single bool; bash also tracks "interactive" separately
(`$-` contains `i`). We're not modeling interactive vs. login distinction
here — outpost cares about login (it owns the session lifetime).
Future-proof by using a flag rather than baking the assumption into the
exit code path.

---

## 2. `umask` — per-Runner virtual umask

### Current state

`case "umask":` calls `syscall.Umask(int(mask))` directly. This is
**process-wide**: two Runners in the same Go process clobber each other,
and any non-shell code in the same binary inherits the shell's umask.
Outpost (which can host multiple Runners) cannot tolerate this.

The bare `umask` (no args) prints `0022` regardless of the actual mask.

### Design

- Add `umask int` field to `Runner`, defaulting to the process umask at
  Runner creation (read it non-destructively via the
  `m := syscall.Umask(0); syscall.Umask(m)` idiom — locked behind a
  package-level mutex to avoid races during init).
- `case "umask":` reads/writes `r.umask` only; never touches
  `syscall.Umask` after init.
- Apply the mask at the file-creation chokepoint:
  `r.open(ctx, path, flags, mode)` in `interp/runner.go`. When
  `flags & os.O_CREATE != 0`, transform `mode &^= os.FileMode(r.umask)`
  before delegating to `r.openHandler`.
- `mkdir`/`mkfifo`/etc. paths (if added later) follow the same pattern.

### Tradeoff

The umask is applied **only** to file-creation calls routed through the
runner. A custom `OpenHandler` that bypasses the runner's `open` method
(e.g., outpost middleware that calls `os.OpenFile` directly) won't have
the umask applied. That's a deliberate limit: we're modeling shell
umask, not system umask. Document on the field.

### Notes

- `syscall.Umask` only exists on Unix; gate via `_unix.go`/`_notunix.go`
  for the *init read*. On non-Unix, default `r.umask = 0o022`.
- Bare `umask` should print the actual mask, formatted as 4-digit octal
  with leading zero, e.g. `umask` → `0022` when the mask is `0o022`.
- `umask -S` (symbolic mode output) is out of scope for this batch.

---

## 3. `fg` — channel-based wait with optional SIGCONT

### Current state

After the merge, `case "fg":` returns `"fg: no current job"` when empty,
and on `fg %N` it waits via `<-bg.done` and propagates `*bg.exit`. Two
gaps:

1. It only accepts `%N` job-spec form; `fg <real-pid>` and `fg gN`
   sentinel aren't recognized (the `wait` merge fixed this for `wait`
   but `fg` wasn't updated).
2. If the underlying real PID was stopped (e.g. an external SIGSTOP),
   `<-bg.done` blocks forever because the process never finishes.

### Why we are not copying outpost's `outpost fg`

`outpost fg <pid>` (cmd/outpost/jobs.go) is a **different abstraction**:
- It reads a persistent on-disk registry of detached PIDs populated via
  `WithBgPidCallback` — pids that survived the original SSH session.
- It polls `syscall.Kill(pid, 0)` every 250 ms because the proc ref is
  gone; the OS exit status cannot be captured.
- It always returns 0 on natural exit (the comment in the file calls
  this out explicitly: "the OS exit status is not captured — this is the
  qiangli/sh detached process trade-off").

The in-shell builtin has strictly more information: the `bgProc` struct
with its `done` channel and `exit` pointer. Polling and dropping the
exit status here would be a regression. The two implementations stay
separate by design — outpost's is for cross-process detached jobs; ours
is for in-process bgProcs.

### Design (implementing in this batch)

- Mirror the merged `wait` logic for argument forms:
  - no args → most recently started `bgProc`
  - `%N` → `bgProcs[N-1]` (bash job-spec form)
  - `gN` sentinel → `bgProcs[N-1]` (matches `$!` legacy form)
  - bare integer → real-PID lookup by scanning `bg.pid.Load()`
- Once a target is picked, **if a real OS PID has been published**
  (`bg.pidReady` closed, `bg.pid.Load() > 0`), send `SIGCONT`
  best-effort to resume any stopped process. Errors are ignored — the
  proc may already be running or gone.
- Then wait `<-bg.done` and propagate exit via `r.exit`.
- For goroutine-only bgProcs (no real PID), the SIGCONT step is
  skipped. There's no "foreground" semantically because there was no
  process group transition.

### Tradeoff

Without process group / terminal control (the in-process shell has no
controlling TTY of its own), `fg` cannot truly "reattach" stdio the way
bash does. The implementation only waits + propagates exit. Embedders
that need real TTY reattach (interactive `cmd/bashy`, outpost SSH
session takeover) will need a `WithFgHandler` middleware in a later
batch; not in scope here.

### Cross-platform

`syscall.SIGCONT` is only defined on Unix. The SIGCONT step is gated
through a new `continueIfStopped(pid int)` helper in
`kill_unix.go`/`kill_notunix.go` — on non-Unix it's a no-op (there are
no suspended jobs to resume).

---

## 4. `coproc` — numbered-fd refactor

### Phase 1 — DONE (`fdTable` + coproc reads)

Shipped via `interp/api.go` (`Runner.fdTable map[int]*os.File`) and
`interp/runner.go`:

- `coproc` registers both pipe ends in `fdTable` keyed by the real OS
  fd numbers it also writes into `${COPROC[0]}` / `${COPROC[1]}`.
- `syntax.DplIn` (`<&N`) and `syntax.DplOut` (`>&N`) now look up
  numeric args via `fdTable`; arbitrary fds work for the dup forms
  instead of returning `unhandled %v arg`.
- Subshells clone `fdTable` so child mutations don't leak to the
  parent (bash inherits fds; the underlying `*os.File` handles are
  shared, the map slot ownership is not).

End-to-end test:

```sh
coproc CO { read line; echo got=$line; }
echo hi >&${CO[1]}
read out <&${CO[0]}
echo $out          # → "got=hi"
```

### Phase 2 — DONE (numbered-fd redirects routed through `fdTable`)

`redir()` now tracks a `targetFd` derived from `rd.N` and dispatches
via two helpers, `setReadFd(targetFd, *os.File)` and
`setWriteFd(targetFd, io.Writer)`. `targetFd == -1` means "use the op's
natural default" (fd 0 for input, fd 1 for output); 0/1/2 update the
stdio slots; N ≥ 3 stores/looks-up in `fdTable`. All redirect ops were
rewritten to go through this routing:

- `exec N<file` / `exec N>file` / `exec N<>file` — opens the file and
  installs it in `fdTable[N]`. `keepRedirs` (set by exec) keeps it past
  this stmt; subsequent stmts see fd N.
- Plain `N>file` / `N<file` (no exec) — same routing but scoped: the
  full `fdTable` is cloned at stmt entry and restored at stmt exit. The
  scoped clone only happens for stmts that *have* redirects, so coproc
  registrations made from inside `cmd()` (which run on a redirect-less
  stmt) still persist.
- `N>&M` / `N<&M` with arbitrary N — `fdTable[N]` is set from
  `fdTable[M]` (or stdin/stdout/stderr if M is 0/1/2).
- `N>&-` / `N<&-` — deletes `fdTable[N]` (or sets the stdio slot to
  `io.Discard` / `nil` for 0/1/2).
- Heredocs (`N<<EOF`) — the input-side pipe now goes through
  `setReadFd(targetFd, …)` so `N<<EOF` for `N >= 3` works too.

Plus two pre-existing bugs uncovered along the way:

- **`cls.Close()` ran even when `keepRedirs` was set** (so `exec 3<file`
  would close the file immediately). The deferred close now reads
  `r.keepRedirs` at fire time and skips when set; the file's ownership
  has already transferred to `fdTable` or stdio.
- **`keepRedirs` leaked across statements** (exec set it once, every
  subsequent stmt observed it and skipped its own restore, which broke
  scoped redirects after any prior exec-with-redirect). `keepRedirs`
  is now reset at the end of every `stmtSync` via a LIFO defer ordered
  after the file-close defers — so the in-stmt close behavior is
  preserved while subsequent stmts get fresh scoping.

### Phase 2 — named-fd allocator DONE

`{varname}>file` / `{varname}<file` / `{varname}>>file` / `{varname}<>file`
now route through `allocateFd()` which picks the next unused fd ≥ 10
(matches bash's convention of starting above the conventional 0-9
stdio range). The allocated number is written back to the named
variable so scripts can use it via `>&$var` / `<&$var`. Also handles
`{var}>&-` / `{var}<&-` (read the fd from `$var`, delete from
`fdTable`).

End-to-end example:

```sh
exec {fd}>f          # fd=10, fdTable[10] = open(f, …)
echo hi >&$fd        # write goes to f
exec {fd}>&-         # close: delete fdTable[10]; $fd keeps the
                     # stale "10" (bash matches this)
cat f                # → "hi"
```

The allocator returns the first gap in `fdTable` starting at 10, so
two simultaneous `exec {a}>… {b}>…` calls hand out distinct numbers
(10, then 11).

### Phase 2 — still TODO

- Custom `OpenHandler` returning non-`*os.File`: `setWriteFd` for
  `N >= 3` requires a `*os.File` because a numbered fd must back a real
  OS handle. A handler that returns a custom `io.ReadWriteCloser`
  (e.g., an in-memory mock) cannot have its result installed at fd N.
  Acceptable limit for now; documented on the helper.

### Tradeoff (still applies)

Real OS fds are still required for `exec` of external commands — bash
passes them through `execve`'s fd inheritance, which only works with
real fds. So the channel-based emulation idea is a dead end for any
script that ever execs an external program against a coproc fd. Phase 1
+ Phase 2 keep real `os.Pipe()` and a virtual fd table — that's the
general answer.

---

## Out of scope for any of this batch

- `bind` (readline keybindings) — irrelevant outside `cmd/bashy`
  interactive mode; punt.
- `caller` / `help` — local has stubs; not blocking outpost.
- `compgen` / `complete` / `compopt` — local stubs are fine; programmable
  completion is genuinely an SSH-client concern.
- `enable -n` (disable builtin) — local tracks it via
  `r.disabledBuiltins`; works for what outpost needs.
- `times` — local has the stub; the `time CMD` form already works via
  `syntax.TimeClause`.
