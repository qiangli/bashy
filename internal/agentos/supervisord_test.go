// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
)

// ─── fakes: no processes, no clock ────────────────────────────────────────

// fakeChild is one scripted run. exit is delivered when the test says so.
type fakeChild struct {
	exit    chan childExit
	signals []os.Signal
	killed  int
	mu      sync.Mutex
}

func newFakeChild() *fakeChild { return &fakeChild{exit: make(chan childExit, 1)} }

func (c *fakeChild) Wait() childExit { return <-c.exit }
func (c *fakeChild) Signal(sig os.Signal) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.signals = append(c.signals, sig)
	return nil
}
func (c *fakeChild) Kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.killed++
	return nil
}

// fakeClock makes after() fire immediately while recording the delay asked
// for, and lets the test move "now" so a run's length is a stated fact.
type fakeClock struct {
	mu     sync.Mutex
	t      time.Time
	slept  []time.Duration
	fireOn chan struct{} // when non-nil, after() blocks until the test fires it
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}
func (c *fakeClock) after(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.slept = append(c.slept, d)
	c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if c.fireOn != nil {
		go func() { <-c.fireOn; ch <- c.t }()
		return ch
	}
	ch <- c.t
	return ch
}
func (c *fakeClock) sleeps() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.slept...)
}

func fakeSupervisor(t *testing.T, clk *fakeClock, sigs chan os.Signal, runs ...*fakeChild) (*supervisor, *bytes.Buffer) {
	t.Helper()
	var log bytes.Buffer
	i := 0
	s := &supervisor{
		self: "fake-bashy", dagFile: "DAG.md", target: "svc",
		minBackoff: time.Second, maxBackoff: 8 * time.Second, healthyAfter: 30 * time.Second, grace: 5 * time.Second,
		log: &log, after: clk.after, now: clk.now, signals: sigs, preflightDone: true,
		start: func() (supervisedChild, error) {
			if i >= len(runs) {
				t.Fatalf("supervisor spawned a %dth child; only %d were scripted", i+1, len(runs))
			}
			c := runs[i]
			i++
			return c, nil
		},
	}
	return s, &log
}

func TestSupervisordCompletesOnExitZero(t *testing.T) {
	clk := &fakeClock{}
	c := newFakeChild()
	s, log := fakeSupervisor(t, clk, make(chan os.Signal), c)
	c.exit <- childExit{code: 0}
	if code := s.run(); code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, log)
	}
	if len(clk.sleeps()) != 0 || c.killed != 0 {
		t.Fatalf("a completed root must not be restarted or killed: sleeps=%v killed=%d", clk.sleeps(), c.killed)
	}
	if !strings.Contains(log.String(), "completed (exit 0)") {
		t.Fatalf("log:\n%s", log)
	}
}

// Crash, crash, crash: the delay doubles from min and stops at max; a run
// that lasts healthy-after puts the ladder back at min. No real time passes.
func TestSupervisordBackoffDoublesCapsAndResetsAfterHealthyRun(t *testing.T) {
	clk := &fakeClock{}
	runs := []*fakeChild{newFakeChild(), newFakeChild(), newFakeChild(), newFakeChild(), newFakeChild(), newFakeChild(), newFakeChild()}
	s, log := fakeSupervisor(t, clk, make(chan os.Signal), runs...)
	// runs 0-4 crash instantly; run 5 is healthy then crashes; run 6 completes.
	for i, c := range runs[:5] {
		c.exit <- childExit{code: 1 + i}
	}
	go func() {
		// The healthy run: let the supervisor observe a long life before the crash.
		for len(clk.sleeps()) < 5 {
			time.Sleep(time.Millisecond)
		}
		clk.advance(31 * time.Second)
		runs[5].exit <- childExit{code: 137, signaled: true}
		for len(clk.sleeps()) < 6 {
			time.Sleep(time.Millisecond)
		}
		runs[6].exit <- childExit{code: 0}
	}()
	if code := s.run(); code != 0 {
		t.Fatalf("exit = %d\n%s", code, log)
	}
	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 1 * time.Second}
	if got := clk.sleeps(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("backoff sequence = %v, want %v\n%s", got, want, log)
	}
	// Every crashed run's group is told to go, then killed before the restart.
	for i, c := range runs[:6] {
		if len(c.signals) != 1 || c.signals[0] != syscall.SIGTERM || c.killed != 1 {
			t.Errorf("run %d: signals=%v killed=%d, want one TERM then one KILL of the old group", i, c.signals, c.killed)
		}
	}
	if !strings.Contains(log.String(), "killed by signal (status 137); restarting in 1s") {
		t.Fatalf("log:\n%s", log)
	}
}

