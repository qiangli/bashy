// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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

// ─── policy primitives ────────────────────────────────────────────────────

func TestBackoffLadderDoublesCapsAndResetsAfterHealthyRun(t *testing.T) {
	b := newBackoffLadder(time.Second, 8*time.Second, 30*time.Second)
	var got []time.Duration
	for range 5 {
		got = append(got, b.delay(0))
	}
	got = append(got, b.delay(31*time.Second), b.delay(0))
	want := []time.Duration{1, 2, 4, 8, 8, 1, 2}
	for i := range want {
		if got[i] != want[i]*time.Second {
			t.Fatalf("delays = %v, want %v seconds", got, want)
		}
	}
}

func TestStartLimiterRefusesBeyondBurstWithinWindow(t *testing.T) {
	l := startLimiter{burst: 3, window: time.Minute}
	t0 := time.Unix(1000, 0)
	for i := range 3 {
		if !l.allow(t0.Add(time.Duration(i) * time.Second)) {
			t.Fatalf("start %d refused inside the burst", i+1)
		}
	}
	if l.allow(t0.Add(10 * time.Second)) {
		t.Fatal("4th start within the window was allowed")
	}
	if !l.allow(t0.Add(61 * time.Second)) {
		t.Fatal("a start after the window slid past the first was refused")
	}
}

func TestPolicyDefaultsAndRestartDecision(t *testing.T) {
	p := policyOf(fleet.Service{Name: "x", Exec: []string{"x"}})
	if p.restart != fleet.RestartOnFailure || p.burst != unitDefaultStartBurst || p.healthFailures != unitDefaultHealthFailures ||
		p.minBackoff != supervisordMinBackoff || p.grace != supervisordGrace {
		t.Fatalf("defaults: %+v", p)
	}
	if p.wantsRestart(childExit{code: 0}) || !p.wantsRestart(childExit{code: 3}) || !p.wantsRestart(childExit{code: 143, signaled: true}) {
		t.Fatal("on-failure must restart failures only")
	}
	p.restart = fleet.RestartAlways
	if !p.wantsRestart(childExit{code: 0}) {
		t.Fatal("always must restart a clean exit")
	}
	p.restart = fleet.RestartNo
	if p.wantsRestart(childExit{code: 1}) {
		t.Fatal("no must never restart")
	}
}

// ─── a scripted provider ──────────────────────────────────────────────────

// fakeProvider is a process-shaped provider with no processes: the test
// decides when a unit exits and whether it starts at all.
type fakeProvider struct {
	name string

	mu       sync.Mutex
	acts     map[string]int
	deacts   map[string]int
	confs    map[string]int
	alive    map[string]bool
	exit     map[string]*int
	rts      map[string]service.Runtime
	crash    map[string]int // exit code to die with right after every start
	confErr  error
	pid      int
	activate []string // activation order
}

func newFakeProvider(name string) *fakeProvider {
	return &fakeProvider{name: name, acts: map[string]int{}, deacts: map[string]int{}, confs: map[string]int{},
		alive: map[string]bool{}, exit: map[string]*int{}, rts: map[string]service.Runtime{}, crash: map[string]int{}, pid: 4000}
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Activate(_ context.Context, u fleet.Service, rt service.Runtime) error {
	f.mu.Lock()
	f.acts[u.Name]++
	f.activate = append(f.activate, u.Name)
	f.alive[u.Name] = true
	f.rts[u.Name] = rt
	f.pid++
	code, crash := f.crash[u.Name]
	f.mu.Unlock()
	if crash {
		go f.die(u.Name, code)
	}
	return nil
}
func (f *fakeProvider) die(name string, code int) {
	f.mu.Lock()
	f.alive[name] = false
	f.exit[name] = &code
	rt := f.rts[name]
	f.mu.Unlock()
	rt.Emit(service.Event{Unit: name, From: service.Starting, To: service.Inactive, Reason: fmt.Sprintf("exit %d", code)})
}
func (f *fakeProvider) Configure(_ context.Context, u fleet.Service, _ service.Runtime) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.confs[u.Name]++
	return f.confErr
}
func (f *fakeProvider) Deactivate(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deacts[name]++
	f.alive[name] = false
	return nil
}
func (f *fakeProvider) Health(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.alive[name] {
		return errors.New("not alive")
	}
	return nil
}
func (f *fakeProvider) Status(name string) service.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := service.Status{Unit: name, Provider: f.name, LastExit: f.exit[name]}
	if f.alive[name] {
		st.PID = f.pid
	}
	return st
}
func (f *fakeProvider) count(m map[string]int, name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return m[name]
}

// unitSet is a mutable registry the test edits between reloads.
type unitSet struct {
	mu    sync.Mutex
	units map[string]fleet.Service
}

