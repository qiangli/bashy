// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Repair profiles (S549.R4a): for each core component, one command runs its
// full repair profile natively on the host it runs on — Linux, macOS or
// Windows, like the release gate — and writes the evidence record the repair
// gate (S549.R3a `src pr`) consumes. Rules of record (docs/repair-profiles.md):
//
//   - a run with the network disabled passes from the SHA-pinned cache;
//   - a killed, partial or truncated run is a failure, never a pass;
//   - GPL suites are fetched by SHA into the gitignored user cache, never
//     vendored (the same cache `make test-bash` uses — see EnsureBash53Fixtures);
//   - VSC/PCTS is excluded (licensed; a different lane, never auto-fetched);
//   - only landmine suites (uutils) run in the Linux oci sandbox lane, and a
//     structurally unavailable lane is reported "skipped (lane unavailable)",
//     never "pass".

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// RepairProfileSchema versions the evidence record. Bump (v2, v3…) on any
// breaking field change — the repair gate pins this string.
const RepairProfileSchema = "bashy-repair-profile-v1"

// Repair suite lanes. Native suites run on the host itself on every OS; the
// oci-linux lane is the landmine lane (uutils), Linux-only by construction.
const (
	RepairLaneNative   = "native"
	RepairLaneOCILinux = "oci-linux"
)

// repairDefaultTimeout bounds one suite when its spec carries none. Killed at
// the bound, the suite is reported incomplete (never a pass).
const repairDefaultTimeout = 2 * time.Hour

// repairDefaultTailBytes bounds the evidence `output_tail`.
const repairDefaultTailBytes = 16 * 1024

// RepairSuiteSpec describes one suite in a component's repair profile.
type RepairSuiteSpec struct {
	Name    string // evidence name, e.g. "test-bash"
	Lane    string // RepairLaneNative or RepairLaneOCILinux
	Kind    string // "argv" or "go-test-list"
	Args    []string
	Env     []string // extra KEY=VALUE for the suite process
	Timeout time.Duration

	// go-test-list kind: the suite is `go test <GoTestArgs> <pkgs>` where pkgs
	// is `go list ./...` minus every package path containing an Exclude
	// substring (outpost's internal/agent/shell needs a real TTY).
	GoTestArgs []string
	Exclude    []string

	// Fixtures: ensure the SHA-pinned bash-5.3 fixture cache first
	// (offline-aware; see ensureRepairFixtures).
	Fixtures bool

	// Command is the display/evidence form. For go-test-list it is replaced by
	// the effective expanded command in the evidence.
	Command string
}

// RepairCounts carries pass/fail/skip counts parsed from the suite output
// when parseable (go-test --- PASS/FAIL/SKIP lines, the bash53suite Results
// line); nil otherwise ("counts if parseable").
type RepairCounts struct {
	Pass int `json:"pass,omitempty"`
	Fail int `json:"fail,omitempty"`
	Skip int `json:"skip,omitempty"`
}

// RepairSuiteResult is one suite's evidence entry. A suite that was killed by
// a signal or watchdog, or never started, has Complete=false and Pass=false
// with the reason in Reason — a killed or partial run is NEVER a pass.
type RepairSuiteResult struct {
	Name            string        `json:"name"`
	Lane            string        `json:"lane"`
	Command         string        `json:"command"`
	Skipped         bool          `json:"skipped,omitempty"`
	ExitCode        *int          `json:"exit_code,omitempty"`
	Signal          string        `json:"signal,omitempty"`
	DurationMS      int64         `json:"duration_ms,omitempty"`
	Counts          *RepairCounts `json:"counts,omitempty"`
	OutputTail      string        `json:"output_tail,omitempty"`
	OutputTruncated bool          `json:"output_truncated,omitempty"`
	Complete        bool          `json:"complete"`
	Pass            bool          `json:"pass"`
	Reason          string        `json:"reason,omitempty"`
}

