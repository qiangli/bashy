// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// `bashy supervisord DAG.md TARGET` supervises exactly ONE `bashy dag` root in
// the foreground — the intentionally small init-shaped verb (Sprint 218).
//
// What it does, and all it does:
//
//   - spawns the current executable as `bashy dag DAG.md TARGET`, in its own
//     process group, with stdin/stdout/stderr inherited (no log files);
//   - on an UNEXPECTED exit (non-zero, or killed by a signal we did not send)
//     restarts it with bounded exponential backoff, reset after a healthy run
//     (Outpost's restart model; supervisord's `autorestart=unexpected`). An
//     exit 0 is the root completing, and the supervisor exits 0 with it;
//   - on TERM/INT forwards the signal to the whole child process group, waits a
//     bounded grace, then KILLs the group and exits 0;
//   - as PID 1 on Linux (or as a child subreaper, which it becomes when it can)
//     reaps adopted orphans without racing the main child's wait — see
//     supervisord_linux.go for the one race that matters and how it is closed.
//
// The DAG's existing `Requires:` edges are the ONLY dependency format: the
// supervisor never reads the file itself. It asks the real runner for the plan
// (`bashy dag --json -n`) once at startup, so an unknown target or unparseable
// file is a diagnostic and an exit, never a restart loop, and the plan's order
// is printed so an operator sees what "the root" pulls in.
//
// Deliberately NOT in this mode (KISS, Sprint 218): daemonizing, pidfiles, a
// control socket, log files, config files, watching the DAG file, multiple
// programs, readiness protocols, mounts, TTY setup, or anything a kernel
// boots. `bashy supervisord` with no operands is the service monitor over
// every enabled `bashy service` unit (Sprint 418 S16; supervisord_monitor.go,
// docs/bashy-service-design.md), which has the socket, logs and many units;
// S19 moves the dag-root callers onto a recipe unit and retires this mode.
package agentos

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Defaults. min/max bound the restart delay; healthyAfter is how long a run
// must last before the next failure starts the ladder over at min; grace is
// how long a forwarded TERM/INT gets before the group is KILLed.
const (
	supervisordMinBackoff   = 1 * time.Second
	supervisordMaxBackoff   = 30 * time.Second
	supervisordHealthyAfter = 30 * time.Second
	supervisordGrace        = 10 * time.Second
)

const supervisordUsage = `usage: bashy supervisord [flags] DAG.md TARGET

Supervise one bashy dag root in the foreground: run ` + "`bashy dag DAG.md TARGET`" + `
in its own process group with this terminal's stdin/stdout/stderr, restart it
on an unexpected exit (non-zero or signaled) with bounded exponential backoff,
and on TERM/INT forward the signal to the whole process group, wait --grace,
then kill it. The target's Requires: edges are the only dependency format.
An exit 0 means the root completed; the supervisor then exits 0 too.

As PID 1 on Linux (a container entrypoint) it also reaps orphaned descendants.

flags:
  --min-backoff DUR    first restart delay (default %v)
  --max-backoff DUR    restart delay ceiling (default %v)
  --healthy-after DUR  a run this long resets the backoff (default %v)
  --grace DUR          TERM/INT → KILL grace (default %v)
`

// dispatchSupervisord implements `bashy supervisord`: with DAG.md TARGET
// operands the one-root supervisor below; without, the service monitor
// (supervisord_service.go). It runs before shell flag parsing and uses the
// process stdio directly, as the dag child will.
func dispatchSupervisord(args []string) int {
	if !supervisordDAGMode(args) {
		return dispatchSupervisordMonitor(args)
	}
	return dispatchSupervisordDAG(args)
}

// supervisordDAGMode: an operand, or a flag only the one-root mode has.
func supervisordDAGMode(args []string) bool {
	valued := map[string]bool{"min-backoff": true, "max-backoff": true, "healthy-after": true, "grace": true,
		"socket": false, "poll": false}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return i+1 < len(args)
		}
		if !strings.HasPrefix(a, "-") {
			return true
		}
		name, _, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		dag, known := valued[name]
		if dag {
			return true
		}
		if known && !hasVal {
			i++
		}
	}
	return false
}

