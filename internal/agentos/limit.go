package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/resources"
	"mvdan.cc/sh/v3/interp"
)

// limit bounds what one action may consume, like a pod's resources.limits.
//
//	bashy limit --memory 4Gi --pids 256 --disk 2Gi -- CMD [ARGS...]  (the wrapper)
//	@limit(memory: "4Gi", pids: 256, disk: "2Gi") f() { go test ./...; } (the decorator)
//
// The guard watches the whole process tree the command starts: --memory caps
// its summed RSS, --pids its process count, and --disk is the free space the
// working volume must keep while it runs. A breach kills the tree (TERM, then
// KILL ~2 s later) with one structured record, exit 137. Under @limit every
// EXTERNAL child a function starts is re-executed through `bashy limit`, so
// each child tree is bounded; nested scopes only tighten.
//
// Host floors (agent mode) refuse NEW work while the host is already under
// water: an external exec when available memory is below the floor, a write
// (or unclassified) command when the working volume is below its disk floor.
// Refusal is exit 75 with one structured line; builtins, in-process tools and
// the recovery set (rm, du, df, ls, ps, kill, …) are never refused.

const (
	limitRefusedStatus = 75  // EX_TEMPFAIL: the host is under its floor; retry later
	limitBreachStatus  = 137 // 128+SIGKILL: the guard killed the tree
)

var (
	limitMemoryNow   = resources.MemoryNow
	limitFreeDiskAt  = resources.FreeDiskAt
	limitSampleEvery = 500 * time.Millisecond
	limitKillGrace   = 2 * time.Second
)

type limitSpec struct {
	memory uint64 // max tree RSS in bytes
	pids   int    // max tree process count
	disk   uint64 // min free bytes on the working volume
}

func (s limitSpec) flags() []string {
	var out []string
	if s.memory > 0 {
		out = append(out, "--memory", strconv.FormatUint(s.memory, 10))
	}
	if s.pids > 0 {
		out = append(out, "--pids", strconv.Itoa(s.pids))
	}
	if s.disk > 0 {
		out = append(out, "--disk", strconv.FormatUint(s.disk, 10))
	}
	return out
}

// narrow is the tighter of two scopes: the smaller caps, the larger floor.
func (s limitSpec) narrow(o limitSpec) limitSpec {
	tighter := func(a, b uint64) uint64 {
		if a == 0 || b != 0 && b < a {
			return b
		}
		return a
	}
	s.memory = tighter(s.memory, o.memory)
	s.pids = int(tighter(uint64(s.pids), uint64(o.pids)))
	s.disk = max(s.disk, o.disk)
	return s
}

func (s limitSpec) set(name, value string) (limitSpec, error) {
	switch name {
	case "memory", "disk":
		q, err := resources.ParseQuantity(value)
		if err != nil || q == 0 {
			return s, fmt.Errorf("%s: invalid quantity %q (want e.g. 512Mi, 4Gi)", name, value)
		}
		if name == "memory" {
			s.memory = q
		} else {
			s.disk = q
		}
	case "pids":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return s, fmt.Errorf("pids: want a positive count, got %q", value)
		}
		s.pids = n
	default:
		return s, fmt.Errorf("unknown limit %q", name)
	}
	return s, nil
}

// limitRecord is the one structured line a refusal or a breach prints.
type limitRecord struct {
	Schema   string `json:"schema_version"`
	Kind     string `json:"kind"` // refused | limit_exceeded
	Resource string `json:"resource"`
	Observed uint64 `json:"observed"`
	Limit    uint64 `json:"limit,omitempty"`
	Floor    uint64 `json:"floor,omitempty"`
	Unit     string `json:"unit"`
	PID      int    `json:"pid,omitempty"`
	Command  string `json:"command,omitempty"`
	Exit     int    `json:"exit"`
	Off      string `json:"off,omitempty"`
}

func writeLimitRecord(w io.Writer, r limitRecord) {
	r.Schema = "bashy-limit-v1"
	b, _ := json.Marshal(r)
	fmt.Fprintf(w, "%s\n", b)
}

// ---- the decorator --------------------------------------------------------

type limitKey struct{}

func limitScopeFrom(ctx context.Context) (limitSpec, bool) {
	s, ok := ctx.Value(limitKey{}).(limitSpec)
	return s, ok
}

func withLimitScope(ctx context.Context, s limitSpec) context.Context {
	if outer, ok := limitScopeFrom(ctx); ok {
		s = outer.narrow(s)
	}
	return context.WithValue(ctx, limitKey{}, s)
}