// RepairProfileEvidence is the versioned record a repair gate consumes.
// Complete=true only if every eligible suite ran to completion; Pass=true
// only if complete and every eligible suite passed. Skipped (lane
// unavailable) suites are recorded, never counted as pass.
type RepairProfileEvidence struct {
	Schema    string              `json:"schema"`
	Component string              `json:"component"`
	Commit    string              `json:"commit"`
	OS        string              `json:"os"`
	Arch      string              `json:"arch"`
	Start     string              `json:"start"`
	End       string              `json:"end"`
	Suites    []RepairSuiteResult `json:"suites"`
	Complete  bool                `json:"complete"`
	Pass      bool                `json:"pass"`
	Reason    string              `json:"reason,omitempty"`
}

// RepairProfileOptions parameterizes RunRepairProfile.
type RepairProfileOptions struct {
	Component    string
	Root         string                // component checkout; suite commands run with Dir=Root
	CacheBase    string                // fixture cache base ("" = os.UserCacheDir)
	Offline      bool                  // BASHY_OFFLINE=1: never fetch, fail clearly on an empty cache
	Out          string                // evidence path ("" = do not write a file)
	TailBytes    int                   // bounded output_tail (0 = repairDefaultTailBytes)
	Timeout      time.Duration         // default per-suite timeout when spec.Timeout == 0
	Goos         string                // "" = runtime.GOOS (overridable for -list)
	Arch         string                // "" = runtime.GOARCH
	Only         []string              // run only these suite names (in matrix order)
	CustomSuites []RepairSuiteSpec     // overrides RepairSuitesFor (tests, host overrides)
	LaneProbe    func() (bool, string) // oci-linux lane availability; nil = default probe
	Stderr       io.Writer             // per-suite status lines (nil = silent)
}

// RepairSuitesFor returns a component's full repair-profile matrix. The suites
// are the same on every OS — the OS lane decides where they run, like the
// release gate; only the uutils landmine lane is Linux-oci-only.
func RepairSuitesFor(component string) ([]RepairSuiteSpec, error) {
	switch component {
	case "bashy":
		return []RepairSuiteSpec{
			{
				Name: "make-test", Lane: RepairLaneNative, Kind: "argv",
				Args: []string{"make", "test"}, Timeout: repairDefaultTimeout,
				Command: "make test",
			},
			{
				Name: "test-bash", Lane: RepairLaneNative, Kind: "argv",
				Args: []string{"make", "test-bash"}, Timeout: repairDefaultTimeout,
				Fixtures: true, // SHA-pinned GPL fixture cache, never vendored
				Command:  "make test-bash",
			},
			{
				Name: "test-uutils", Lane: RepairLaneOCILinux, Kind: "argv",
				Args: []string{"make", "test-uutils"}, Timeout: repairDefaultTimeout,
				Command: "make test-uutils",
			},
		}, nil
	case "outpost":
		return []RepairSuiteSpec{
			{
				Name: "go-test-short-headless", Lane: RepairLaneNative, Kind: "go-test-list",
				GoTestArgs: []string{"-short", "-tags", "fb_archives"},
				Exclude:    []string{"internal/agent/shell"}, // needs a real TTY
				Timeout:    repairDefaultTimeout,
				Command:    "go test -short -tags fb_archives $(go list ./... | grep -v internal/agent/shell)",
			},
		}, nil
	}
	return nil, fmt.Errorf("repair profile: unknown component %q (want bashy or outpost)", component)
}

// DefaultRepairLaneProbe reports whether the Linux oci sandbox lane is usable
// on this host: linux host + the uutils input clone + an OCI runtime. False is
// reported with the structural reason (the evidence "skipped (lane
// unavailable)" wording carries it); the suite itself never runs natively.
func DefaultRepairLaneProbe(root string) func() (bool, string) {
	return func() (bool, string) {
		if runtime.GOOS != "linux" {
			return false, "landmine suites run only in the Linux oci lane (host is " + runtime.GOOS + ")"
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return false, "cannot resolve the component root: " + err.Error()
		}
		clone := filepath.Join(filepath.Dir(abs), "coreutils", "reference", "uutils-coreutils")
		if fi, err := os.Stat(filepath.Join(clone, "tests", "by-util")); err != nil || !fi.IsDir() {
			return false, "uutils input clone missing at " + clone
		}
		if os.Getenv("UUTILS_OCI") != "" {
			return true, ""
		}
		if _, err := exec.LookPath("docker"); err == nil {
			return true, ""
		}
		if _, err := exec.LookPath("bashy"); err == nil { // bashy podman / bashy oci
			return true, ""
		}
		return false, "no OCI runtime on PATH (docker or bashy podman)"
	}
}