func TestSupervisordForwardsSignalAndExitsCleanlyWhenChildStops(t *testing.T) {
	clk := &fakeClock{fireOn: make(chan struct{})}
	sigs := make(chan os.Signal, 1)
	c := newFakeChild()
	s, log := fakeSupervisor(t, clk, sigs, c)
	done := make(chan int, 1)
	go func() { done <- s.run() }()
	sigs <- syscall.SIGTERM
	// The child honours the forwarded TERM before the grace timer fires.
	for {
		c.mu.Lock()
		n := len(c.signals)
		c.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if c.signals[0] != syscall.SIGTERM {
		t.Fatalf("forwarded %v, want SIGTERM", c.signals[0])
	}
	c.exit <- childExit{code: 143, signaled: true}
	if code := <-done; code != 0 {
		t.Fatalf("exit = %d, want 0 (a stop is not a failure)\n%s", code, log)
	}
	if c.killed != 0 {
		t.Fatalf("child was killed although it stopped within the grace\n%s", log)
	}
	if got := clk.sleeps(); len(got) != 1 || got[0] != 5*time.Second {
		t.Fatalf("grace timer = %v, want [5s]", got)
	}
}

func TestSupervisordKillsGroupWhenGraceExpires(t *testing.T) {
	clk := &fakeClock{fireOn: make(chan struct{})}
	sigs := make(chan os.Signal, 1)
	c := newFakeChild()
	s, log := fakeSupervisor(t, clk, sigs, c)
	done := make(chan int, 1)
	go func() { done <- s.run() }()
	sigs <- os.Interrupt
	for len(clk.sleeps()) < 1 {
		time.Sleep(time.Millisecond)
	}
	close(clk.fireOn) // the grace expires; the child has ignored INT
	for {
		c.mu.Lock()
		k := c.killed
		c.mu.Unlock()
		if k == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	c.exit <- childExit{code: 137, signaled: true}
	if code := <-done; code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, log)
	}
	if !strings.Contains(log.String(), "grace expired; killing") || !strings.Contains(log.String(), "killed (killed by signal (status 137))") {
		t.Fatalf("log:\n%s", log)
	}
}

func TestSupervisordSignalDuringBackoffExitsWithoutRestarting(t *testing.T) {
	clk := &fakeClock{fireOn: make(chan struct{})}
	sigs := make(chan os.Signal, 1)
	c := newFakeChild()
	s, log := fakeSupervisor(t, clk, sigs, c) // only ONE run scripted
	c.exit <- childExit{code: 2}
	done := make(chan int, 1)
	go func() { done <- s.run() }()
	for len(clk.sleeps()) < 1 {
		time.Sleep(time.Millisecond)
	}
	sigs <- syscall.SIGTERM
	if code := <-done; code != 0 {
		t.Fatalf("exit = %d\n%s", code, log)
	}
	if !strings.Contains(log.String(), "while waiting to restart; exiting") {
		t.Fatalf("log:\n%s", log)
	}
}

func TestSupervisordUsageAndValidation(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--help"}, 0},
		{[]string{"-h"}, 0},
		{nil, 2},
		{[]string{"DAG.md"}, 2},
		{[]string{"DAG.md", "a", "b"}, 2},
		{[]string{"--no-such", "DAG.md", "a"}, 2},
		{[]string{filepath.Join(t.TempDir(), "missing.md"), "a"}, 2},
	} {
		if code := dispatchSupervisord(tc.args); code != tc.code {
			t.Errorf("supervisord %v: exit %d, want %d", tc.args, code, tc.code)
		}
	}
	s := &supervisor{minBackoff: 2 * time.Second, maxBackoff: time.Second, dagFile: "x"}
	if err := s.validate(); err == nil || !strings.Contains(err.Error(), "max-backoff") {
		t.Errorf("min > max accepted: %v", err)
	}
}

// ─── the real thing: this test binary stands in for the dag root ──────────

// supervisordTestChild is what the re-exec'd test binary runs (main_test.go).
// It answers the preflight (`dag --json -n FILE TARGET`) with a plan and runs
// the root (`dag FILE TARGET`) per BASHY_SUPERVISORD_TEST_CHILD:
//
//	exit:N            print argv, exit N
//	count:FILE        append one line to FILE, exit 1 (until FILE has 3 lines: exit 0)
//	hold              print argv, ignore nothing, block until signaled
//	ignore-term       block; on TERM keep going (only KILL ends it)
//	orphan:FILE       spawn a grandchild that outlives us, write its pid to FILE, exit 0
func supervisordTestChild() int {
	script := os.Getenv("BASHY_SUPERVISORD_TEST_CHILD")
	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "dag" && args[1] == "--json" {
		// The plan the real runner would print for the fixture in dagFixture.
		fmt.Println(`{"schema_version":"dag-v1","status":"ok","result":{"plan":[{"name":"a"},{"name":"b"},{"name":"svc"}]}}`)
		return 0
	}
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
	}
	fmt.Fprintln(os.Stderr, "unknown script", script)
	return 9
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

