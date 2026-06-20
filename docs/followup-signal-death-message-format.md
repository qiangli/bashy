# Tracked follow-up: foreground signal-death message FORMAT (#25/#26)

**Status:** behaviors #25/#26 are merged as **POSIX-conformant** (the *gating* is
correct — see below). What remains is **byte-exact bash message formatting**,
which POSIX does NOT mandate, so it does not block conformance. This note records
the gap and — importantly — how to handle it when running the **actual POSIX
conformance test suites** (Phase 2 open suites / Phase 3 VSC-PCTS).

## What shipped (sh `interp/handler.go` + `kill_unix.go` + `kill_notunix.go`)

When a **foreground external** command in a **non-interactive** shell is killed
by a fatal signal, bashy now writes a status line and:
- **default mode** → prints; **POSIX mode** → suppresses (defers to wait/jobs);
- **interactive** → suppresses (handled elsewhere);
- **SIGINT / SIGPIPE** → suppressed; exit code unchanged (128+sig).

This gating mirrors GNU bash 5.3 `jobs.c notify_of_job_status` and is the
POSIX-relevant content of #25/#26 ("non-interactive shells print status messages
after a foreground job completes; interactive shells defer"). Verified against
the bash 5.3 oracle (`ycode podman run bash:5.3`): posix-suppress, INT-suppress,
default-print all MATCH.

## The format gaps (non-POSIX-mandated; tracked)

Comparing bashy vs bash 5.3 on the *exact stderr text*:

| aspect | bash 5.3 | bashy now | why |
|---|---|---|---|
| command rendering | `sh -c 'kill -TERM $$'` (original quoting) | `sh -c kill -TERM $$` (argv joined) | bash re-serializes the parsed **pipeline AST** via `make_command_string()`; bashy's `ExecHandler` only has post-expansion `args []string`. |
| column padding | job-list aligned (`Terminated⎵⎵…⎵cmd`) | single space | bash routes through `pretty_print_job`/`print_pipeline`; bashy uses a plain `fmt.Fprintf`. |
| target stream | the **shell's** stderr (fd 2 of the shell) | `hc.Stderr` (the command's, possibly-redirected) | so `cmd 2>/dev/null` suppresses bashy's line but not bash's. |
| `(core dumped)` | present when a core was produced (`WIFCORED`) | present iff `WaitStatus.CoreDump()` | platform-dependent: macOS defaults to no core (ulimit) → absent locally; Linux → present. Likely matches on Linux. |
| prefixed vs bare form | `bash: line N: <pid> <state> (core dumped) <cmd>` (JLIST_NONINTERACTIVE) vs the foreground `print_pipeline` form | approximated | bash has two branches keyed on signal/foreground + build flags (`DONT_REPORT_SIGTERM`/`SIGPIPE`). |

### Root cause (why byte-exact is a structural change, not a tweak)
GNU bash keeps a **job table holding each job's parsed pipeline**, and renders the
message through the *same* job-list pretty-printer used by `jobs`/`bg` — so the
command keeps its original quoting and column padding for free. bashy/sh's
pure-Go model does **not** retain a parsed-pipeline job-table entry for a
foreground external command; the `ExecHandler` sees only the expanded argv and
the command's stderr. Matching bash byte-for-byte would require threading the
command's source/AST + the shell's real stderr into the exec handler and reusing
`formatJob`'s padding (i.e. wiring foreground externals through the same
job-rendering path bash uses). Real work; deferred until justified.

## Handling this in the POSIX conformance test suites (Phase 2/3)

POSIX.1-2017 / the VSC-PCTS test the shell's **observable contract**, not bash's
diagnostic wording. Concretely, when these suites are run against `bashy --posix`:

1. **POSIX mode suppresses the message entirely** — so for the conformance target
   (`--posix`), there is *no* signal-death text to mismatch. The Phase-1 parity
   harness (`scripts/posix-parity.sh`) already compares **stdout + success/fail**
   and **discards stderr**, so this never affects the parity score.
2. If a suite assertion ever inspects this message, treat a *wording/padding/
   quoting* difference as **EXPECTED / non-conformance** (annotate it like the
   `kill -l` host-OS INFO case in this doc), and a *missing-when-required* or
   *present-when-should-defer* difference as a **real failure**. Only the latter
   is a conformance bug; the former is bash-mimicry.
3. The `(core dumped)` suffix is **environment-dependent** (OS + ulimit) — never
   assert on it; gate any such probe as INFO (host-specific), same as `kill -l`.
4. If/when byte-exact bash diagnostics become a goal (e.g. for the
   bash-5.3 fixture suite rather than POSIX), do the structural job-table/AST
   wiring described above; it is NOT needed for POSIX conformance.

## Pointers
- Implementation: sh `interp/handler.go` (call site), `interp/kill_unix.go`
  `notifyForegroundSignalDeath` + `signalDescription`, `interp/kill_notunix.go`
  (no-op), `interp/signotify_test.go` (unit test of the format function).
- bash source: `jobs.c` `notify_of_job_status` (~line 4626), `print_cmd.c`
  `make_command_string` (~line 152), `pretty_print_job`/`print_pipeline`.