// RunRepairProfile executes the component's suites on THIS host and returns
// the evidence record (writing it to opts.Out when set). The evidence is
// written once, atomically, after all suites finished — a runner killed
// mid-profile leaves no new record, which the gate reads as failure.
func RunRepairProfile(opts RepairProfileOptions) (*RepairProfileEvidence, error) {
	if opts.Goos == "" {
		opts.Goos = runtime.GOOS
	}
	if opts.Arch == "" {
		opts.Arch = runtime.GOARCH
	}
	if opts.TailBytes <= 0 {
		opts.TailBytes = repairDefaultTailBytes
	}
	if opts.Root == "" {
		opts.Root = "."
	}
	specs := opts.CustomSuites
	if specs == nil {
		var err error
		specs, err = RepairSuitesFor(opts.Component)
		if err != nil {
			return nil, err
		}
	}
	if len(opts.Only) > 0 {
		want := map[string]bool{}
		for _, n := range opts.Only {
			want[n] = true
		}
		var kept []RepairSuiteSpec
		for _, s := range specs {
			if want[s.Name] {
				kept = append(kept, s)
			}
		}
		if len(kept) == 0 {
			return nil, fmt.Errorf("repair profile: -only selected no suite (have %v)", suiteNames(specs))
		}
		specs = kept
	}

	start := time.Now().UTC()
	ev := &RepairProfileEvidence{
		Schema: RepairProfileSchema, Component: opts.Component,
		Commit: gitCommit(opts.Root), OS: opts.Goos, Arch: opts.Arch,
		Start: start.Format(time.RFC3339),
	}

	// The SHA-pinned fixture cache, offline-aware: an empty cache under
	// BASHY_OFFLINE fails the whole profile closed before anything runs.
	for _, s := range specs {
		if s.Fixtures {
			cacheBase := opts.CacheBase
			if cacheBase == "" {
				if d, err := os.UserCacheDir(); err == nil {
					cacheBase = d
				}
			}
			if err := ensureRepairFixtures(opts.Root, cacheBase, opts.Offline); err != nil {
				ev.End = time.Now().UTC().Format(time.RFC3339)
				ev.Reason = err.Error()
				for _, s2 := range specs {
					r := RepairSuiteResult{Name: s2.Name, Lane: s2.Lane, Command: s2.Command,
						Reason: "not run: " + err.Error()}
					ev.Suites = append(ev.Suites, r)
				}
				finalizeRepairEvidence(ev)
				if opts.Out != "" {
					if werr := WriteRepairEvidence(opts.Out, ev); werr != nil {
						return ev, werr
					}
				}
				return ev, nil
			}
			break
		}
	}

	laneProbe := opts.LaneProbe
	if laneProbe == nil {
		probe := DefaultRepairLaneProbe(opts.Root)
		laneProbe = func() (bool, string) {
			if opts.Goos != "linux" {
				return false, "landmine suites run only in the Linux oci lane (host is " + opts.Goos + ")"
			}
			return probe()
		}
	}

	for _, spec := range specs {
		if spec.Lane == RepairLaneOCILinux {
			ok, reason := laneProbe()
			if !ok {
				ev.Suites = append(ev.Suites, skippedRepairSuite(spec, reason))
				continue
			}
		}
		res, err := runRepairSuite(spec, opts)
		if err != nil {
			return ev, err
		}
		ev.Suites = append(ev.Suites, res)
	}
	ev.End = time.Now().UTC().Format(time.RFC3339)
	finalizeRepairEvidence(ev)

	if opts.Out != "" {
		if err := WriteRepairEvidence(opts.Out, ev); err != nil {
			return ev, err
		}
	}
	return ev, nil
}

// finalizeRepairEvidence computes the aggregate verdicts: Complete only when
// every ELIGIBLE (non-skipped) suite ran to completion, Pass only when
// complete and all of them passed, and fail closed when nothing ran at all.
func finalizeRepairEvidence(ev *RepairProfileEvidence) {
	ran := 0
	complete, pass := true, true
	for _, s := range ev.Suites {
		if s.Skipped {
			continue
		}
		ran++
		complete = complete && s.Complete
		pass = pass && s.Complete && s.Pass
	}
	ev.Complete = ran > 0 && complete
	ev.Pass = ev.Complete && pass
	if ran == 0 {
		ev.Reason = "no suite ran on this host (fail closed — a profile that measured nothing never passes)"
	}
}