func limitDecorator(stderr io.Writer) nativeDecoratorFunc {
	return func(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
		if c.Advised != "" {
			return errors.New("limit is a declaration and never applied by advice")
		}
		const usage = `limit takes memory: "4Gi", pids: 256, disk: "2Gi" (at least one, each once)`
		var spec limitSpec
		seen := map[string]bool{}
		for _, a := range args {
			if a.Name == "" || seen[a.Name] {
				return errors.New(usage)
			}
			seen[a.Name] = true
			var err error
			if spec, err = spec.set(a.Name, a.Value); err != nil {
				return fmt.Errorf("limit: %v; %s", err, usage)
			}
		}
		if len(seen) == 0 {
			return errors.New(usage)
		}
		c.Next(withLimitScope(ctx, spec))
		return nil
	}
}

// limitHandler is the innermost rung, after contain: what reaches it is about
// to run as an external child, so inside @limit it is re-executed through
// `bashy limit` — around any contain wrapper, so the guard sees that tree too.
func limitHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			spec, ok := limitScopeFrom(ctx)
			if len(args) == 0 || !ok {
				return next(ctx, args)
			}
			wrapped := append([]string{bashySelfPath(), "limit"}, spec.flags()...)
			return next(ctx, append(append(wrapped, "--"), args...))
		}
	}
}

// ---- host floors ----------------------------------------------------------

type floors struct {
	off          bool
	memory, disk uint64
}

// hostFloors resolves the floors: BASHY_LIMITS=off disables them;
// BASHY_MIN_FREE_MEMORY / BASHY_MIN_FREE_DISK set them; the defaults are
// min(1 GiB, 5% of total memory) and min(2 GiB, 5% of the volume).
func hostFloors(getenv func(string) string, totalMem, totalDisk uint64) floors {
	switch strings.ToLower(getenv("BASHY_LIMITS")) {
	case "off", "0", "false", "no":
		return floors{off: true}
	}
	floor := func(env string, abs, total uint64) uint64 {
		if q, err := resources.ParseQuantity(strings.TrimSpace(getenv(env))); err == nil && getenv(env) != "" {
			return q
		}
		if total > 0 {
			return min(abs, total/20)
		}
		return abs
	}
	return floors{memory: floor("BASHY_MIN_FREE_MEMORY", 1<<30, totalMem), disk: floor("BASHY_MIN_FREE_DISK", 2<<30, totalDisk)}
}

// The floor cache keeps exec cheap: memory and disk are sampled at most about
// once a second.
var floorCache struct {
	sync.Mutex
	memAt, diskAt     time.Time
	mem               resources.Memory
	memErr            error
	diskDir           string
	diskFree, diskTot uint64
	diskErr           error
}

func resetFloorCache() {
	floorCache.Lock()
	floorCache.memAt, floorCache.diskAt, floorCache.diskDir = time.Time{}, time.Time{}, ""
	floorCache.Unlock()
}

func cachedMemory() (resources.Memory, error) {
	floorCache.Lock()
	defer floorCache.Unlock()
	if time.Since(floorCache.memAt) > time.Second {
		floorCache.mem, floorCache.memErr = limitMemoryNow()
		floorCache.memAt = time.Now()
	}
	return floorCache.mem, floorCache.memErr
}

func cachedDisk(dir string) (free, total uint64, err error) {
	floorCache.Lock()
	defer floorCache.Unlock()
	if dir != floorCache.diskDir || time.Since(floorCache.diskAt) > time.Second {
		floorCache.diskFree, floorCache.diskTot, floorCache.diskErr = limitFreeDiskAt(dir)
		floorCache.diskAt, floorCache.diskDir = time.Now(), dir
	}
	return floorCache.diskFree, floorCache.diskTot, floorCache.diskErr
}

// floorRefusal checks the host floors for one command about to start. A probe
// that cannot read the host admits: a floor refuses on evidence only.
func floorRefusal(getenv func(string) string, dir, command string, write func() bool) (limitRecord, bool) {
	if strings.ToLower(getenv("BASHY_LIMITS")) == "off" {
		return limitRecord{}, false
	}
	if mem, err := cachedMemory(); err == nil && mem.AvailableBytes > 0 {
		if f := hostFloors(getenv, mem.TotalBytes, 0); !f.off && mem.AvailableBytes < f.memory {
			return limitRecord{Kind: "refused", Resource: "memory", Observed: mem.AvailableBytes, Floor: f.memory, Unit: "bytes", Command: command, Exit: limitRefusedStatus, Off: "BASHY_LIMITS=off"}, true
		}
	}
	if write() {
		if free, total, err := cachedDisk(dir); err == nil && total > 0 {
			if f := hostFloors(getenv, 0, total); !f.off && free < f.disk {
				return limitRecord{Kind: "refused", Resource: "disk", Observed: free, Floor: f.disk, Unit: "bytes", Command: command, Exit: limitRefusedStatus, Off: "BASHY_LIMITS=off"}, true
			}
		}
	}
	return limitRecord{}, false
}

