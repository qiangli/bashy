package agentos

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/resources"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Every test here keeps bashySelfPath() away from the test binary: a limit
// scope re-executes children through `bashy limit`, and in a test binary
// "bashy" would be the test binary running itself.
func noSelf(t *testing.T) string {
	t.Helper()
	self := filepath.Join(t.TempDir(), "no-bashy-self")
	t.Setenv("BASHY_SELF", self)
	return self
}

// stubHost replaces the host probes and resets the floor cache.
func stubHost(t *testing.T, availMem, totalMem, freeDisk, totalDisk uint64) *int {
	t.Helper()
	calls := new(int)
	oldMem, oldDisk := limitMemoryNow, limitFreeDiskAt
	limitMemoryNow = func() (resources.Memory, error) {
		*calls++
		return resources.Memory{TotalBytes: totalMem, AvailableBytes: availMem}, nil
	}
	limitFreeDiskAt = func(string) (uint64, uint64, error) { return freeDisk, totalDisk, nil }
	resetFloorCache()
	t.Cleanup(func() { limitMemoryNow, limitFreeDiskAt = oldMem, oldDisk; resetFloorCache() })
	return calls
}

const gib = uint64(1) << 30

func TestParseLimitFlags(t *testing.T) {
	spec, argv, err := parseLimitArgs([]string{"--memory", "64Mi", "--pids=5", "--disk", "2G", "--", "sh", "-c", "true"})
	if err != nil || spec.memory != 64<<20 || spec.pids != 5 || spec.disk != 2e9 || !slices.Equal(argv, []string{"sh", "-c", "true"}) {
		t.Fatalf("parse = %+v %v %v", spec, argv, err)
	}
	for _, bad := range [][]string{nil, {"--memory", "64Mi"}, {"--memory", "lots", "--", "true"}, {"--pids", "-1", "--", "true"}, {"--cpu", "1", "--", "true"}} {
		if _, _, err := parseLimitArgs(bad); err == nil {
			t.Errorf("parseLimitArgs(%q) accepted", bad)
		}
	}
	if code := dispatchLimit([]string{"--memory"}); code != 2 {
		t.Fatalf("usage error: exit %d, want 2", code)
	}
}

func unixOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh and sleep")
	}
	for _, p := range []string{"sh", "sleep"} {
		if _, err := exec.LookPath(p); err != nil {
			t.Skipf("no %s", p)
		}
	}
}

func TestRunLimitPassesChildStatus(t *testing.T) {
	unixOnly(t)
	noSelf(t)
	stubHost(t, 8*gib, 16*gib, 100*gib, 500*gib)
	var errOut bytes.Buffer
	if code := runLimit(limitSpec{memory: 1 << 40, pids: 64}, []string{"sh", "-c", "exit 3"}, t.TempDir(), &errOut); code != 3 {
		t.Fatalf("exit %d, want the child's 3 (stderr %q)", code, errOut.String())
	}
}

func TestRunLimitPidsBreachKillsTree(t *testing.T) {
	unixOnly(t)
	noSelf(t)
	stubHost(t, 8*gib, 16*gib, 100*gib, 500*gib)
	dir := t.TempDir()
	marker := filepath.Join(dir, "survived")
	var errOut bytes.Buffer
	start := time.Now()
	code := runLimit(limitSpec{pids: 3}, []string{"sh", "-c", "for i in 1 2 3 4 5 6; do sleep 4 & done; wait; touch " + marker}, dir, &errOut)
	if code != limitBreachStatus {
		t.Fatalf("exit %d, want %d (stderr %q)", code, limitBreachStatus, errOut.String())
	}
	if el := time.Since(start); el > 4*time.Second {
		t.Fatalf("took %v: the breach did not stop the tree", el)
	}
	rec := errOut.String()
	if !strings.Contains(rec, `"kind":"limit_exceeded"`) || !strings.Contains(rec, `"resource":"pids"`) || !strings.Contains(rec, `"limit":3`) || !strings.Contains(rec, `"pid":`) {
		t.Fatalf("breach record = %q", rec)
	}
	time.Sleep(4500*time.Millisecond - time.Since(start))
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the tree survived the breach")
	}
}

func TestRunLimitMemoryBreach(t *testing.T) {
	unixOnly(t)
	noSelf(t)
	stubHost(t, 8*gib, 16*gib, 100*gib, 500*gib)
	var errOut bytes.Buffer
	// Any real process is bigger than 1 KiB: no allocation needed to trip it.
	if code := runLimit(limitSpec{memory: 1 << 10}, []string{"sleep", "5"}, t.TempDir(), &errOut); code != limitBreachStatus {
		t.Fatalf("exit %d, want %d (stderr %q)", code, limitBreachStatus, errOut.String())
	}
	if !strings.Contains(errOut.String(), `"resource":"memory"`) {
		t.Fatalf("record = %q", errOut.String())
	}
}