// realSupervisor drives the loop against re-exec'd copies of this test binary.
func realSupervisor(t *testing.T, script string, dagFile string) (*supervisor, *os.File, *bytes.Buffer) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	// Windows refuses to remove a file that is still open: without this the
	// TempDir cleanup fails the test after every assertion has passed.
	t.Cleanup(func() { out.Close() })
	t.Setenv("BASHY_SUPERVISORD_TEST_CHILD", script)
	var log bytes.Buffer
	s := &supervisor{
		self: os.Args[0], dagFile: dagFile, target: "svc",
		minBackoff: 10 * time.Millisecond, maxBackoff: 40 * time.Millisecond, healthyAfter: time.Hour, grace: 500 * time.Millisecond,
		log: &log, stdin: nil, stdout: out, stderr: out,
		after: time.After, now: time.Now, signals: make(chan os.Signal),
	}
	s.start = s.startDagChild
	return s, out, &log
}

func readAll(t *testing.T, f *os.File) string {
	t.Helper()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// argv, the DAG path and the Requires order reach the root untouched, and
// the root's output lands on the supervisor's own stdout.
func TestSupervisordPropagatesArgvAndInheritsOutput(t *testing.T) {
	dag := dagFixture(t)
	s, out, log := realSupervisor(t, "exit:0", dag)
	if code := s.run(); code != 0 {
		t.Fatalf("exit = %d\n%s\n%s", code, log, readAll(t, out))
	}
	got := readAll(t, out)
	if want := "child argv: dag " + dag + " svc\n"; !strings.Contains(got, want) {
		t.Fatalf("child output %q does not carry %q", got, want)
	}
	if !strings.Contains(log.String(), "plan: a -> b -> svc") {
		t.Fatalf("the Requires order was not reported:\n%s", log)
	}
}

func TestSupervisordRestartsARealChildUntilItCompletes(t *testing.T) {
	dag := dagFixture(t)
	counter := filepath.Join(t.TempDir(), "runs")
	s, out, log := realSupervisor(t, "count:"+counter, dag)
	if code := s.run(); code != 0 {
		t.Fatalf("exit = %d\n%s\n%s", code, log, readAll(t, out))
	}
	b, _ := os.ReadFile(counter)
	if n := strings.Count(string(b), "\n"); n != 3 {
		t.Fatalf("root ran %d times, want 3 (two crashes, then exit 0)\n%s", n, log)
	}
	if strings.Count(log.String(), "restarting in") != 2 || !strings.Contains(log.String(), "restarting in 10ms") || !strings.Contains(log.String(), "restarting in 20ms") {
		t.Fatalf("log:\n%s", log)
	}
}

// A preflight the runner rejects is a diagnostic and an exit, not a loop.
func TestSupervisordPreflightFailureNeverRestarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper's exit-status shape is unix")
	}
	dag := dagFixture(t)
	s, out, log := realSupervisor(t, "exit:0", dag)
	s.target = "nosuch"
	// The helper answers every `dag --json -n` with the fixture plan, so
	// route the preflight to a runner that refuses: the real dag itself.
	bin := os.Getenv("BASHY_E2E_BIN")
	if bin == "" {
		if p, err := exec.LookPath("bashy"); err == nil {
			bin = p
		}
	}
	if bin == "" {
		t.Skip("no bashy binary on PATH to run the real preflight")
	}
	s.self = bin
	code := s.run()
	if code == 0 {
		t.Fatalf("unknown target planned successfully?\n%s\n%s", log, readAll(t, out))
	}
	if strings.Contains(log.String(), "restarting") {
		t.Fatalf("an unplannable root was restarted:\n%s", log)
	}
	if !strings.Contains(readAll(t, out), "nosuch") {
		t.Fatalf("the runner's own diagnostic was not passed through:\n%s", readAll(t, out))
	}
}

func TestSupervisordForwardsTermToARealProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	dag := dagFixture(t)
	sigs := make(chan os.Signal, 1)
	s, out, log := realSupervisor(t, "hold", dag)
	s.signals = sigs
	done := make(chan int, 1)
	go func() { done <- s.run() }()
	waitForOutput(t, out, "child holding")
	sigs <- syscall.SIGTERM
	if code := <-done; code != 0 {
		t.Fatalf("exit = %d\n%s", code, log)
	}
	if !strings.Contains(log.String(), "stopped (killed by signal (status 143))") {
		t.Fatalf("the child did not die of the forwarded TERM:\n%s", log)
	}
}

func TestSupervisordKillsARealChildThatIgnoresTerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	dag := dagFixture(t)
	sigs := make(chan os.Signal, 1)
	s, out, log := realSupervisor(t, "ignore-term", dag)
	s.signals = sigs
	done := make(chan int, 1)
	go func() { done <- s.run() }()
	waitForOutput(t, out, "child holding")
	sigs <- syscall.SIGTERM
	if code := <-done; code != 0 {
		t.Fatalf("exit = %d\n%s", code, log)
	}
	if !strings.Contains(log.String(), "grace expired") || !strings.Contains(log.String(), "killed (killed by signal (status 137))") {
		t.Fatalf("log:\n%s", log)
	}
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

var _ = json.Marshal