func skippedRepairSuite(spec RepairSuiteSpec, detail string) RepairSuiteResult {
	cmd := spec.Command
	if cmd == "" {
		cmd = strings.Join(spec.Args, " ")
	}
	return RepairSuiteResult{
		Name: spec.Name, Lane: spec.Lane, Command: cmd,
		Skipped:  true,
		Complete: false, Pass: false,
		Reason: "skipped (lane unavailable: " + detail + ")",
	}
}

func suiteNames(specs []RepairSuiteSpec) []string {
	var names []string
	for _, s := range specs {
		names = append(names, s.Name)
	}
	return names
}

// runRepairSuite runs one suite under a wall-time bound and captures its
// combined output into a bounded tail plus parseable counts.
func runRepairSuite(spec RepairSuiteSpec, opts RepairProfileOptions) (RepairSuiteResult, error) {
	res := RepairSuiteResult{Name: spec.Name, Lane: spec.Lane}

	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = opts.Timeout
	}
	if timeout <= 0 {
		timeout = repairDefaultTimeout
	}

	argv := spec.Args
	display := spec.Command
	if display == "" {
		display = strings.Join(spec.Args, " ")
	}
	switch spec.Kind {
	case "go-test-list":
		expanded, cmdDisplay, err := expandGoTestList(opts.Root, spec)
		if err != nil {
			res.Command = display
			res.Reason = "go list: " + err.Error()
			return res, nil // a failed expansion is a failed suite, not a runner error
		}
		argv, display = expanded, cmdDisplay
	case "argv":
		if len(argv) == 0 {
			res.Command = display
			res.Reason = "suite has no command"
			return res, nil
		}
	default:
		res.Command = display
		res.Reason = fmt.Sprintf("unknown suite kind %q", spec.Kind)
		return res, nil
	}

	eff, resolved, err := resolveRepairArgv(argv)
	if err != nil {
		res.Command = display
		res.Reason = "start: " + err.Error()
		return res, nil
	}
	if resolved {
		display = strings.Join(eff, " ")
	}
	res.Command = display

	capw := newSuiteCapture(opts.TailBytes)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, eff[0], eff[1:]...)
	cmd.Dir = opts.Root
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Stdout = capw
	cmd.Stderr = capw
	// Bound the pipe-drain after a watchdog kill: grandchildren that inherit
	// the pipes cannot hold the evidence hostage past the suite's own death.
	cmd.WaitDelay = 30 * time.Second
	runErr := cmd.Run()
	res.DurationMS = time.Since(start).Milliseconds()

	res.Counts = capw.countsOrNil()
	res.OutputTail, res.OutputTruncated = capw.tail()

	if ctx.Err() == context.DeadlineExceeded {
		res.Reason = fmt.Sprintf("timeout: killed after %s (incomplete run, never a pass)", timeout)
		return res, nil
	}
	// ErrWaitDelay: the suite itself exited, only orphaned pipe holders were
	// cut loose — the suite's own exit code stands.
	if runErr == nil || (errors.Is(runErr, exec.ErrWaitDelay) && cmd.ProcessState != nil) {
		code := cmd.ProcessState.ExitCode()
		res.ExitCode = &code
		res.Complete = true
		res.Pass = code == 0
		if code != 0 {
			res.Reason = fmt.Sprintf("exit status %d", code)
		}
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		code := exitErr.ExitCode()
		if code == -1 { // killed by a signal before exiting on its own
			res.Signal = cmd.ProcessState.String()
			res.Reason = "killed by signal (" + res.Signal + ") — incomplete run, never a pass"
			return res, nil
		}
		// Through the bashy multicall fallback, 127 is the dispatch layer
		// saying the applet does not exist: the suite never started.
		if resolved && code == 127 {
			res.ExitCode = &code
			res.Reason = fmt.Sprintf("start: %s not found on PATH and not a bashy applet (exit 127)", argv[0])
			return res, nil
		}
		res.ExitCode = &code
		res.Complete = true
		res.Pass = false
		res.Reason = fmt.Sprintf("exit status %d", code)
		return res, nil
	}
	res.Reason = "start: " + runErr.Error()
	return res, nil
}