func (s *unitSet) load() ([]fleet.Service, []error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []fleet.Service
	for _, u := range s.units {
		out = append(out, u)
	}
	return out, nil
}
func (s *unitSet) put(u fleet.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.units[u.Name] = u
}

type eventLog struct {
	mu       sync.Mutex
	events   []service.Event
	notified []service.Event
}

func (l *eventLog) kinds(unit string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, e := range l.events {
		if e.Unit == unit {
			out = append(out, e.Kind())
		}
	}
	return out
}

// testMonitor runs a monitor over set with prov as the process provider.
func testMonitor(t *testing.T, set *unitSet, prov service.Provider) (*monitor, *eventLog, chan os.Signal) {
	t.Helper()
	m := newMonitor()
	m.load = set.load
	m.providers = service.Providers{fleet.ServiceProviderProcess: prov}
	m.checkPorts = nil
	el := &eventLog{}
	m.events = func(e service.Event) { el.mu.Lock(); el.events = append(el.events, e); el.mu.Unlock() }
	m.notify = func(e service.Event) { el.mu.Lock(); el.notified = append(el.notified, e); el.mu.Unlock() }
	var logMu sync.Mutex
	var logBuf strings.Builder
	m.log = writerFunc(func(b []byte) (int, error) { logMu.Lock(); defer logMu.Unlock(); return logBuf.Write(b) })
	sigs := make(chan os.Signal, 2)
	done := make(chan int, 1)
	go func() { done <- m.run(sigs) }()
	t.Cleanup(func() {
		sigs <- os.Interrupt
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("monitor did not shut down")
		}
		if t.Failed() {
			logMu.Lock()
			t.Log(logBuf.String())
			logMu.Unlock()
		}
	})
	return m, el, sigs
}

type writerFunc func([]byte) (int, error)

func (w writerFunc) Write(b []byte) (int, error) { return w(b) }

func ctl(t *testing.T, m *monitor, req ctlRequest) ctlResponse {
	t.Helper()
	reply := make(chan ctlResponse, 1)
	m.msgs <- msgControl{req: req, reply: reply}
	return <-reply
}

func unitStatus(t *testing.T, m *monitor, name string) service.Status {
	t.Helper()
	r := ctl(t, m, ctlRequest{Op: "status", Unit: name})
	if len(r.Units) != 1 {
		return service.Status{}
	}
	return r.Units[0]
}

func waitState(t *testing.T, m *monitor, name string, want service.State) service.Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var st service.Status
	for time.Now().Before(deadline) {
		if st = unitStatus(t, m, name); st.State == want {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never reached %s; last %+v", name, want, st)
	return st
}

func testUnit(name string, enabled bool) fleet.Service {
	return fleet.Service{Name: name, Kind: fleet.KindService, Exec: []string{"true"}, Enabled: enabled,
		Restart: fleet.ServiceRestart{Backoff: fleet.ServiceBackoff{Min: "1ms", Max: "4ms", HealthyAfter: "1h"},
			StartLimit: fleet.ServiceStartLim{Burst: 50, Interval: "1m"}},
		Health: fleet.ServiceHealth{Interval: "20ms"}}
}

// ─── acceptance: the monitor's policy ─────────────────────────────────────

// A unit that dies on every start is restarted with backoff until the start
// limit, then left failed — with a reason, a service.failed event and one
// notification — and never started again on its own.
func TestMonitorStartLimitMovesCrashLoopToFailed(t *testing.T) {
	u := testUnit("crasher", true)
	u.Restart.StartLimit = fleet.ServiceStartLim{Burst: 3, Interval: "1m"}
	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	prov.crash["crasher"] = 2
	m, el, _ := testMonitor(t, set, prov)
	st := waitState(t, m, "crasher", service.Failed)
	if !strings.Contains(st.Reason, "start limit hit: 3 starts within 1m0s") {
		t.Fatalf("reason = %q", st.Reason)
	}
	if st.Restarts != 2 || st.LastExit == nil || *st.LastExit != 2 {
		t.Fatalf("restarts/last exit: %+v", st)
	}
	time.Sleep(50 * time.Millisecond)
	if n := prov.count(prov.acts, "crasher"); n != 3 {
		t.Fatalf("activations = %d, want exactly the burst (3)", n)
	}
	el.mu.Lock()
	notified := len(el.notified)
	el.mu.Unlock()
	if notified != 1 {
		t.Fatalf("failed must notify once, got %d", notified)
	}
	if k := el.kinds("crasher"); k[len(k)-1] != "service.failed" {
		t.Fatalf("events = %v", k)
	}
	// An explicit start is the operator's answer: the limit is reset.
	ctl(t, m, ctlRequest{Op: "start", Unit: "crasher"})
	waitState(t, m, "crasher", service.Failed)
	if n := prov.count(prov.acts, "crasher"); n != 6 {
		t.Fatalf("after start: activations = %d, want 6", n)
	}
}

// Consecutive probe failures past health.failures restart the unit; a
// passing probe keeps it active. The probe verdict is an error, never text.
func TestMonitorHealthFailureRestartsWithProbe(t *testing.T) {
	u := testUnit("web", true)
	u.Health = fleet.ServiceHealth{TCP: true, Interval: "10ms", Failures: 2}
	u.Ports = []fleet.ServicePort{{Port: 1}}
	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	var mu sync.Mutex
	fail := false
	m := newMonitor()
	m.checkPorts = nil
	m.probe = func(_ context.Context, _ fleet.Service, _ unitPolicy) (error, bool) {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			return errors.New("tcp 127.0.0.1:1: connection refused"), true
		}
		return nil, true
	}
	runMonitor(t, m, set, prov)
	st := waitState(t, m, "web", service.Active)
	if st.Health != "ok" {
		t.Fatalf("health = %q", st.Health)
	}
	mu.Lock()
	fail = true
	mu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for prov.count(prov.acts, "web") < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	fail = false
	mu.Unlock()
	st = waitState(t, m, "web", service.Active)
	if prov.count(prov.deacts, "web") < 1 || st.Restarts < 1 {
		t.Fatalf("a failing probe must restart the unit: deacts=%d %+v", prov.count(prov.deacts, "web"), st)
	}
}