// limitRecoverySet is what an operator reaches for to get a host back above
// water; the floors never refuse it.
var limitRecoverySet = map[string]bool{
	"rm": true, "rmdir": true, "du": true, "df": true, "ls": true, "ps": true, "top": true, "htop": true,
	"kill": true, "pkill": true, "killall": true, "cat": true, "head": true, "tail": true, "less": true,
	"grep": true, "find": true, "stat": true, "wc": true, "free": true, "vm_stat": true, "uptime": true,
	"memory_pressure": true, "pgrep": true, "lsof": true, "id": true, "whoami": true, "which": true,
	"true": true, "false": true, "tasklist": true, "taskkill": true,
}

var limitRecoveryVerbs = map[string]map[string]bool{
	"git":    {"status": true, "log": true, "diff": true, "show": true, "rev-parse": true, "branch": true},
	"bashy":  {"resources": true, "resource": true, "inspect": true, "doctor": true, "limit": true, "ps": true, "kill": true},
	"podman": {"ps": true, "stop": true, "kill": true, "rm": true, "rmi": true, "system": true},
	"docker": {"ps": true, "stop": true, "kill": true, "rm": true, "rmi": true, "system": true},
}

func commandName(arg0 string) string {
	return strings.TrimSuffix(strings.ToLower(baseName(arg0)), ".exe")
}

func limitRecovery(args []string) bool {
	name := commandName(args[0])
	if limitRecoverySet[name] {
		return true
	}
	if verbs, ok := limitRecoveryVerbs[name]; ok && len(args) > 1 {
		return verbs[args[1]]
	}
	return false
}

// limitWrites reports whether the disk floor applies: a command the atlas
// says writes, or one nobody has classified.
func limitWrites(args []string) bool {
	name := args[0]
	if isBashyExecutable(name) && len(args) > 1 {
		name = args[1]
	}
	effects := declaredEffects(name)
	return effects == nil || slices.Contains(effects, atlas.EffWrite)
}

// hostFloorHandler refuses a new external exec while the host is under its
// floors (agent mode only; see WireExec). It sits before contain and limit,
// so it judges the command itself, not a wrapper.
func hostFloorHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 || limitRecovery(args) {
				return next(ctx, args)
			}
			hc := interp.HandlerCtx(ctx)
			getenv := func(k string) string { return hc.Env.Get(k).String() }
			name := commandName(args[0])
			if isBashyExecutable(args[0]) && len(args) > 1 { // a verb shim: name the verb
				name = "bashy " + args[1]
			}
			if rec, refused := floorRefusal(getenv, hc.Dir, name, func() bool { return limitWrites(args) }); refused {
				writeLimitRecord(hc.Stderr, rec)
				return interp.ExitStatus(limitRefusedStatus)
			}
			return next(ctx, args)
		}
	}
}

// ---- the wrapper ----------------------------------------------------------

func parseLimitArgs(args []string) (limitSpec, []string, error) {
	var spec limitSpec
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			if i+1 >= len(args) {
				break
			}
			return spec, args[i+1:], nil
		}
		name, value, inline := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if !strings.HasPrefix(a, "--") {
			if spec == (limitSpec{}) {
				return spec, nil, errors.New("set at least one limit")
			}
			return spec, args[i:], nil
		}
		if !inline {
			if i+1 >= len(args) {
				return spec, nil, fmt.Errorf("--%s needs a value", name)
			}
			i++
			value = args[i]
		}
		var err error
		if spec, err = spec.set(name, value); err != nil {
			return spec, nil, err
		}
	}
	return spec, nil, errors.New("no command")
}

func limitUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: bashy limit [--memory Q] [--pids N] [--disk Q] -- command [args...]
  run one command under resource limits, like a pod's resources.limits:
    --memory Q  cap the command's whole process tree at Q bytes of RSS
    --pids N    cap the tree at N processes
    --disk Q    keep at least Q bytes free on the working volume while it runs
  Q is a quantity: Ki/Mi/Gi/Ti (binary), K/M/G/T (decimal), k/m/g/t (binary).
  A breach kills the tree (TERM, then KILL ~2s later), prints one JSON record
  and exits 137; otherwise the exit status is the command's own.
  Before starting, the host floors apply: exit 75 with one JSON record when
  available memory is below min(1Gi, 5% of RAM) or free disk on the working
  volume below min(2Gi, 5%). BASHY_MIN_FREE_MEMORY / BASHY_MIN_FREE_DISK set
  the floors, BASHY_LIMITS=off disables them. The decorator form is
  @limit(memory: "4Gi", pids: 256, disk: "2Gi"). Nests with contain and awd.`)
}

func dispatchLimit(args []string) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		limitUsage(os.Stdout)
		return 0
	}
	spec, argv, err := parseLimitArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy limit: %v\n", err)
		limitUsage(os.Stderr)
		return 2
	}
	dir, _ := os.Getwd()
	return runLimit(spec, argv, dir, os.Stderr)
}

// runLimit admits, starts and watches one command; stderr takes the records.
func runLimit(spec limitSpec, argv []string, dir string, stderr io.Writer) int {
	if rec, refused := floorRefusal(os.Getenv, dir, commandName(argv[0]), func() bool { return true }); refused {
		writeLimitRecord(stderr, rec)
		return limitRefusedStatus
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		fmt.Fprintf(stderr, "bashy limit: %v\n", err)
		return 127
	}
	cmd := exec.Command(path, argv[1:]...)
	cmd.Args[0] = argv[0]
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = dir, os.Stdin, os.Stdout, os.Stderr
	// Its own process group, so one signal reaches the tree — except on a
	// terminal, where a background group could not read it; there the guard
	// signals the tree's pids one by one.
	grouped := !isTerminal(os.Stdin)
	if grouped {
		cmd.SysProcAttr = ownProcessGroupAttr()
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "bashy limit: %v\n", err)
		return 126
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)

	pid := cmd.Process.Pid
	var tree []int
	tick := time.NewTimer(limitSampleEvery / 5) // first look early
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			return childExitStatus(err)
		case s := <-sigs:
			if grouped {
				_ = signalProcessGroup(cmd.Process, s)
			} else if s != os.Interrupt { // the terminal already told the child
				_ = cmd.Process.Signal(s)
			}
		case <-tick.C:
			tick.Reset(limitSampleEvery)
			rec, breached := limitBreach(spec, pid, dir, &tree)
			if !breached {
				continue
			}
			writeLimitRecord(stderr, rec)
			killLimitTree(cmd.Process, grouped, tree, done)
			return limitBreachStatus
		}
	}
}

// limitBreach samples the tree (and the volume) once; tree keeps the last
// membership so a kill reaches pids that left the group.
func limitBreach(spec limitSpec, pid int, dir string, tree *[]int) (limitRecord, bool) {
	if spec.memory > 0 || spec.pids > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), limitSampleEvery)
		u, err := resources.ProcessTree(ctx, pid)
		cancel()
		if err == nil && u.Processes > 0 {
			*tree = u.PIDs
			if spec.memory > 0 && u.RSSBytes > spec.memory {
				return limitRecord{Kind: "limit_exceeded", Resource: "memory", Observed: u.RSSBytes, Limit: spec.memory, Unit: "bytes", PID: pid, Exit: limitBreachStatus}, true
			}
			if spec.pids > 0 && u.Processes > spec.pids {
				return limitRecord{Kind: "limit_exceeded", Resource: "pids", Observed: uint64(u.Processes), Limit: uint64(spec.pids), Unit: "processes", PID: pid, Exit: limitBreachStatus}, true
			}
		}
	}
	if spec.disk > 0 {
		if free, _, err := limitFreeDiskAt(dir); err == nil && free < spec.disk {
			return limitRecord{Kind: "limit_exceeded", Resource: "disk", Observed: free, Limit: spec.disk, Unit: "bytes", PID: pid, Exit: limitBreachStatus}, true
		}
	}
	return limitRecord{}, false
}

// killLimitTree: TERM the group and every known tree pid, wait up to the
// grace for the root, then KILL whatever is left.
func killLimitTree(p *os.Process, grouped bool, tree []int, done <-chan error) {
	signalAll := func(s os.Signal) {
		if grouped {
			if s == os.Kill {
				_ = killProcessGroup(p)
			} else {
				_ = signalProcessGroup(p, s)
			}
		}
		for _, pid := range tree {
			if q, err := os.FindProcess(pid); err == nil {
				if s == os.Kill {
					_ = q.Kill()
				} else {
					_ = q.Signal(s)
				}
			}
		}
	}
	signalAll(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(limitKillGrace):
	}
	signalAll(os.Kill)
	select {
	case <-done:
	case <-time.After(limitKillGrace):
	}
}

// childExitStatus maps a child's wait result to a shell exit status.
func childExitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	return 1
}