// expandGoTestList resolves a go-test-list suite into a concrete go test
// argv: `go list ./...` in the component root minus the excluded packages
// (fail closed on an empty package set), then go test with the spec's flags.
func expandGoTestList(root string, spec RepairSuiteSpec) ([]string, string, error) {
	goArgv, _, err := resolveRepairArgv([]string{"go", "list", "./..."})
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goArgv[0], goArgv[1:]...)
	cmd.Dir = root
	cmd.WaitDelay = 30 * time.Second
	out, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("go list ./... failed in %s: %v", root, err)
	}
	var pkgs []string
	for _, ln := range strings.Split(string(out), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		excluded := false
		for _, ex := range spec.Exclude {
			if strings.Contains(ln, ex) {
				excluded = true
				break
			}
		}
		if !excluded {
			pkgs = append(pkgs, ln)
		}
	}
	if len(pkgs) == 0 {
		return nil, "", fmt.Errorf("no packages left after excluding %v in %s (fail closed)", spec.Exclude, root)
	}
	argv := append([]string{"go", "test"}, spec.GoTestArgs...)
	argv = append(argv, pkgs...)
	return argv, strings.Join(argv, " "), nil
}

// resolveRepairArgv resolves argv[0] on PATH. When the program is absent but
// a bashy is on PATH, the suite runs through bashy's multicall (bashy go /
// bashy make …) — on a QA host bashy IS the userland (see ci.dag.md). The
// display string always shows the effective command.
func resolveRepairArgv(argv []string) ([]string, bool, error) {
	if _, err := exec.LookPath(argv[0]); err == nil {
		return argv, false, nil
	}
	if _, err := exec.LookPath("bashy"); err == nil {
		eff := append([]string{"bashy"}, argv...)
		return eff, true, nil
	}
	return nil, false, fmt.Errorf("%s not found on PATH (and no bashy to dispatch it)", argv[0])
}

// gitCommit reports the component checkout's HEAD ("unknown" when git cannot
// answer — the evidence records the fact instead of failing).
func gitCommit(root string) string {
	cmd := exec.Command("git", "rev-parse", "--short=7", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	c := strings.TrimSpace(string(out))
	if c == "" {
		return "unknown"
	}
	return c
}

// WriteRepairEvidence writes the record atomically (temp file + rename in the
// target directory) so a gate never reads a half-written evidence file.
func WriteRepairEvidence(path string, ev *RepairProfileEvidence) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".repair-profile-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// ensureRepairFixtures makes the SHA-pinned bash-5.3 fixture tree available,
// reusing the fetch-by-SHA user cache the test-bash lane owns
// (EnsureBash53Fixtures). Offline rule: BASHY_OFFLINE=1 with an empty cache
// fails clearly instead of reaching for the network.
func ensureRepairFixtures(root, cacheBase string, offline bool) error {
	link := filepath.Join(root, "external", "bash-5.3")
	if fi, err := os.Stat(filepath.Join(link, "tests")); err == nil && fi.IsDir() {
		return nil
	}
	if cacheBase == "" {
		d, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		cacheBase = d
	}
	dst := filepath.Join(cacheBase, "bashy", "conformance", "bash-5.3")
	if !validBash53Cache(dst) && offline {
		return fmt.Errorf("bash-5.3 fixture cache is empty at %s and BASHY_OFFLINE=1: "+
			"populate it once online (make test-bash-fixtures) — refusing to fetch", dst)
	}
	if _, err := EnsureBash53Fixtures(root); err != nil {
		return fmt.Errorf("bash-5.3 fixtures: %w", err)
	}
	return nil
}

// suiteCapture is the suite-output sink: it scans the FULL stream for
// parseable counts while retaining only the last cap bytes for the evidence
// tail (memory-bounded for arbitrarily chatty suites).
type suiteCapture struct {
	mu        sync.Mutex
	cap       int
	ring      []byte
	total     int64
	counts    RepairCounts
	seenCount bool
}

func newSuiteCapture(capBytes int) *suiteCapture {
	if capBytes <= 0 {
		capBytes = repairDefaultTailBytes
	}
	return &suiteCapture{cap: capBytes}
}

func (c *suiteCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total += int64(len(p))
	c.ring = append(c.ring, p...)
	if len(c.ring) > c.cap {
		c.ring = c.ring[len(c.ring)-c.cap:]
	}
	c.scan(p)
	return len(p), nil
}

// scan counts go-test verdict lines and the bash53suite Results summary.
func (c *suiteCapture) scan(p []byte) {
	for _, ln := range strings.Split(string(p), "\n") {
		ln = strings.TrimRight(ln, "\r")
		switch {
		case strings.Contains(ln, "--- PASS:"):
			c.counts.Pass++
			c.seenCount = true
		case strings.Contains(ln, "--- FAIL:"):
			c.counts.Fail++
			c.seenCount = true
		case strings.Contains(ln, "--- SKIP:"):
			c.counts.Skip++
			c.seenCount = true
		case strings.HasPrefix(ln, "Results:"):
			var passed, failed, skipped, timedOut int
			if n, _ := fmt.Sscanf(ln, "Results: %d passed, %d failed, %d skipped, %d timed out", &passed, &failed, &skipped, &timedOut); n == 4 {
				c.counts.Pass += passed
				c.counts.Fail += failed + timedOut // a timed-out fixture is not a pass
				c.counts.Skip += skipped
				c.seenCount = true
			}
		}
	}
}

func (c *suiteCapture) countsOrNil() *RepairCounts {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.seenCount {
		return nil
	}
	cp := c.counts
	return &cp
}

func (c *suiteCapture) tail() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.ring), c.total > int64(len(c.ring))
}

