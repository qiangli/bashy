// `bashy supervisord` is the service monitor (Sprint 418 S16): it keeps every
// enabled `bashy service` unit in its desired state (supervisord_monitor.go).
// The Sprint 218 form `bashy supervisord DAG.md TARGET`, which supervised one
// dag root, is retired (S19): a dag root is now a unit whose exec is
// `bashy dag DAG.md TARGET`, and what that form guaranteed lives on as unit
// semantics under the one monitor policy:
//
//   - restart with bounded exponential backoff, reset after a healthy run
//     (restart.backoff.min|max|healthy_after; a clean exit completes the unit);
//   - TERM/INT to the monitor stop every unit: the declared stop signal to the
//     unit's whole process group, stop.timeout of grace, then KILL;
//   - as PID 1 on Linux (or as a child subreaper, which it becomes when it can)
//     orphans are reaped without racing a unit's own wait — see
//     supervisord_linux.go for the one race that matters and how it is closed.
//
// This file keeps what units and probes share: the defaults, the exit status
// of one run, and the tracked spawn. The old form is only recognised in order
// to be refused, with the unit that replaces it spelled out.
package agentos

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Defaults for a unit that declares no policy. min/max bound the restart
// delay; healthyAfter is how long a run must last before the next failure
// starts the ladder over at min; grace is how long a stop signal gets before
// the unit's group is KILLed.
const (
	supervisordMinBackoff   = 1 * time.Second
	supervisordMaxBackoff   = 30 * time.Second
	supervisordHealthyAfter = 30 * time.Second
	supervisordGrace        = 10 * time.Second
)

// dispatchSupervisord implements `bashy supervisord`: the service monitor.
// It runs before shell flag parsing and uses the process stdio directly. The
// retired one-dag-root form is refused (exit 2) with a pointer to its unit.
func dispatchSupervisord(args []string) int {
	if retired := retiredDAGForm(args); retired != nil {
		fmt.Fprint(os.Stderr, retired.message())
		return 2
	}
	return dispatchSupervisordMonitor(args)
}

// retiredDAG is what was recognised of a Sprint 218 invocation.
type retiredDAG struct {
	dagFile, target string
	flags           map[string]string // retired flag -> value as given
}

// retiredFlagField maps each retired flag to the unit field that replaced it.
var retiredFlagField = map[string]string{
	"min-backoff":   "restart.backoff.min",
	"max-backoff":   "restart.backoff.max",
	"healthy-after": "restart.backoff.healthy_after",
	"grace":         "stop.timeout",
}

// retiredDAGForm reports whether args are the retired form: any operand (the
// monitor takes none), or a flag only that form had.
func retiredDAGForm(args []string) *retiredDAG {
	r := &retiredDAG{flags: map[string]string{}}
	var operands []string
	found := false
	monitorValued := map[string]bool{"socket": true, "poll": true, "only": true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			operands = append(operands, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			operands = append(operands, a)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if _, ok := retiredFlagField[name]; ok {
			found = true
			if !hasVal && i+1 < len(args) {
				i++
				val = args[i]
			}
			r.flags[name] = val
			continue
		}
		if monitorValued[name] && !hasVal {
			i++
		}
	}
	if len(operands) > 0 {
		found = true
		r.dagFile = operands[0]
		if len(operands) > 1 {
			r.target = operands[1]
		}
	}
	if !found {
		return nil
	}
	return r
}

// message is the refusal: what was retired, the unit that replaces it (with
// the operands and flags the caller gave already filled in), and how to run it.
func (r *retiredDAG) message() string {
	var b strings.Builder
	b.WriteString("bashy supervisord: the `DAG.md TARGET` form was retired (Sprint 418 S19).\n")
	b.WriteString("`bashy supervisord` is now the service monitor and takes no operands. A dag root is a\n")
	b.WriteString("unit whose exec is `bashy dag DAG.md TARGET`; restart backoff, the stop grace and the\n")
	b.WriteString("TERM/INT forwarding are fields of that unit.\n\n")
	dag, target := r.dagFile, r.target
	if dag == "" {
		dag = "DAG.md"
	}
	if target == "" {
		target = "TARGET"
	}
	fmt.Fprintf(&b, "  # root.yaml\n  name: root\n  kind: service\n  exec: [bashy, dag, %s, %s]\n  enabled: true\n", dag, target)
	var restart, stop []string
	for _, n := range []string{"min-backoff", "max-backoff", "healthy-after"} {
		if v, ok := r.flags[n]; ok {
			restart = append(restart, strings.TrimPrefix(retiredFlagField[n], "restart.backoff.")+": "+v)
		}
	}
	if v, ok := r.flags["grace"]; ok {
		stop = append(stop, "timeout: "+v)
	}
	if len(restart) > 0 {
		fmt.Fprintf(&b, "  restart: {backoff: {%s}}\n", strings.Join(restart, ", "))
	}
	if len(stop) > 0 {
		fmt.Fprintf(&b, "  stop: {%s}\n", strings.Join(stop, ", "))
	}
	b.WriteString("\nthen:\n")
	b.WriteString("  bashy service add -f root.yaml\n")
	b.WriteString("  bashy supervisord --pid1          # the monitor; a container entrypoint\n")
	b.WriteString("  bashy service run root            # or just that unit, in the foreground\n")
	b.WriteString("\nSee docs/bashy-service-design.md (\"A dag root is a unit\").\n")
	return b.String()
}

// childExit is how one run of a unit (or probe) ended.
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

// execChild is one tracked child process: Signal and Kill address the whole
// process group it leads, and are no-ops once the group is empty.
type execChild struct {
	cmd    *exec.Cmd
	reaper *reaper
}

// startTracked starts cmd inside a reaper spawn window (r may be nil), so the
// status of a child the reaper collects first is never lost. Process units and
// the monitor's exec probes spawn through it.
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
