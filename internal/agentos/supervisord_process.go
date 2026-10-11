// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

// processProvider is the `provider: process` runtime (S16): a unit's exec runs
// as a FOREGROUND child in its own process group, which the monitor owns for
// the child's whole life — no daemonizing, no pidfile, so stop is a signal to
// a group we created and a wait we are guaranteed to get. It is mechanism
// only: whether an exit is restarted, when, and how often is the monitor's
// policy, identical for every unit.
type processProvider struct {
	self    string // what an exec argv[0] of "bashy" runs: this executable
	logRoot string // BASHY_HOME/service; "" = inherit stdout/stderr
	stdout  io.Writer
	stderr  io.Writer
	reaper  *reaper
	now     func() time.Time

	mu    sync.Mutex
	units map[string]*procUnit
}

type procUnit struct {
	child    *execChild
	print    string // runtime fingerprint: argv + env + workdir
	pol      unitPolicy
	status   service.Status
	stopping bool
	done     chan struct{} // closed when the child's wait returned
	log      io.Closer
}

func newProcessProvider(self, logRoot string, r *reaper) *processProvider {
	return &processProvider{self: self, logRoot: logRoot, stdout: os.Stdout, stderr: os.Stderr,
		reaper: r, now: time.Now, units: map[string]*procUnit{}}
}

// Name implements service.Provider.
func (p *processProvider) Name() string { return fleet.ServiceProviderProcess }

// unitCommand is the argv, environment and directory a unit runs with. The
// argv is the declaration's exec with every placeholder expanded; a leading
// `bashy` is this executable, so a monitor started from one bashy never runs
// another found on PATH.
func (p *processProvider) unitCommand(u fleet.Service, rt service.Runtime) (argv, env []string, dir string, err error) {
	argv, err = u.ExpandExec(rt.Download)
	if err != nil {
		return nil, nil, "", err
	}
	if len(argv) > 0 && argv[0] == "bashy" && p.self != "" {
		argv[0] = p.self
	}
	keys := make([]string, 0, len(rt.Env))
	for k := range rt.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+rt.Env[k])
	}
	dir = u.Workdir
	if strings.HasPrefix(dir, "~") {
		if home, herr := os.UserHomeDir(); herr == nil {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
		}
	}
	return argv, env, dir, nil
}

func fingerprint(argv, env []string, dir string) string {
	b, _ := json.Marshal([]any{argv, env, dir})
	return string(b)
}

// Activate implements service.Provider: spawn the unit as the leader of a new
// process group, output to its rotating log.
func (p *processProvider) Activate(_ context.Context, u fleet.Service, rt service.Runtime) error {
	argv, env, dir, err := p.unitCommand(u, rt)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if old, ok := p.units[u.Name]; ok && old.status.State != service.Inactive && old.status.State != service.Failed {
		p.mu.Unlock()
		return fmt.Errorf("service: %s is already running (pid %d)", u.Name, old.status.PID)
	}
	p.mu.Unlock()

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = dir
	cmd.SysProcAttr = ownProcessGroupAttr()
	// A grandchild that keeps the output pipe open must not wedge the wait.
	cmd.WaitDelay = 2 * time.Second
	var logc io.Closer
	if p.logRoot != "" {
		lw, lerr := openUnitLog(p.logRoot, u.Name)
		if lerr != nil {
			return lerr
		}
		cmd.Stdout, cmd.Stderr, logc = lw, lw, lw
		lw.mark(fmt.Sprintf("supervisord: start %s", strings.Join(argv, " ")))
	} else {
		cmd.Stdout, cmd.Stderr = p.stdout, p.stderr
	}
	child, err := startTracked(cmd, p.reaper)
	if err != nil {
		if logc != nil {
			logc.Close()
		}
		return err
	}
	unit := &procUnit{child: child, print: fingerprint(argv, env, dir), pol: policyOf(u), done: make(chan struct{}), log: logc,
		status: service.Status{Unit: u.Name, Provider: p.Name(), Enabled: u.Enabled, State: service.Starting,
			Since: p.now(), PID: cmd.Process.Pid}}
	p.mu.Lock()
	p.units[u.Name] = unit
	p.mu.Unlock()
	go p.wait(u.Name, unit, rt)
	return nil
}

// wait owns the child's one wait. An exit nobody asked for is reported to the
// monitor (To: inactive, the exit in LastExit and Reason); the rest of the
// group is told to go so a straggler cannot hold the port a restart needs.
func (p *processProvider) wait(name string, u *procUnit, rt service.Runtime) {
	ex := u.child.Wait()
	_ = u.child.Signal(syscall.SIGTERM)
	p.mu.Lock()
	code := ex.code
	from := u.status.State
	u.status.LastExit = &code
	u.status.PID = 0
	u.status.State, u.status.Reason, u.status.Since = service.Inactive, ex.String(), p.now()
	stopping := u.stopping
	p.mu.Unlock()
	if lw, ok := u.log.(*rotatingLog); ok {
		lw.mark("supervisord: " + ex.String())
	}
	close(u.done)
	if !stopping && rt.Emit != nil {
		rt.Emit(service.Event{Unit: name, From: from, To: service.Inactive, Reason: ex.String(), At: p.now()})
	}
}

