// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

// The Sprint 218 `bashy supervisord DAG.md TARGET` contract — restart with
// bounded backoff, TERM/INT forwarding with a bounded grace, PID 1 reaping —
// is now unit semantics under the one monitor (Sprint 418 S19). These tests
// are the old supervisor's tests ported onto a unit whose exec is
// `bashy dag DAG.md TARGET`: fake provider and fake clock for the policy,
// re-exec'd copies of this test binary for the real processes.

// unitClock is the monitor's clock, moved by the test so a run's length is a
// stated fact.
type unitClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *unitClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *unitClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// waitRunning waits for the nth incarnation of name to be up as far as the
// monitor can see, so a scripted exit cannot arrive before it is tracked.
func waitRunning(t *testing.T, m *monitor, prov *fakeProvider, name string, n int) {
	t.Helper()
	waitUntil(t, fmt.Sprintf("%s incarnation %d running", name, n), func() bool {
		if prov.count(prov.acts, name) < n {
			return false
		}
		switch unitStatus(t, m, name).State {
		case service.Starting, service.Active, service.Degraded:
			return true
		}
		return false
	})
}

// restartDelays are the "restarting in D" delays of name's transitions, in order.
func restartDelays(el *eventLog, name string) []string {
	el.mu.Lock()
	defer el.mu.Unlock()
	var out []string
	for _, e := range el.events {
		if _, d, ok := strings.Cut(e.Reason, "restarting in "); ok && e.Unit == name {
			out = append(out, d)
		}
	}
	return out
}

// dagRootUnit is the unit that replaces `bashy supervisord DAG.md TARGET`.
func dagRootUnit(dag, target string) fleet.Service {
	return fleet.Service{Name: "root", Kind: fleet.KindService, Enabled: true,
		Exec: []string{"bashy", "dag", dag, target},
		Restart: fleet.ServiceRestart{Backoff: fleet.ServiceBackoff{Min: "10ms", Max: "40ms", HealthyAfter: "1h"},
			StartLimit: fleet.ServiceStartLim{Burst: 50, Interval: "1m"}},
		Stop: fleet.ServiceStop{Timeout: "500ms"}}
}

// ─── policy, with no processes and no waiting ─────────────────────────────

// A clean exit completes the unit: it is not restarted, killed again or
// notified, and stays down.
func TestDagRootUnitCompletesOnExitZero(t *testing.T) {
	u := testUnit("root", true)
	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	prov.crash["root"] = 0
	m, el, _ := testMonitor(t, set, prov)
	waitUntil(t, "root completed", func() bool {
		st := unitStatus(t, m, "root")
		return st.State == service.Inactive && strings.Contains(st.Reason, "completed (")
	})
	time.Sleep(50 * time.Millisecond) // longer than the backoff: a restart would show
	if n := prov.count(prov.acts, "root"); n != 1 {
		t.Fatalf("a completed root was started %d times", n)
	}
	el.mu.Lock()
	notified := len(el.notified)
	el.mu.Unlock()
	if notified != 0 {
		t.Fatalf("a completed root must not notify, got %d", notified)
	}
}