func dispatchSupervisordDAG(args []string) int {
	fs := flag.NewFlagSet("supervisord", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	s := &supervisor{
		self:   bashySelfPath(),
		log:    os.Stderr,
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		after:  time.After,
		now:    time.Now,
	}
	fs.DurationVar(&s.minBackoff, "min-backoff", supervisordMinBackoff, "")
	fs.DurationVar(&s.maxBackoff, "max-backoff", supervisordMaxBackoff, "")
	fs.DurationVar(&s.healthyAfter, "healthy-after", supervisordHealthyAfter, "")
	fs.DurationVar(&s.grace, "grace", supervisordGrace, "")
	usage := func(w io.Writer) {
		fmt.Fprintf(w, supervisordUsage, supervisordMinBackoff, supervisordMaxBackoff, supervisordHealthyAfter, supervisordGrace)
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage(os.Stdout)
			return 0
		}
		fmt.Fprintln(os.Stderr, "bashy supervisord:", err)
		usage(os.Stderr)
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "bashy supervisord: exactly one DAG file and one target are supervised")
		usage(os.Stderr)
		return 2
	}
	s.dagFile, s.target = fs.Arg(0), fs.Arg(1)
	if err := s.validate(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy supervisord:", err)
		return 2
	}
	s.start = s.startDagChild

	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(sigs)
	s.signals = sigs

	if r := startReaper(); r != nil {
		s.reaper = r
		defer r.stop()
	}
	return s.run()
}

// childExit is how one run of the root ended.
type childExit struct {
	code     int
	signaled bool
	err      error // the wait itself failed (status unknown); code is then 1
}

func (e childExit) String() string {
	if e.err != nil {
		return "wait failed: " + e.err.Error()
	}
	if e.signaled {
		return fmt.Sprintf("killed by signal (status %d)", e.code)
	}
	return fmt.Sprintf("exit %d", e.code)
}

// supervisedChild is one spawned root: the seam the unit tests fake. Signal
// and Kill address the WHOLE process group the child leads, never just the
// child, and are no-ops once the group is empty.
type supervisedChild interface {
	Wait() childExit
	Signal(sig os.Signal) error
	Kill() error
}

// supervisor is the loop. Everything that touches a clock or the OS is a
// field so a test can inject a fake child, a fake timer and a fake signal
// source and drive the loop with no sleeping and no real processes.
type supervisor struct {
	self, dagFile, target string

	minBackoff, maxBackoff, healthyAfter, grace time.Duration

	log            io.Writer
	stdin          io.Reader
	stdout, stderr io.Writer

	start   func() (supervisedChild, error)
	after   func(time.Duration) <-chan time.Time
	now     func() time.Time
	signals <-chan os.Signal
	reaper  *reaper

	// preflightDone lets a test skip the plan query when the fake start does
	// not spawn the real runner.
	preflightDone bool
}

func (s *supervisor) validate() error {
	if s.minBackoff <= 0 || s.maxBackoff < s.minBackoff {
		return fmt.Errorf("--min-backoff must be > 0 and --max-backoff >= --min-backoff (got %v, %v)", s.minBackoff, s.maxBackoff)
	}
	if s.grace < 0 || s.healthyAfter < 0 {
		return fmt.Errorf("--grace and --healthy-after must not be negative")
	}
	if _, err := os.Stat(s.dagFile); err != nil {
		return err
	}
	return nil
}

func (s *supervisor) logf(format string, args ...any) {
	fmt.Fprintf(s.log, "supervisord: "+format+"\n", args...)
}

// dagArgv is the one argv every spawn of the root uses.
func (s *supervisor) dagArgv() []string {
	return []string{"dag", s.dagFile, s.target}
}

// preflight asks the real runner for the dry-run plan of the root. A file or
// target the runner rejects is reported with the runner's own diagnostic and
// exit code — it is never restarted — and the plan (the Requires: closure in
// topological order) is logged so the supervised set is visible.
func (s *supervisor) preflight() int {
	cmd := exec.Command(s.self, "dag", "--json", "-n", s.dagFile, s.target)
	cmd.Env = os.Environ()
	cmd.Stderr = s.stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			s.logf("cannot plan %s %s (dag exit %d)", s.dagFile, s.target, ee.ExitCode())
			return ee.ExitCode()
		}
		s.logf("cannot run %s: %v", s.self, err)
		return 127
	}
	var plan struct {
		Result struct {
			Plan []struct {
				Name string `json:"name"`
			} `json:"plan"`
		} `json:"result"`
	}
	names := []string{}
	if json.Unmarshal(out, &plan) == nil {
		for _, p := range plan.Result.Plan {
			names = append(names, p.Name)
		}
	}
	s.logf("supervising %s %s (plan: %s)", s.dagFile, s.target, strings.Join(names, " -> "))
	return 0
}