// Configure implements service.Provider: a running process cannot change its
// argv, environment or directory, so any change there needs a restart; a
// change the monitor alone acts on (health, restart, exposure) applies live.
func (p *processProvider) Configure(_ context.Context, u fleet.Service, rt service.Runtime) error {
	argv, env, dir, err := p.unitCommand(u, rt)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cur, ok := p.units[u.Name]
	if !ok {
		return nil
	}
	cur.pol = policyOf(u)
	cur.status.Enabled = u.Enabled
	if cur.print != fingerprint(argv, env, dir) {
		return service.ErrRestartRequired
	}
	return nil
}

// Deactivate implements service.Provider: the declared stop signal to the
// whole group, the declared grace, then KILL. Idempotent; returns once the
// child has been waited for.
func (p *processProvider) Deactivate(ctx context.Context, name string) error {
	p.mu.Lock()
	u, ok := p.units[name]
	if !ok {
		p.mu.Unlock()
		return nil
	}
	u.stopping = true
	running := u.status.State != service.Inactive && u.status.State != service.Failed
	if running {
		u.status.State = service.Stopping
	}
	pol := u.pol
	p.mu.Unlock()
	if running {
		if err := u.child.Signal(pol.stopSignal); err != nil {
			_ = u.child.Kill()
		}
		select {
		case <-u.done:
		case <-time.After(pol.grace):
		case <-ctx.Done():
		}
	}
	_ = u.child.Kill() // whatever is left of the group; a no-op when empty
	<-u.done
	if u.log != nil {
		u.log.Close()
	}
	p.mu.Lock()
	if p.units[name] == u {
		u.status.State, u.status.PID = service.Inactive, 0
	}
	p.mu.Unlock()
	return nil
}

// Health implements service.Provider: the provider-level liveness under the
// declared probe — the child is still running.
func (p *processProvider) Health(_ context.Context, name string) error {
	p.mu.Lock()
	u, ok := p.units[name]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("service: %s is not running", name)
	}
	select {
	case <-u.done:
		return fmt.Errorf("service: %s exited (%s)", name, u.status.Reason)
	default:
		return nil
	}
}

// Status implements service.Provider.
func (p *processProvider) Status(name string) service.Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	if u, ok := p.units[name]; ok {
		return u.status
	}
	return service.Status{Unit: name, Provider: p.Name(), State: service.Inactive}
}

// ─── rotating unit logs ───────────────────────────────────────────────────

const (
	unitLogMax  = 10 << 20 // bytes before rotation
	unitLogKeep = 3        // rotated generations kept: output.log.1 … .3
	unitLogName = "output.log"
)

// unitLogDir is BASHY_HOME/service/NAME, where a unit's output lives.
func unitLogDir(root, name string) string { return filepath.Join(root, name) }

// rotatingLog is a size-bounded append log: at unitLogMax the file becomes
// output.log.1 (older generations shift up, the oldest is dropped) and a new
// one starts. Child stdout and stderr share it, in write order.
type rotatingLog struct {
	mu   sync.Mutex
	path string
	max  int64
	keep int
	f    *os.File
	size int64
}

func openUnitLog(root, name string) (*rotatingLog, error) {
	dir := unitLogDir(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	l := &rotatingLog{path: filepath.Join(dir, unitLogName), max: unitLogMax, keep: unitLogKeep}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *rotatingLog) open() error {
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	st, _ := f.Stat()
	l.f, l.size = f, 0
	if st != nil {
		l.size = st.Size()
	}
	return nil
}

func (l *rotatingLog) rotate() error {
	l.f.Close()
	for i := l.keep; i >= 1; i-- {
		src := l.path
		if i > 1 {
			src = fmt.Sprintf("%s.%d", l.path, i-1)
		}
		_ = os.Rename(src, fmt.Sprintf("%s.%d", l.path, i))
	}
	return l.open()
}

func (l *rotatingLog) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return 0, os.ErrClosed
	}
	if l.size > 0 && l.size+int64(len(b)) > l.max {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(b)
	l.size += int64(n)
	return n, err
}

// mark writes one supervisor line into the unit's own log, so `service logs`
// shows the starts and exits between the unit's output.
func (l *rotatingLog) mark(s string) {
	fmt.Fprintf(l, "%s %s\n", time.Now().UTC().Format(time.RFC3339), s)
}

func (l *rotatingLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}