// Crash, crash, crash: the delay doubles from min and stops at max; a run
// that lasts healthy_after puts the ladder back at min. Every exit's group is
// cleaned up before the replacement starts. No real time passes in the run.
func TestDagRootUnitBackoffDoublesCapsAndResetsAfterHealthyRun(t *testing.T) {
	u := testUnit("root", true)
	u.Restart.Backoff = fleet.ServiceBackoff{Min: "1ms", Max: "8ms", HealthyAfter: "30s"}
	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	clk := &unitClock{t: time.Unix(1_800_000_000, 0)}
	m := newMonitor()
	m.now = clk.now
	m.checkPorts = nil
	el := runMonitor(t, m, set, prov)

	// Runs 1-5 crash instantly; run 6 is healthy for 31s then is killed;
	// run 7 completes.
	for i := 1; i <= 5; i++ {
		waitRunning(t, m, prov, "root", i)
		prov.die("root", i)
	}
	waitRunning(t, m, prov, "root", 6)
	clk.advance(31 * time.Second)
	prov.die("root", 137)
	waitRunning(t, m, prov, "root", 7)
	prov.die("root", 0)
	waitUntil(t, "root completed", func() bool {
		return strings.Contains(unitStatus(t, m, "root").Reason, "completed (")
	})

	want := []string{"1ms", "2ms", "4ms", "8ms", "8ms", "1ms"}
	if got := restartDelays(el, "root"); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("backoff sequence = %v, want %v", got, want)
	}
	if n := prov.count(prov.deacts, "root"); n != 7 {
		t.Fatalf("Deactivate (TERM then KILL of the old group) ran %d times, want once per exit (7)", n)
	}
}

// A signal while a unit sleeps out its backoff ends the monitor now: no
// restart, and no waiting for the delay.
func TestMonitorSignalDuringBackoffExitsWithoutRestarting(t *testing.T) {
	u := testUnit("root", true)
	u.Restart.Backoff = fleet.ServiceBackoff{Min: "1h", Max: "1h", HealthyAfter: "1h"}
	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	prov.crash["root"] = 2
	m := newMonitor()
	m.checkPorts = nil
	m.load = set.load
	m.providers = service.Providers{fleet.ServiceProviderProcess: prov}
	var log lockedBuffer
	m.log = &log
	sigs := make(chan os.Signal, 1)
	done := make(chan int, 1)
	go func() { done <- m.run(sigs) }()
	waitUntil(t, "root sleeping out its backoff", func() bool {
		return strings.Contains(unitStatus(t, m, "root").Reason, "restarting in 1h")
	})
	sigs <- syscall.SIGTERM
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (a stop is not a failure)\n%s", code, log.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("the signal did not end the backoff wait\n%s", log.String())
	}
	if n := prov.count(prov.acts, "root"); n != 1 {
		t.Fatalf("root was started %d times, want 1", n)
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// ─── the retired form is refused, with the unit that replaces it ──────────

func TestSupervisordRefusesTheRetiredDAGForm(t *testing.T) {
	for _, args := range [][]string{
		{"DAG.md", "svc"},
		{"DAG.md"},
		{"DAG.md", "a", "b"},
		{"--grace", "5s", "DAG.md", "svc"},
		{"--min-backoff=2s", "--max-backoff=8s", "--healthy-after", "1m", "DAG.md", "svc"},
		{"--pid1", "DAG.md", "svc"},
		{"--no-such", "DAG.md", "a"},
		{"--", "DAG.md", "svc"},
		{"--grace=1s"},
	} {
		if retiredDAGForm(args) == nil {
			t.Errorf("supervisord %v: not recognised as the retired form", args)
		}
		if code := dispatchSupervisord(args); code != 2 {
			t.Errorf("supervisord %v: exit %d, want 2", args, code)
		}
	}
	for _, args := range [][]string{nil, {"--pid1"}, {"--until-done"}, {"--socket", "/x/s.sock"}, {"--socket=/x/s.sock", "--poll", "1s"}} {
		if retiredDAGForm(args) != nil {
			t.Errorf("supervisord %v: the monitor's own flags were refused as the retired form", args)
		}
	}
}

func TestSupervisordRefusalNamesTheUnitForm(t *testing.T) {
	msg := retiredDAGForm([]string{"--grace", "5s", "--min-backoff", "2s", "ops/DAG.md", "serve"}).message()
	for _, want := range []string{
		"the `DAG.md TARGET` form was retired",
		"exec: [bashy, dag, ops/DAG.md, serve]",
		"restart: {backoff: {min: 2s}}",
		"stop: {timeout: 5s}",
		"bashy service add -f root.yaml",
		"bashy supervisord --pid1",
		"bashy service run root",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal lacks %q:\n%s", want, msg)
		}
	}
	// The unit it prints is a valid declaration, with the flags carried over.
	var yaml []string
	for _, l := range strings.Split(msg, "\n") {
		if strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "  # ") && !strings.HasPrefix(l, "  bashy ") {
			yaml = append(yaml, strings.TrimPrefix(l, "  "))
		}
	}
	u, err := fleet.ParseService("root", []byte(strings.Join(yaml, "\n")), nil)
	if err != nil {
		t.Fatalf("the printed unit does not parse: %v\n%s", err, strings.Join(yaml, "\n"))
	}
	if err := u.Validate(); err != nil {
		t.Fatalf("the printed unit is invalid: %v", err)
	}
	if !u.Enabled || fmt.Sprint(u.Exec) != "[bashy dag ops/DAG.md serve]" ||
		u.Restart.Backoff.Min != "2s" || u.Stop.Timeout != "5s" {
		t.Fatalf("printed unit lost the operands: %+v", u)
	}
}

func TestSupervisordHelpAndFlagErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--help"}, 0},
		{[]string{"-h"}, 0},
		{[]string{"--no-such"}, 2},
		{[]string{"--poll"}, 2},
	} {
		if code := dispatchSupervisord(tc.args); code != tc.code {
			t.Errorf("supervisord %v: exit %d, want %d", tc.args, code, tc.code)
		}
	}
}

// ─── the real thing: this test binary stands in for `bashy dag` ───────────

// supervisordTestChild is what the re-exec'd test binary runs (main_test.go).
// It is the dag root a unit execs (`bashy dag FILE TARGET`), scripted by
// BASHY_SUPERVISORD_TEST_CHILD:
//
//	exit:N            print argv, exit N
//	count:FILE        append one line to FILE, exit 1 (until FILE has 3 lines: exit 0)
//	hold              print argv, ignore nothing, block until signaled
//	ignore-term       block; on TERM keep going (only KILL ends it)
//	orphan:FILE       spawn a grandchild that outlives us, write its pid to FILE, exit 0
//	listen:PORT       listen on 127.0.0.1:PORT and block
//	monitor:ROOT,PORT run a service monitor (logs and ownership under ROOT)
//	                  over one unit that is this binary in listen:PORT mode
func supervisordTestChild() int {
	script := os.Getenv("BASHY_SUPERVISORD_TEST_CHILD")
	args := os.Args[1:]
	fmt.Printf("child argv: %s\n", strings.Join(args, " "))
	mode, arg, _ := strings.Cut(script, ":")
	switch mode {
	case "exit":
		n, _ := strconv.Atoi(arg)
		return n
	case "count":
		f, _ := os.OpenFile(arg, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		fmt.Fprintln(f, "run")
		f.Close()
		b, _ := os.ReadFile(arg)
		if strings.Count(string(b), "\n") >= 3 {
			return 0
		}
		return 1
	case "hold", "ignore-term":
		if mode == "ignore-term" {
			signalIgnoreTerm()
		}
		fmt.Println("child holding")
		select {}
	case "orphan":
		gc := exec.Command(os.Args[0], "grandchild")
		gc.Env = append(os.Environ(), "BASHY_SUPERVISORD_TEST_CHILD=grandchild")
		r, w, _ := os.Pipe()
		gc.Stdin = r
		if err := gc.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		r.Close()
		_ = os.WriteFile(arg, []byte(strconv.Itoa(gc.Process.Pid)), 0o644)
		_ = w // held open until we exit: our exit is the grandchild's EOF
		return 0
	case "grandchild":
		_, _ = io.ReadAll(os.Stdin) // EOF when the parent is gone
		return 0
	case "listen":
		l, err := net.Listen("tcp", "127.0.0.1:"+arg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		defer l.Close()
		select {}
	case "monitor":
		root, port, _ := strings.Cut(arg, ",")
		m := newMonitor()
		m.load = func() ([]fleet.Service, []error) { return []fleet.Service{listenerUnit(port)}, nil }
		m.providers = service.Providers{fleet.ServiceProviderProcess: newProcessProvider(os.Args[0], root, nil)}
		m.resolve = resolveUnitRuntime
		m.log = os.Stderr
		return m.run(make(chan os.Signal))
	}
	fmt.Fprintln(os.Stderr, "unknown script", script)
	return 9
}

// listenerUnit is a process unit that holds port: this binary in listen mode.
func listenerUnit(port string) fleet.Service {
	n, _ := strconv.Atoi(port)
	u := testUnit("listener", true)
	u.Exec = []string{os.Args[0], "listen"}
	u.Env = map[string]string{"BASHY_SUPERVISORD_TEST_CHILD": "listen:" + port}
	u.Ports = []fleet.ServicePort{{Port: n}}
	return u
}

func dagFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	md := "## Tasks\n\n### a\nA.\n\n```bash\necho A\n```\n\n### b\nB.\n\nRequires: a\n\n```bash\necho B\n```\n\n### svc\nRoot.\n\nRequires: b a\n\n```bash\necho SVC\n```\n"
	p := filepath.Join(dir, "DAG.md")
	if err := os.WriteFile(p, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// realUnit is a monitor driving the dag-root unit against re-exec'd copies of
// this test binary, the unit's stdout going to out.
type realUnit struct {
	m    *monitor
	prov *processProvider
	out  *os.File
	log  *lockedBuffer
	sigs chan os.Signal
	done chan int
	dag  string
}

// startRealUnit runs the monitor over one dag-root unit (mutate edits it).
// With untilDone the monitor returns when the root settles, like the
// container entrypoint; otherwise it runs until signaled. r may be nil.
func startRealUnit(t *testing.T, script string, untilDone bool, r *reaper, mutate func(*fleet.Service)) *realUnit {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	// Windows refuses to remove a file that is still open: without this the
	// TempDir cleanup fails the test after every assertion has passed.
	t.Cleanup(func() { out.Close() })
	t.Setenv("BASHY_SUPERVISORD_TEST_CHILD", script)
	dag := dagFixture(t)
	u := dagRootUnit(dag, "svc")
	if mutate != nil {
		mutate(&u)
	}
	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	prov := newProcessProvider(os.Args[0], "", r)
	prov.stdout, prov.stderr = out, out
	m := newMonitor()
	m.checkPorts = nil
	m.load = set.load
	m.providers = service.Providers{fleet.ServiceProviderProcess: prov}
	m.untilDone = untilDone
	if r != nil {
		m.probe = func(ctx context.Context, u fleet.Service, p unitPolicy) (error, bool) { return runProbe(ctx, u, p, r) }
	}
	ru := &realUnit{m: m, prov: prov, out: out, log: &lockedBuffer{}, sigs: make(chan os.Signal, 2), done: make(chan int, 1), dag: dag}
	m.log = ru.log
	go func() { ru.done <- m.run(ru.sigs) }()
	t.Cleanup(func() {
		select {
		case <-ru.done:
		default:
			ru.sigs <- os.Interrupt
			select {
			case <-ru.done:
			case <-time.After(10 * time.Second):
				t.Error("monitor did not shut down")
			}
		}
	})
	return ru
}

func (ru *realUnit) wait(t *testing.T) int {
	t.Helper()
	select {
	case code := <-ru.done:
		ru.done <- code
		return code
	case <-time.After(30 * time.Second):
		t.Fatalf("monitor did not return\n%s\n%s", ru.log.String(), readAll(t, ru.out))
		return -1
	}
}

func readAll(t *testing.T, f *os.File) string {
	t.Helper()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func waitForOutput(t *testing.T, f *os.File, want string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(readAll(t, f), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("child never printed %q: %q", want, readAll(t, f))
}

// argv and the DAG path reach the root untouched, and the root's output lands
// on the supervisor's own stdout; a clean exit completes the unit and, as the
// container entrypoint, the monitor.
func TestDagRootUnitPropagatesArgvAndInheritsOutput(t *testing.T) {
	ru := startRealUnit(t, "exit:0", true, nil, nil)
	if code := ru.wait(t); code != 0 {
		t.Fatalf("exit = %d\n%s\n%s", code, ru.log.String(), readAll(t, ru.out))
	}
	got := readAll(t, ru.out)
	if want := "child argv: dag " + ru.dag + " svc\n"; !strings.Contains(got, want) {
		t.Fatalf("child output %q does not carry %q", got, want)
	}
	if !strings.Contains(ru.log.String(), "completed (exit 0)") {
		t.Fatalf("log:\n%s", ru.log.String())
	}
}

func TestDagRootUnitRestartsARealChildUntilItCompletes(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "runs")
	ru := startRealUnit(t, "count:"+counter, true, nil, nil)
	if code := ru.wait(t); code != 0 {
		t.Fatalf("exit = %d\n%s\n%s", code, ru.log.String(), readAll(t, ru.out))
	}
	b, _ := os.ReadFile(counter)
	if n := strings.Count(string(b), "\n"); n != 3 {
		t.Fatalf("root ran %d times, want 3 (two crashes, then exit 0)\n%s", n, ru.log.String())
	}
	log := ru.log.String()
	if strings.Count(log, "restarting in") != 2 || !strings.Contains(log, "restarting in 10ms") || !strings.Contains(log, "restarting in 20ms") {
		t.Fatalf("log:\n%s", log)
	}
}

// A root that can never succeed (an unknown target, an unparseable file) is
// restarted only up to the start limit, then left failed: the monitor
// returns 1 instead of looping.
func TestDagRootUnitThatNeverSucceedsEndsFailedAtTheStartLimit(t *testing.T) {
	ru := startRealUnit(t, "exit:2", true, nil, func(u *fleet.Service) {
		u.Restart.StartLimit = fleet.ServiceStartLim{Burst: 3, Interval: "1m"}
	})
	if code := ru.wait(t); code != 1 {
		t.Fatalf("exit = %d, want 1 (failed)\n%s", code, ru.log.String())
	}
	if n := strings.Count(readAll(t, ru.out), "child argv:"); n != 3 {
		t.Fatalf("root was started %d times, want exactly the burst (3)\n%s", n, ru.log.String())
	}
	if !strings.Contains(ru.log.String(), "start limit hit: 3 starts within 1m0s") {
		t.Fatalf("log:\n%s", ru.log.String())
	}
}

func TestMonitorForwardsTermToARealProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	ru := startRealUnit(t, "hold", false, nil, nil)
	waitForOutput(t, ru.out, "child holding")
	ru.sigs <- syscall.SIGTERM
	if code := ru.wait(t); code != 0 {
		t.Fatalf("exit = %d\n%s", code, ru.log.String())
	}
	if st := ru.prov.Status("root"); st.LastExit == nil || *st.LastExit != 143 {
		t.Fatalf("the child did not die of the forwarded TERM (143): %+v\n%s", st, ru.log.String())
	}
}

func TestMonitorKillsARealChildThatIgnoresTerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	ru := startRealUnit(t, "ignore-term", false, nil, nil)
	waitForOutput(t, ru.out, "child holding")
	start := time.Now()
	ru.sigs <- syscall.SIGTERM
	if code := ru.wait(t); code != 0 {
		t.Fatalf("exit = %d\n%s", code, ru.log.String())
	}
	if st := ru.prov.Status("root"); st.LastExit == nil || *st.LastExit != 137 {
		t.Fatalf("the child was not killed when the grace (stop.timeout 500ms) expired (137): %+v\n%s", st, ru.log.String())
	}
	if time.Since(start) < 400*time.Millisecond {
		t.Fatalf("killed after %v, before the grace elapsed", time.Since(start))
	}
}