// RepairPlanRow is one line of the -list plan.
type RepairPlanRow struct {
	Name    string `json:"name"`
	Lane    string `json:"lane"`
	Command string `json:"command"`
	Status  string `json:"status"` // "run" or "skipped (lane unavailable: …)"
}

// ListRepairProfile prints (and returns) the per-host plan for a component:
// which suites would run natively on this OS and the landmine lane's
// availability — the dry-run view of the recipe.
func ListRepairProfile(opts RepairProfileOptions, w io.Writer) ([]RepairPlanRow, error) {
	if opts.Goos == "" {
		opts.Goos = runtime.GOOS
	}
	if opts.Arch == "" {
		opts.Arch = runtime.GOARCH
	}
	if opts.Root == "" {
		opts.Root = "."
	}
	specs := opts.CustomSuites
	if specs == nil {
		var err error
		specs, err = RepairSuitesFor(opts.Component)
		if err != nil {
			return nil, err
		}
	}
	if len(opts.Only) > 0 {
		want := map[string]bool{}
		for _, n := range opts.Only {
			want[n] = true
		}
		var kept []RepairSuiteSpec
		for _, s := range specs {
			if want[s.Name] {
				kept = append(kept, s)
			}
		}
		if len(kept) == 0 {
			return nil, fmt.Errorf("repair profile: -only selected no suite (have %v)", suiteNames(specs))
		}
		specs = kept
	}
	laneProbe := opts.LaneProbe
	if laneProbe == nil {
		probe := DefaultRepairLaneProbe(opts.Root)
		laneProbe = func() (bool, string) {
			if opts.Goos != "linux" {
				return false, "landmine suites run only in the Linux oci lane (host is " + opts.Goos + ")"
			}
			return probe()
		}
	}
	fmt.Fprintf(w, "component=%s os=%s arch=%s\n", opts.Component, opts.Goos, opts.Arch)
	var rows []RepairPlanRow
	for _, s := range specs {
		cmd := s.Command
		if cmd == "" {
			cmd = strings.Join(s.Args, " ")
		}
		status := "run"
		if s.Lane == RepairLaneOCILinux {
			if ok, reason := laneProbe(); !ok {
				status = "skipped (lane unavailable: " + reason + ")"
			}
		}
		fmt.Fprintf(w, "%-10s %-22s %s  [%s]\n", s.Lane, s.Name, cmd, status)
		rows = append(rows, RepairPlanRow{Name: s.Name, Lane: s.Lane, Command: cmd, Status: status})
	}
	return rows, nil
}