// runMonitor starts an already-built monitor (so a test can set fields first).
func runMonitor(t *testing.T, m *monitor, set *unitSet, prov service.Provider) *eventLog {
	t.Helper()
	m.load = set.load
	if m.providers == nil {
		m.providers = service.Providers{fleet.ServiceProviderProcess: prov}
	}
	el := &eventLog{}
	m.events = func(e service.Event) { el.mu.Lock(); el.events = append(el.events, e); el.mu.Unlock() }
	m.notify = func(e service.Event) { el.mu.Lock(); el.notified = append(el.notified, e); el.mu.Unlock() }
	sigs := make(chan os.Signal, 2)
	done := make(chan int, 1)
	go func() { done <- m.run(sigs) }()
	t.Cleanup(func() {
		sigs <- os.Interrupt
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("monitor did not shut down")
		}
	})
	return el
}

// Something the monitor does not own listening on a declared port: the unit
// is failed with that reason, never started, never retried.
func TestMonitorForeignListenerOnUnitPortIsFailedWithReason(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	u := testUnit("squatted", true)
	u.Ports = []fleet.ServicePort{{Name: "http", Port: port}}
	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	m := newMonitor() // the real port check
	el := runMonitor(t, m, set, prov)
	st := waitState(t, m, "squatted", service.Failed)
	want := fmt.Sprintf("port %d/tcp (127.0.0.1:%d) is held by a foreign listener", port, port)
	if st.Reason != want {
		t.Fatalf("reason = %q, want %q", st.Reason, want)
	}
	time.Sleep(30 * time.Millisecond)
	if n := prov.count(prov.acts, "squatted"); n != 0 {
		t.Fatalf("a squatted unit was activated %d times", n)
	}
	el.mu.Lock()
	defer el.mu.Unlock()
	if len(el.notified) != 1 || el.notified[0].Reason != want {
		t.Fatalf("notify = %+v", el.notified)
	}
}