func TestRunLimitDiskBreach(t *testing.T) {
	unixOnly(t)
	noSelf(t)
	stubHost(t, 8*gib, 16*gib, 100*gib, 500*gib)
	var errOut bytes.Buffer
	if code := runLimit(limitSpec{disk: 200 * gib}, []string{"sleep", "5"}, t.TempDir(), &errOut); code != limitBreachStatus {
		t.Fatalf("exit %d, want %d (stderr %q)", code, limitBreachStatus, errOut.String())
	}
	if !strings.Contains(errOut.String(), `"resource":"disk"`) {
		t.Fatalf("record = %q", errOut.String())
	}
}

func TestRunLimitAdmissionRefusesBeforeStart(t *testing.T) {
	unixOnly(t)
	noSelf(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	stubHost(t, 100<<20, 16*gib, 100*gib, 500*gib) // 100 MiB available < 5% of 16 GiB
	var errOut bytes.Buffer
	if code := runLimit(limitSpec{}, []string{"touch", marker}, dir, &errOut); code != limitRefusedStatus {
		t.Fatalf("exit %d, want %d", code, limitRefusedStatus)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a refused command ran")
	}
	if rec := errOut.String(); !strings.Contains(rec, `"kind":"refused"`) || !strings.Contains(rec, `"resource":"memory"`) || !strings.Contains(rec, `"floor":858993459`) {
		t.Fatalf("refusal = %q", rec)
	}
	t.Setenv("BASHY_LIMITS", "off")
	errOut.Reset()
	if code := runLimit(limitSpec{}, []string{"touch", marker}, dir, &errOut); code != 0 {
		t.Fatalf("BASHY_LIMITS=off: exit %d (%q)", code, errOut.String())
	}
}

func TestHostFloorDefaultsAndOverrides(t *testing.T) {
	env := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	f := hostFloors(env(nil), 8*gib, 1000*gib)
	if f.memory != 8*gib/20 || f.disk != 2*gib {
		t.Fatalf("defaults = %+v, want 5%% of 8Gi memory and 2Gi disk", f)
	}
	f = hostFloors(env(map[string]string{"BASHY_MIN_FREE_MEMORY": "256Mi", "BASHY_MIN_FREE_DISK": "10Gi"}), 64*gib, 1000*gib)
	if f.memory != 256<<20 || f.disk != 10*gib {
		t.Fatalf("overrides = %+v", f)
	}
	if f = hostFloors(env(map[string]string{"BASHY_LIMITS": "off"}), 64*gib, 1000*gib); !f.off {
		t.Fatal("BASHY_LIMITS=off must disable the floors")
	}
}

// runFloors runs script through a runner whose exec chain is the floor rung
// over a recorder: nothing external is ever executed.
func runFloors(t *testing.T, script string) ([][]string, string, error) {
	t.Helper()
	var ran [][]string
	var errOut bytes.Buffer
	record := func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error { ran = append(ran, args); return nil }
	}
	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "floors.sh")
	if err != nil {
		t.Fatal(err)
	}
	r, err := interp.New(interp.Env(nil), interp.StdIO(nil, new(bytes.Buffer), &errOut), interp.Dir(t.TempDir()),
		interp.ExecHandlers(hostFloorHandler(), record))
	if err != nil {
		t.Fatal(err)
	}
	err = r.Run(context.Background(), prog)
	return ran, errOut.String(), err
}

func TestHostFloorRefusesNewExecOnLowMemory(t *testing.T) {
	calls := stubHost(t, 200<<20, 16*gib, 100*gib, 500*gib)
	ran, stderr, err := runFloors(t, "go build ./...; echo $?; ps; du -s .; foo-tool")
	var names []string
	for _, a := range ran {
		names = append(names, a[0])
	}
	if !slices.Equal(names, []string{"ps", "du"}) {
		t.Fatalf("ran %v, want only the recovery set (err %v)", names, err)
	}
	if !strings.Contains(stderr, `"kind":"refused"`) || !strings.Contains(stderr, `"command":"go"`) {
		t.Fatalf("refusal = %q", stderr)
	}
	if *calls != 1 {
		t.Fatalf("memory sampled %d times in one second, want 1 (cached)", *calls)
	}
	ran, stderr, _ = runFloors(t, "bashy.real sprint list")
	if len(ran) != 0 || !strings.Contains(stderr, `"command":"bashy sprint"`) {
		t.Fatalf("a verb shim must be refused by its verb name: ran %v, %q", ran, stderr)
	}
	var st interp.ExitStatus
	if !errors.As(err, &st) || st != limitRefusedStatus {
		t.Fatalf("last refused exec: %v, want exit %d", err, limitRefusedStatus)
	}
}

