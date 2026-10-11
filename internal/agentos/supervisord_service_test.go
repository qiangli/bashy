// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

// svcRun runs `bashy service ARGS…` in-process, the real cobra tree.
func svcRun(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newServiceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bashy service %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func svcStatus(t *testing.T) map[string]service.Status {
	t.Helper()
	var doc serviceStatusDoc
	if err := json.Unmarshal([]byte(svcRun(t, "status", "--json")), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Schema != ServiceStatusSchema {
		t.Fatalf("schema = %q", doc.Schema)
	}
	out := map[string]service.Status{}
	for _, st := range doc.Units {
		out[st.Unit] = st
	}
	return out
}

func svcWait(t *testing.T, what string, ok func(map[string]service.Status) bool) map[string]service.Status {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last map[string]service.Status
	for time.Now().Before(deadline) {
		if last = svcStatus(t); ok(last) {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; last status %+v", what, last)
	return nil
}

func stateIs(name string, want service.State) func(map[string]service.Status) bool {
	return func(s map[string]service.Status) bool { return s[name].State == want }
}

// fakeOwner plays a process hosting an internal capability (what outpost and
// meet do in S18): it asks the monitor which of ITS units are wanted, applies
// each to the capability it registered through yoke's service.Internal, and
// reports the result. No child process, no pid, no feature flag.
type fakeOwner struct {
	sock string
	prov *service.Internal
	cap  *countingCap
}

type countingCap struct {
	mu                 sync.Mutex
	active             bool
	activations, confs int
	settings           map[string]string
}

func (c *countingCap) Activate(_ context.Context, cfg service.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active, c.settings = true, cfg.Settings
	c.activations++
	return nil
}
func (c *countingCap) Configure(_ context.Context, cfg service.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settings = cfg.Settings
	c.confs++
	return nil
}
func (c *countingCap) Deactivate(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = false
	return nil
}
func (c *countingCap) Health(context.Context) error { return nil }

// sync is one owner pass over the `desired` / `report` protocol.
func (o *fakeOwner) sync(t *testing.T, owner string) {
	t.Helper()
	resp, err := ctlCall(o.sock, ctlRequest{Op: "desired", Owner: owner}, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range resp.Units {
		decl, ok := fleet.New().Service(want.Unit)
		if !ok {
			t.Fatalf("desired names unknown unit %s", want.Unit)
		}
		cur := o.prov.Status(want.Unit).State
		switch {
		case want.Enabled && cur != service.Active:
			if err := o.prov.Activate(context.Background(), decl, service.Runtime{}); err != nil {
				t.Fatal(err)
			}
		case !want.Enabled && cur == service.Active:
			_ = o.prov.Deactivate(context.Background(), want.Unit)
		}
		if want.Enabled {
			if _, err := ctlCall(o.sock, ctlRequest{Op: "report", Status: o.prov.Status(want.Unit)}, 2*time.Second); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// The S16 acceptance: a seeded built-in, an internal capability and a fresh
// custom declaration go through the SAME verbs and the SAME monitor, an
// operator override changes the built-in's argv live, and toggling one unit
// never restarts another. No code below (or in the monitor) names a unit.
func TestServiceMonitorSeedInternalAndCustomShareOneMechanism(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process groups and signals")
	}
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_SERVICES_DIR", filepath.Join(home, "services"))
	t.Setenv("BASHY_SERVICES_PATH", "")
	sock := shortSocket(t)
	t.Setenv("BASHY_SUPERVISORD_SOCKET", sock)
	t.Setenv("BASHY_SUPERVISORD_TEST_CHILD", "hold")

	// Seeded built-in: enabled through the generic toggle, no file copied.
	svcRun(t, "enable", "schedule")
	// Fresh custom (external) declaration.
	custom := filepath.Join(home, "worker.yaml")
	writeUnitFile(t, custom, "name: worker\nkind: service\nexec: ["+os.Args[0]+", worker, --mode, '{settings.mode}']\n"+
		"settings: {mode: fast}\nrestart: {policy: always, backoff: {min: 10ms, max: 40ms}}\nenabled: true\n")
	svcRun(t, "add", "-f", custom)
	// Internal capability hosted by the custom unit's process.
	internal := filepath.Join(home, "cap.yaml")
	writeUnitFile(t, internal, "name: cap\nkind: service\nprovider: internal\ncapability: test.cap\nowner: worker\n"+
		"settings: {level: one}\nenabled: true\n")
	svcRun(t, "add", "-f", internal)

	var logMu sync.Mutex
	var logBuf bytes.Buffer
	m := newServiceMonitor(nil, serviceHome(), writerFunc(func(b []byte) (int, error) {
		logMu.Lock()
		defer logMu.Unlock()
		return logBuf.Write(b)
	}))
	m.pollEvery = 50 * time.Millisecond
	var evMu sync.Mutex
	var events []string
	m.events = func(e service.Event) { evMu.Lock(); events = append(events, e.Unit+":"+e.Kind()); evMu.Unlock() }
	m.notify = func(service.Event) {}
	stop, err := m.serveControl(sock)
	if err != nil {
		t.Fatal(err)
	}
	sigs := make(chan os.Signal, 2)
	done := make(chan int, 1)
	go func() { done <- m.run(sigs) }()
	defer func() {
		stop()
		if t.Failed() {
			logMu.Lock()
			t.Log(logBuf.String())
			logMu.Unlock()
		}
	}()

	st := svcWait(t, "schedule and worker active, cap awaiting its owner", func(s map[string]service.Status) bool {
		return s["schedule"].State == service.Active && s["worker"].State == service.Active && s["cap"].State == service.Starting
	})
	if st["schedule"].PID == 0 || st["worker"].PID == 0 {
		t.Fatalf("process units have no pid: %+v", st)
	}
	if st["meet"].State != service.Inactive || st["meet"].Enabled {
		t.Fatalf("a disabled seed was started: %+v", st["meet"])
	}

	// The owner applies its internal unit; the capability's state IS the unit's.
	reg := service.NewRegistry()
	capImpl := &countingCap{}
	if err := reg.Register("test.cap", capImpl); err != nil {
		t.Fatal(err)
	}
	owner := &fakeOwner{sock: sock, prov: service.NewInternal("worker", reg), cap: capImpl}
	owner.sync(t, "worker")
	st = svcWait(t, "cap active", stateIs("cap", service.Active))
	if st["cap"].PID != 0 || st["cap"].Provider != fleet.ServiceProviderInternal || capImpl.settings["level"] != "one" {
		t.Fatalf("internal unit: %+v settings=%v", st["cap"], capImpl.settings)
	}

	// Output lands in the unit's rotating log; `service logs` reads it.
	waitFile(t, filepath.Join(home, "service", "worker", "output.log"), "child argv: worker --mode fast")
	waitFile(t, filepath.Join(home, "service", "schedule", "output.log"), "child argv: schedule daemon")
	if out := svcRun(t, "logs", "schedule"); !strings.Contains(out, "child argv: schedule daemon") {
		t.Fatalf("service logs schedule:\n%s", out)
	}

	// Operator override of the seed: a sparse overlay that changes its argv.
	// The monitor restarts schedule only; worker keeps its pid.
	workerPID, schedPID := st["worker"].PID, st["schedule"].PID
	svcRun(t, "set", "schedule", "--exec", "bashy", "--exec", "schedule", "--exec", "daemon", "--exec", "--interval=1m")
	st = svcWait(t, "schedule restarted with the override", func(s map[string]service.Status) bool {
		return s["schedule"].State == service.Active && s["schedule"].PID != schedPID
	})
	if st["worker"].PID != workerPID || st["schedule"].Restarts != 1 {
		t.Fatalf("override touched a bystander or miscounted: %+v", st)
	}
	waitFile(t, filepath.Join(home, "service", "schedule", "output.log"), "child argv: schedule daemon --interval=1m")

	// stop / start are runtime verbs over the socket; the dependent internal
	// unit follows its owner down and back up.
	svcRun(t, "stop", "worker")
	svcWait(t, "worker stopped and cap down", func(s map[string]service.Status) bool {
		return s["worker"].State == service.Inactive && s["cap"].State == service.Failed &&
			s["cap"].Reason == "requires worker, which is disabled or stopped"
	})
	if !processGone(workerPID) {
		t.Fatalf("worker pid %d survived stop", workerPID)
	}
	svcRun(t, "start", "worker")
	svcWait(t, "worker back, cap awaiting owner", func(s map[string]service.Status) bool {
		return s["worker"].State == service.Active && s["cap"].State == service.Starting
	})
	owner.prov = service.NewInternal("worker", reg) // the restarted owner process
	owner.sync(t, "worker")
	svcWait(t, "cap active again", stateIs("cap", service.Active))

	// Live disable of the seed: schedule goes, worker stays.
	workerPID = svcStatus(t)["worker"].PID
	svcRun(t, "disable", "schedule")
	st = svcWait(t, "schedule inactive", stateIs("schedule", service.Inactive))
	if st["worker"].PID != workerPID || st["worker"].State != service.Active {
		t.Fatalf("disabling schedule touched worker: %+v", st["worker"])
	}

	// TERM: every unit stops and the monitor exits 0.
	sigs <- os.Interrupt
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("monitor exit = %d", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("monitor did not stop")
	}
	if !processGone(workerPID) {
		t.Fatalf("worker pid %d survived shutdown", workerPID)
	}
	evMu.Lock()
	defer evMu.Unlock()
	joined := strings.Join(events, " ")
	for _, want := range []string{"schedule:service.active", "worker:service.active", "cap:service.active", "worker:service.stopping", "cap:service.failed"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("event %s missing from %v", want, events)
		}
	}
}

func writeUnitFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(path); strings.Contains(string(b), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("%s never contained %q:\n%s", path, want, b)
}