// run is the supervisor loop: spawn → wait → (complete | restart with backoff
// | shutdown on signal). Returns the process exit status.
func (s *supervisor) run() int {
	if !s.preflightDone {
		if code := s.preflight(); code != 0 {
			return code
		}
	}
	ladder := newBackoffLadder(s.minBackoff, s.maxBackoff, s.healthyAfter)
	for {
		var backoff time.Duration
		child, err := s.start()
		if err != nil {
			backoff = ladder.delay(0)
			s.logf("cannot start %s: %v", strings.Join(append([]string{s.self}, s.dagArgv()...), " "), err)
		} else {
			started := s.now()
			exitCh := make(chan childExit, 1)
			go func() { exitCh <- child.Wait() }()
			var ex childExit
			select {
			case ex = <-exitCh:
			case sig := <-s.signals:
				return s.shutdown(child, sig, exitCh)
			}
			if ex.code == 0 && !ex.signaled && ex.err == nil {
				s.logf("%s completed (exit 0); done", s.target)
				return 0
			}
			backoff = ladder.delay(s.now().Sub(started)) // a healthy run starts the ladder over
			// Whatever the root left behind in its group is told to go now and
			// is killed before the replacement starts, so a straggler never
			// holds the port the restart needs. Both are no-ops on an empty group.
			_ = child.Signal(syscall.SIGTERM)
			s.logf("%s %s; restarting in %v", s.target, ex, backoff)
		}
		if err != nil {
			s.logf("restarting in %v", backoff)
		}
		select {
		case <-s.after(backoff):
		case sig := <-s.signals:
			s.logf("%v while waiting to restart; exiting", sig)
			return 0
		}
		if child != nil {
			_ = child.Kill()
		}
	}
}

// shutdown forwards sig to the child's whole process group, waits at most
// grace for it to exit (a second signal shortens that to now), then KILLs the
// group. The supervisor's own exit is clean (0) either way — being told to
// stop is not a failure.
func (s *supervisor) shutdown(child supervisedChild, sig os.Signal, exitCh <-chan childExit) int {
	s.logf("%v: forwarding to the process group of %s, grace %v", sig, s.target, s.grace)
	if err := child.Signal(sig); err != nil {
		s.logf("forwarding %v: %v", sig, err)
	}
	select {
	case ex := <-exitCh:
		s.logf("%s stopped (%s)", s.target, ex)
		return 0
	case <-s.after(s.grace):
		s.logf("grace expired; killing the process group of %s", s.target)
	case sig2 := <-s.signals:
		s.logf("%v again; killing the process group of %s", sig2, s.target)
	}
	if err := child.Kill(); err != nil {
		s.logf("kill: %v", err)
	}
	ex := <-exitCh
	s.logf("%s killed (%s)", s.target, ex)
	return 0
}

// execChild is the real supervisedChild: the current executable running the
// dag root as the leader of a fresh process group, stdio inherited.
type execChild struct {
	cmd    *exec.Cmd
	reaper *reaper
}

func (s *supervisor) startDagChild() (supervisedChild, error) {
	cmd := exec.Command(s.self, s.dagArgv()...)
	cmd.Env = os.Environ()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.stdin, s.stdout, s.stderr
	cmd.SysProcAttr = ownProcessGroupAttr()
	return startTracked(cmd, s.reaper)
}

// startTracked starts cmd inside a reaper spawn window (r may be nil), so the
// status of a child the reaper collects first is never lost. Both supervisord
// modes and the monitor's exec probes spawn through it.
func startTracked(cmd *exec.Cmd, r *reaper) (*execChild, error) {
	if r != nil {
		r.arm()
	}
	if err := cmd.Start(); err != nil {
		if r != nil {
			r.disarm()
		}
		return nil, err
	}
	if r != nil {
		r.watch(cmd.Process.Pid)
	}
	return &execChild{cmd: cmd, reaper: r}, nil
}

func (c *execChild) Wait() childExit {
	err := c.cmd.Wait()
	// take also forgets the pid, so it runs whichever way the wait went.
	var stashed childExit
	var ok bool
	if c.reaper != nil {
		stashed, ok = c.reaper.take(c.cmd.Process.Pid)
	}
	if ps := c.cmd.ProcessState; ps != nil {
		code, signaled := procStatus(ps)
		return childExit{code: code, signaled: signaled}
	}
	// The wait itself failed. On Linux that is the reaper having collected our
	// child first (ECHILD); it kept the status for exactly this moment.
	if ok {
		return stashed
	}
	return childExit{code: 1, err: err}
}

func (c *execChild) Signal(sig os.Signal) error { return signalProcessGroup(c.cmd.Process, sig) }
func (c *execChild) Kill() error                { return killProcessGroup(c.cmd.Process) }