func TestHostFloorRefusesWritesOnLowDisk(t *testing.T) {
	stubHost(t, 8*gib, 16*gib, 1*gib, 500*gib) // 1 GiB free < 2 GiB floor
	ran, stderr, _ := runFloors(t, "cp a b; foo-tool; date; rm -f x; du -sh .; git status; git commit -m x")
	var got []string
	for _, a := range ran {
		got = append(got, strings.Join(a, " "))
	}
	want := []string{"date", "rm -f x", "du -sh .", "git status"}
	if !slices.Equal(got, want) {
		t.Fatalf("ran %q, want %q", got, want)
	}
	if !strings.Contains(stderr, `"resource":"disk"`) {
		t.Fatalf("refusal = %q", stderr)
	}
	ran, _, _ = runFloors(t, "export BASHY_LIMITS=off; cp a b")
	if len(ran) != 1 {
		t.Fatalf("BASHY_LIMITS=off still refused: %v", ran)
	}
}

func TestLimitDecoratorWrapsExternalChildren(t *testing.T) {
	self := noSelf(t)
	var ran [][]string
	var errOut bytes.Buffer
	record := func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error { ran = append(ran, args); return nil }
	}
	script := `@limit(memory: "64Mi", pids: 8, disk: "1Gi")
f() { ext1 a; }
@limit(memory: "1Gi", pids: 4)
g() { f; }
f; g; ext2`
	prog, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(script), "limit.bsh")
	if err != nil {
		t.Fatal(err)
	}
	r, err := interp.New(interp.Lang(syntax.LangBashPP), interp.Env(nil), interp.StdIO(nil, new(bytes.Buffer), &errOut),
		interp.Decorators(map[string]interp.DecoratorFunc{"limit": adaptInterpreterDecorator(limitDecorator(&errOut))}),
		interp.ExecHandlers(limitHandler(), record))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background(), prog); err != nil {
		t.Fatalf("run: %v (%s)", err, errOut.String())
	}
	want := [][]string{
		{self, "limit", "--memory", "67108864", "--pids", "8", "--disk", "1073741824", "--", "ext1", "a"},
		// nested scopes narrow: the tighter of each limit applies
		{self, "limit", "--memory", "67108864", "--pids", "4", "--disk", "1073741824", "--", "ext1", "a"},
		{"ext2"},
	}
	if len(ran) != len(want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	for i := range want {
		if !slices.Equal(ran[i], want[i]) {
			t.Fatalf("exec %d = %q, want %q", i, ran[i], want[i])
		}
	}
}

func TestLimitDecoratorRejectsUnknownArgs(t *testing.T) {
	noSelf(t)
	for _, args := range []string{`cpu: "2"`, `"4Gi"`, `memory: "lots"`, `pids: "x"`, ``} {
		var errOut bytes.Buffer
		script := "@limit(" + args + ")\nf() { true; }\nf"
		prog, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(script), "limit.bsh")
		if err != nil {
			t.Fatal(err)
		}
		r, _ := interp.New(interp.Lang(syntax.LangBashPP), interp.Env(nil), interp.StdIO(nil, new(bytes.Buffer), &errOut),
			interp.Decorators(map[string]interp.DecoratorFunc{"limit": adaptInterpreterDecorator(limitDecorator(&errOut))}))
		if err := r.Run(context.Background(), prog); err == nil && !strings.Contains(errOut.String(), "limit") {
			t.Errorf("@limit(%s) accepted", args)
		}
	}
}

func TestLimitComposesWithContain(t *testing.T) {
	self := noSelf(t)
	var got []string
	chain := containHandler()(limitHandler()(func(ctx context.Context, args []string) error { got = args; return nil }))
	ctx := withLimitScope(withContainNet(context.Background(), ""), limitSpec{pids: 8})
	if err := chain(ctx, []string{"python3", "t.py"}); err != nil {
		t.Fatal(err)
	}
	want := []string{self, "limit", "--pids", "8", "--", self, "contain", "--net", "deny", "--", "python3", "t.py"}
	if !slices.Equal(got, want) {
		t.Fatalf("argv = %q, want %q: the limit must watch the contained tree", got, want)
	}
}