// enable / disable / set reach a running monitor as a reload; only the unit
// that changed moves, and no bystander is restarted or reconfigured.
func TestMonitorLiveEnableDisableLeavesOtherUnitsAlone(t *testing.T) {
	a, b := testUnit("a", true), testUnit("b", false)
	set := &unitSet{units: map[string]fleet.Service{"a": a, "b": b}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	m, _, _ := testMonitor(t, set, prov)
	pidA := waitState(t, m, "a", service.Active).PID
	if st := unitStatus(t, m, "b"); st.State != service.Inactive || prov.count(prov.acts, "b") != 0 {
		t.Fatalf("disabled b started: %+v", st)
	}

	b.Enabled = true
	set.put(b)
	ctl(t, m, ctlRequest{Op: "reload"})
	waitState(t, m, "b", service.Active)

	a.Enabled = false
	set.put(a)
	ctl(t, m, ctlRequest{Op: "reload"})
	waitState(t, m, "a", service.Inactive)
	if prov.count(prov.acts, "a") != 1 || prov.count(prov.confs, "a") != 0 {
		t.Fatalf("a: acts=%d confs=%d", prov.count(prov.acts, "a"), prov.count(prov.confs, "a"))
	}
	if prov.count(prov.acts, "b") != 1 || prov.count(prov.deacts, "b") != 0 || prov.count(prov.confs, "b") != 0 {
		t.Fatalf("disabling a touched b: acts=%d deacts=%d confs=%d", prov.count(prov.acts, "b"), prov.count(prov.deacts, "b"), prov.count(prov.confs, "b"))
	}
	_ = pidA

	// A declaration change the provider cannot apply live restarts that unit
	// only, and does not count against its start limit.
	prov.confErr = service.ErrRestartRequired
	b.Exec = []string{"true", "--changed"}
	set.put(b)
	ctl(t, m, ctlRequest{Op: "reload"})
	deadline := time.Now().Add(5 * time.Second)
	for prov.count(prov.acts, "b") < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if st := waitState(t, m, "b", service.Active); st.Restarts != 1 || prov.count(prov.confs, "b") != 1 {
		t.Fatalf("reconfigure: %+v confs=%d", st, prov.count(prov.confs, "b"))
	}
	if prov.count(prov.acts, "a") != 1 {
		t.Fatal("reconfiguring b restarted a")
	}
}

// requires orders starts: a dependent starts only once what it requires is
// active, and a dependent of a disabled unit is failed with that reason.
func TestMonitorRequiresOrdering(t *testing.T) {
	db, app := testUnit("db", true), testUnit("app", true)
	app.Requires = []string{"db"}
	set := &unitSet{units: map[string]fleet.Service{"db": db, "app": app}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	m, _, _ := testMonitor(t, set, prov)
	waitState(t, m, "app", service.Active)
	prov.mu.Lock()
	order := append([]string(nil), prov.activate...)
	prov.mu.Unlock()
	if strings.Join(order, ",") != "db,app" {
		t.Fatalf("activation order = %v", order)
	}
	db.Enabled = false
	set.put(db)
	ctl(t, m, ctlRequest{Op: "reload"})
	st := waitState(t, m, "app", service.Failed)
	if st.Reason != "requires db, which is disabled or stopped" {
		t.Fatalf("reason = %q", st.Reason)
	}
}

// ─── status --json schema golden ──────────────────────────────────────────

// The status document's shape is a contract outpost's exposure reconciler,
// the admin UI and MCP read. Regenerate with BASHY_UPDATE_GOLDEN=1.
func TestServiceStatusJSONGolden(t *testing.T) {
	sock := shortSocket(t)
	at := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	m := newMonitor()
	// The fake provider does not bind ports. Keep the fixed golden ports
	// independent of listeners elsewhere on the test host.
	m.checkPorts = nil
	m.now = func() time.Time { return at }
	web := testUnit("web", true)
	web.Ports = []fleet.ServicePort{{Name: "http", Port: 18080}, {Name: "admin", Port: 18081, Bind: "lan"}}
	web.Expose = fleet.ServiceExpose{App: &fleet.ServiceApp{RequireLogin: true}, Mesh: []fleet.ServiceMesh{{Name: "web"}}}
	bad := testUnit("bad", true)
	bad.Restart.StartLimit = fleet.ServiceStartLim{Burst: 2, Interval: "1m"}
	off := testUnit("off", false)
	set := &unitSet{units: map[string]fleet.Service{"web": web, "bad": bad, "off": off}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	prov.crash["bad"] = 3
	runMonitor(t, m, set, prov)
	stop, err := m.serveControl(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	waitState(t, m, "web", service.Active)
	waitState(t, m, "bad", service.Failed)
	doc, err := serviceStatus(sock, nil, set.load)
	if err != nil {
		t.Fatal(err)
	}
	doc.Monitor.PID, doc.Monitor.Socket = 1234, "/run/bashy/service/supervisord.sock"
	for i := range doc.Units {
		if doc.Units[i].PID != 0 {
			doc.Units[i].PID = 4242
		}
	}
	got, _ := json.MarshalIndent(doc, "", "  ")
	got = append(got, '\n')
	golden := filepath.Join("testdata", "service-status.golden.json")
	if os.Getenv("BASHY_UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("status --json drifted from %s:\n%s", golden, got)
	}

	// Without a monitor the same document reports desired state, inactive.
	stop()
	doc, err = serviceStatus(sock, []string{"web"}, set.load)
	if err != nil || doc.Monitor.Running || len(doc.Units) != 1 || doc.Units[0].State != service.Inactive ||
		doc.Units[0].Reason != "supervisord is not running" || !doc.Units[0].Enabled {
		t.Fatalf("no-monitor status: %+v %v", doc, err)
	}
}

// shortSocket is a unix socket path short enough for macOS's 104 bytes.
func shortSocket(t *testing.T) string {
	t.Helper()
	base := "/tmp"
	if runtime.GOOS == "windows" {
		base = os.TempDir()
	}
	dir, err := os.MkdirTemp(base, "sv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}
