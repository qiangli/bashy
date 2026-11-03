// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Repair profiles (S549.R4a, R4b): for each core component, one command runs
// its full repair profile natively on the host it runs on — Linux, macOS or
// Windows, like the release gate — and writes the evidence record the repair
// gate (S549.R3a `src pr`) consumes. Rules of record (docs/repair-profiles.md):
//
//   - every suite runs INSIDE the bashy shell — `<bashy> -c '<suite command>'`
//     — where make, go and cc resolve to bashy's own commands/providers,
//     never host binaries ("bashy is all you need"); the evidence records
//     which bashy ran (path + version) and which provider each tool resolved
//     to;
//   - the suite bin dir also carries bashy's own POSIX/coreutils utilities
//     as links to the running bashy (argv[0] multicall dispatch; `sh` takes
//     the shell route), so an external `/bin/sh -c "sleep 30"` finds sleep
//     on the suite PATH of a bare host — the evidence records count + bin
//     dir (runtime.posix_links);
//   - a run with the network disabled passes from the SHA-pinned cache;
//   - a killed, partial or truncated run is a failure, never a pass;
//   - GPL suites are fetched by SHA into the gitignored user cache, never
//     vendored (the same cache `make test-bash` uses — see EnsureBash53Fixtures);
//   - VSC/PCTS is excluded (licensed; a different lane, never auto-fetched);
//   - only landmine suites (uutils) run in the Linux oci sandbox lane, and a
//     structurally unavailable lane is reported "skipped (lane unavailable)",
//     never "pass";
//   - a suite that needs a tool the bashy runtime cannot provide is reported
//     "skipped (tool <x> unavailable in bashy runtime)" — never pass, and the
//     missing tool is named.

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
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/yoke/external/gotoolchain"
	"github.com/qiangli/yoke/pkg/atlas"
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

	// Tools names the tools the suite needs resolved in the bashy runtime
	// (canonical spellings: "make", "go", "cc", "c++"). A tool the runtime
	// cannot provide skips the suite — never pass — naming the tool.
	Tools []string

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
// unavailable, tool unavailable) suites are recorded, never counted as pass.
type RepairProfileEvidence struct {
	Schema    string              `json:"schema"`
	Component string              `json:"component"`
	Commit    string              `json:"commit"`
	OS        string              `json:"os"`
	Arch      string              `json:"arch"`
	Runtime   *RepairRuntime      `json:"runtime,omitempty"`
	Start     string              `json:"start"`
	End       string              `json:"end"`
	Suites    []RepairSuiteResult `json:"suites"`
	Complete  bool                `json:"complete"`
	Pass      bool                `json:"pass"`
	Reason    string              `json:"reason,omitempty"`
}

// RepairBashyIdent records which bashy ran every suite (S549.R4b): the
// absolute path of the executable the runner shelled out to, and the first
// line of its `--version` banner.
type RepairBashyIdent struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
}

// RepairToolResolution records how one tool resolved inside the bashy runtime
// that ran the suites — the provider bashy served it from, and the route
// (in-process shell handler, or the wrapper unit materialized on the suite's
// private PATH).
type RepairToolResolution struct {
	Tool     string `json:"tool"`
	Provider string `json:"provider"`
	Via      string `json:"via"`
}

// RepairRuntime is the S549.R4b evidence block: the bashy every suite ran
// inside, the provider each tool the profile needs resolved to, and the
// POSIX/coreutils links that carried the suite PATH's utilities.
type RepairRuntime struct {
	Bashy      RepairBashyIdent       `json:"bashy"`
	Tools      []RepairToolResolution `json:"tools,omitempty"`
	PosixLinks *RepairPosixLinks      `json:"posix_links,omitempty"`
}

// RepairPosixLinks is the S549.R4b rework evidence: the suite bin dir carries
// bashy's POSIX/coreutils utilities as links to the running bashy executable,
// so a bare host's external shells still find sh/sleep/ls/head/… through
// bashy's own userland — never a host binary, never a per-name copy of the
// ~250 MB binary. The bin dir is per-suite and removed after the run; the
// record proves provenance, not a live path.
type RepairPosixLinks struct {
	Count  int    `json:"count"`
	BinDir string `json:"bin_dir,omitempty"`
	Kind   string `json:"kind,omitempty"` // symlink (unix) · hard link .exe / cmd shim fallback (windows)
}

// RepairProfileOptions parameterizes RunRepairProfile.
type RepairProfileOptions struct {
	Component    string
	Root         string                // component checkout; suite commands run with Dir=Root
	Bashy        string                // pinned bashy every suite runs under ("" = resolve: this executable when built into bashy, else PATH)
	Commit       string                // recorded verbatim when set (git archive trees carry no .git; -commit / BASHY_REPAIR_COMMIT)
	Env          []string              // base environment for suites (nil = os.Environ(); tests strip PATH here)
	UnitsDir     string                // Bash# wrapper units directory ("" = BASHY_REPAIR_UNITS, else ci/bin found upward from the cwd)
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
				Args: []string{"make", "test"}, Tools: []string{"make"},
				Timeout: repairDefaultTimeout,
				Command: "make test",
			},
			{
				Name: "test-bash", Lane: RepairLaneNative, Kind: "argv",
				Args: []string{"make", "test-bash"}, Tools: []string{"make"},
				Timeout:  repairDefaultTimeout,
				Fixtures: true, // SHA-pinned GPL fixture cache, never vendored
				Command:  "make test-bash",
			},
			{
				Name: "test-uutils", Lane: RepairLaneOCILinux, Kind: "argv",
				Args: []string{"make", "test-uutils"}, Tools: []string{"make"},
				Timeout: repairDefaultTimeout,
				Command: "make test-uutils",
			},
		}, nil
	case "outpost":
		return []RepairSuiteSpec{
			{
				Name: "go-test-short-headless", Lane: RepairLaneNative, Kind: "go-test-list",
				Tools:      []string{"go"},
				GoTestArgs: []string{"-short", "-tags", "fb_archives"},
				Exclude:    []string{"internal/agent/shell"}, // needs a real TTY
				// Test what ships: outpost releases are CGO_ENABLED=0
				// (release.yml, scripts/build.sh). A cgo build would need a
				// platform SDK (e.g. macOS CoreServices) no runtime provides.
				Env:     []string{"CGO_ENABLED=0"},
				Timeout: repairDefaultTimeout,
				Command: "CGO_ENABLED=0 go test -short -tags fb_archives $(go list ./... | grep -v internal/agent/shell)",
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
		Commit: repairCommit(opts), OS: opts.Goos, Arch: opts.Arch,
		Start: start.Format(time.RFC3339),
	}

	// S549.R4b: every suite runs inside the bashy shell. Resolve that bashy
	// once, up front — no bashy, no profile (never silently fall back to host
	// make/go). The runtime block is evidence, not decoration: it names the
	// executable that ran every suite and the provider each tool resolved to.
	bashy, bashyErr := resolveRepairBashy(opts.Bashy)
	if bashyErr == nil {
		ev.Runtime = &RepairRuntime{Bashy: RepairBashyIdent{
			Path: bashy, Version: repairBashyVersion(bashy),
		}}
	} else {
		ev.Runtime = &RepairRuntime{}
	}

	// The wrapper units behind the tool table: their absence makes every
	// unit-backed tool unavailable (suite-level skip, never pass).
	units := repairUnitsIndex(findRepairUnitsDir(opts))

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

	recordedTools := map[string]bool{}
	recordTool := func(name string) {
		if recordedTools[name] {
			return
		}
		recordedTools[name] = true
		if bashyErr == nil {
			if row, canonical, ok := repairToolRowFor(name); ok && units.rowAvailable(canonical) {
				via := row.via
				if name != canonical && row.unitPath() != "" {
					via = "wrapper unit ci/bin/" + row.unitPath() + " as " + name + " (" + row.via + ")"
				}
				ev.Runtime.Tools = append(ev.Runtime.Tools, RepairToolResolution{
					Tool: name, Provider: row.provider, Via: via,
				})
				return
			}
		}
		ev.Runtime.Tools = append(ev.Runtime.Tools, RepairToolResolution{
			Tool: name, Provider: "unavailable in the bashy runtime", Via: "-",
		})
	}
	// CC/CXX default to the wrapper units for every suite, so the compilers
	// are evidence even when no suite names them explicitly.
	recordTool("cc")
	recordTool("c++")

	for _, spec := range specs {
		for _, tool := range spec.Tools {
			recordTool(tool)
		}
		if spec.Lane == RepairLaneOCILinux {
			ok, reason := laneProbe()
			if !ok {
				ev.Suites = append(ev.Suites, skippedRepairSuite(spec, reason))
				continue
			}
		}
		if bashyErr != nil {
			ev.Suites = append(ev.Suites, RepairSuiteResult{
				Name: spec.Name, Lane: spec.Lane, Command: repairDisplayCommand(spec),
				Reason: "not run: " + bashyErr.Error(),
			})
			continue
		}
		if missing := repairMissingTool(spec.Tools, units); missing != "" {
			ev.Suites = append(ev.Suites, skippedRepairToolSuite(spec, missing))
			continue
		}
		res, err := runRepairSuite(spec, opts, bashy, units, ev.Runtime)
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

// repairDisplayCommand is the evidence command string for a spec before any
// expansion.
func repairDisplayCommand(spec RepairSuiteSpec) string {
	if spec.Command != "" {
		return spec.Command
	}
	return repairCommandString(spec.Args)
}

// runRepairSuite runs one suite under a wall-time bound and captures its
// combined output into a bounded tail plus parseable counts.
//
// S549.R4b: the suite NEVER runs as a host process — the runner executes
// `<bashy> -c '<suite command>'`, and inside that shell make, go and cc
// resolve to bashy's own commands/providers (coreutils in-process make;
// wrapper units from ci/bin on the suite's private PATH for go/cc/c++),
// never host binaries. The S549.R4b rework adds the POSIX/coreutils links:
// the same private PATH carries bashy's own sh/sleep/ls/…, so external
// shells a suite spawns find the POSIX utilities on a bashy-only host.
func runRepairSuite(spec RepairSuiteSpec, opts RepairProfileOptions, bashy string, units repairUnits, rt *RepairRuntime) (RepairSuiteResult, error) {
	res := RepairSuiteResult{Name: spec.Name, Lane: spec.Lane}

	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = opts.Timeout
	}
	if timeout <= 0 {
		timeout = repairDefaultTimeout
	}

	if missing := repairMissingTool(spec.Tools, units); missing != "" {
		return skippedRepairToolSuite(spec, missing), nil
	}

	bin, err := materializeRepairShims(units, bashy)
	if bin.path != "" {
		defer os.RemoveAll(bin.path)
	}
	if err != nil {
		res.Command = repairDisplayCommand(spec)
		res.Reason = "bashy runtime wrappers: " + err.Error()
		return res, nil
	}
	if bin.path != "" && rt != nil {
		// The bin dir is per-suite and cleaned up after the run; the evidence
		// records the provenance (count + a representative bin dir), not a
		// promise the path still exists.
		rt.PosixLinks = &RepairPosixLinks{Count: bin.posix, BinDir: bin.path, Kind: bin.posixKind}
	}

	command := repairDisplayCommand(spec)
	argv := spec.Args
	switch spec.Kind {
	case "go-test-list":
		expanded, cmdDisplay, err := expandGoTestList(opts.Root, spec, bashy, bin.path, repairSuiteEnv(opts, spec, bin.path))
		if err != nil {
			res.Command = command
			res.Reason = "go list: " + err.Error()
			return res, nil // a failed expansion is a failed suite, not a runner error
		}
		argv, command = expanded, cmdDisplay
	case "argv":
		if len(argv) == 0 {
			res.Command = command
			res.Reason = "suite has no command"
			return res, nil
		}
		command = repairCommandString(argv)
	default:
		res.Command = command
		res.Reason = fmt.Sprintf("unknown suite kind %q", spec.Kind)
		return res, nil
	}
	res.Command = command

	// THE rule of R4b: the suite runs inside the bashy shell, as
	// `<bashy> -c '<command>'` — never `make`/`go` exec'd directly.
	invocation := []string{bashy, "-c", command}
	capw := newSuiteCapture(opts.TailBytes)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, invocation[0], invocation[1:]...)
	cmd.Dir = opts.Root
	cmd.Env = repairSuiteEnv(opts, spec, bin.path)
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
		if code == -1 { // the bashy shell itself was killed by a signal
			res.Signal = cmd.ProcessState.String()
			res.Reason = "killed by signal (" + res.Signal + ") — incomplete run, never a pass"
			return res, nil
		}
		// Any other non-zero status is the suite's own verdict as reported by
		// the bashy shell — including 127 (a name that resolved nowhere
		// inside the runtime), which is a failed suite, never a pass.
		res.ExitCode = &code
		res.Complete = true
		res.Pass = false
		res.Reason = fmt.Sprintf("exit status %d", code)
		return res, nil
	}
	res.Reason = "start: " + runErr.Error()
	return res, nil
}

// repairCommandString renders a suite argv as the shell text handed to
// `<bashy> -c`: plain words stay readable, anything the shell would reparse
// is single-quoted.
func repairCommandString(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		if repairPlainWord(a) {
			parts = append(parts, a)
		} else {
			parts = append(parts, shellQuote(a))
		}
	}
	return strings.Join(parts, " ")
}

var repairPlainWordRe = regexp.MustCompile(`^[A-Za-z0-9_./:=,+@%-]+$`)

func repairPlainWord(s string) bool {
	return s != "" && repairPlainWordRe.MatchString(s)
}

// repairSuiteEnv assembles the suite process environment: the base env
// (opts.Env, else the runner's own), the spec's overrides, then the R4b
// runtime rules — the suite bin dir (wrapper units + the POSIX/coreutils
// links) FIRST on PATH with the host PATH as a trailing fallback, so go/cc
// and every POSIX utility inside the suite shell resolve to bashy's own
// before any host binary, and CC/CXX pointing at the wrapper units when the
// environment provides no compiler. Later entries win, so spec.Env genuinely
// overrides the base.
func repairSuiteEnv(opts RepairProfileOptions, spec RepairSuiteSpec, shimDir string) []string {
	base := opts.Env
	if base == nil {
		base = os.Environ()
	}
	kv := map[string]string{}
	var order []string
	set := func(k, v string) {
		if _, seen := kv[k]; !seen {
			order = append(order, k)
		}
		kv[k] = v
	}
	apply := func(entries []string) {
		for _, e := range entries {
			k, v, _ := strings.Cut(e, "=")
			set(k, v)
		}
	}
	apply(base)
	apply(spec.Env)
	if shimDir != "" {
		if p := kv["PATH"]; p == "" {
			set("PATH", shimDir)
		} else {
			set("PATH", shimDir+string(os.PathListSeparator)+p)
		}
		if _, ok := kv["CC"]; !ok {
			set("CC", filepath.Join(shimDir, repairShimName("cc")))
		}
		if _, ok := kv["CXX"]; !ok {
			set("CXX", filepath.Join(shimDir, repairShimName("c++")))
		}
	}
	out := make([]string, 0, len(order))
	for _, k := range order {
		out = append(out, k+"="+kv[k])
	}
	return out
}

// repairShimName spells a shim file name the platform can execute directly:
// on Windows cgo's CreateProcess needs a PATHEXT suffix, so the compiler
// wrappers carry a .cmd twin (cmd.exe ships with Windows — nothing is
// installed); elsewhere the Bash# unit itself is the executable.
func repairShimName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".cmd"
	}
	return tool
}

// expandGoTestList resolves a go-test-list suite into a concrete go test
// argv: `go list ./...` in the component root minus the excluded packages
// (fail closed on an empty package set), then go test with the spec's flags.
// The `go list` probe itself runs through the bashy runtime —
// `<bashy> -c 'go list ./...'` with the suite's wrapper PATH — so it never
// execs a host go either.
func expandGoTestList(root string, spec RepairSuiteSpec, bashy, shimDir string, env []string) ([]string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bashy, "-c", "go list ./...")
	cmd.Dir = root
	cmd.Env = env
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

// --- the bashy runtime every suite runs inside (S549.R4b) ---

// resolveRepairBashy pins the bashy executable every suite runs under. Order:
// an explicit value (the runner's -bashy flag / opts.Bashy), then the current
// executable when the runner is built into bashy, then $BASHY_SELF, then
// `bashy` on PATH. No hit is a hard error — the profile never silently falls
// back to host make/go.
func resolveRepairBashy(explicit string) (string, error) {
	candidates := []string{explicit}
	if explicit == "" {
		if RunningAsBashy {
			if exe, err := os.Executable(); err == nil {
				candidates = append(candidates, exe)
			}
		}
		if self := strings.TrimSpace(os.Getenv("BASHY_SELF")); self != "" {
			candidates = append(candidates, self)
		}
		if p, err := exec.LookPath("bashy"); err == nil {
			candidates = append(candidates, p)
		}
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("no bashy runtime found — every repair suite runs inside the bashy shell; pass -bashy <path> or put bashy on PATH")
}

// repairBashyVersion records the first line of the pinned bashy's --version
// banner (best-effort; "" when the probe cannot run).
func repairBashyVersion(bashy string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bashy, "--version")
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	if i := strings.IndexByte(v, '\n'); i >= 0 {
		v = v[:i]
	}
	return v
}

// repairCommit is the evidence commit: an explicit -commit /
// BASHY_REPAIR_COMMIT wins (a `git archive` tree has no .git to ask), else
// the component checkout's HEAD, else the recorded fact "unknown".
func repairCommit(opts RepairProfileOptions) string {
	if c := strings.TrimSpace(opts.Commit); c != "" {
		return c
	}
	return gitCommit(opts.Root)
}

// repairToolRow describes one tool the bashy runtime provides for suites.
type repairToolRow struct {
	unit     string   // unit stem under ci/bin ("" = no unit: served in-process)
	aliases  []string // shim names materialized from the unit
	provider string   // evidence label
	via      string   // evidence route
}

func (r repairToolRow) unitPath() string {
	if r.unit == "" {
		return ""
	}
	return r.unit + ".md"
}

// repairBashyToolRows is the tool table of the bashy runtime: what suites may
// ask for by name, and where each resolves. make is the in-process coreutils
// applet of the shell itself; go/cc/c++ are the pinned providers reached
// through the Bash# wrapper units in ci/bin (materialized onto the suite's
// private PATH, so bare `go`/`cc` inside a suite — including from a Makefile
// recipe or cgo — is bashy's own, never a host binary).
var repairBashyToolRows = map[string]repairToolRow{
	"make": {
		provider: "bashy make (in-process coreutils make)",
		via:      "bashy shell ExecHandler, ahead of PATH",
	},
	"go": {
		unit:     "go",
		aliases:  []string{"go"},
		provider: "bashy go (pinned provider go" + gotoolchain.DefaultVersion + ")",
		via:      "wrapper unit ci/bin/go.md on the suite PATH",
	},
	"cc": {
		unit:     "cc",
		aliases:  []string{"cc", "gcc", "clang"},
		provider: "bashy zig cc (pinned zig provider; zig cc is clang)",
		via:      "wrapper unit ci/bin/cc.md on the suite PATH (CC default)",
	},
	"c++": {
		unit:     "cxx",
		aliases:  []string{"c++", "g++", "clang++"},
		provider: "bashy zig c++ (pinned zig provider)",
		via:      "wrapper unit ci/bin/cxx.md on the suite PATH (CXX default)",
	},
}

// repairToolRowFor resolves a tool name (canonical or alias) to its row.
func repairToolRowFor(name string) (row repairToolRow, canonical string, ok bool) {
	if r, hit := repairBashyToolRows[name]; hit {
		return r, name, true
	}
	for canon, r := range repairBashyToolRows {
		for _, a := range r.aliases {
			if a == name {
				return r, canon, true
			}
		}
	}
	return repairToolRow{}, "", false
}

// repairUnits is the parsed ci/bin wrapper-unit set available to a run.
type repairUnits struct {
	dir    string
	bodies map[string]string // unit stem ("cc") → substituted Bash# body
	runs   map[string]string // unit stem → front-matter run words ("zig cc")
}

// findRepairUnitsDir locates the Bash# wrapper units: opts.UnitsDir or
// $BASHY_REPAIR_UNITS when set (authoritative — a bad path means no units,
// never a silent fallback), else ci/bin found by walking up from the working
// directory (a `go run ./tools/repairprofile` or `go test` invocation starts
// inside the checkout). "" means no units on disk.
func findRepairUnitsDir(opts RepairProfileOptions) string {
	explicit := ""
	for _, c := range []string{opts.UnitsDir, os.Getenv("BASHY_REPAIR_UNITS")} {
		if c = strings.TrimSpace(c); c == "" {
			continue
		}
		if fi, err := os.Stat(filepath.Join(c, "go.md")); err == nil && !fi.IsDir() {
			return c
		}
		explicit = c
	}
	if explicit != "" {
		return ""
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for i := 0; i < 12; i++ {
		cand := filepath.Join(dir, "ci", "bin")
		if fi, err := os.Stat(filepath.Join(cand, "go.md")); err == nil && !fi.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

var repairUnitFence = regexp.MustCompile("(?s)```(?:bashpp|bsh|bash)\\s*\\n(.*?)```")

// repairUnitsIndex reads every unit the tool table needs from dir. Missing
// dir or missing files leave the corresponding tools unavailable (suites
// needing them skip, never pass) — a run without its wrappers never pretends.
func repairUnitsIndex(dir string) repairUnits {
	units := repairUnits{dir: dir, bodies: map[string]string{}, runs: map[string]string{}}
	if dir == "" {
		return units
	}
	for _, row := range repairBashyToolRows {
		if row.unit == "" {
			continue
		}
		path := filepath.Join(dir, row.unitPath())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		m := repairUnitFence.FindStringSubmatch(string(b))
		if m == nil || !strings.Contains(m[1], "{{bashy}}") {
			continue
		}
		units.bodies[row.unit] = strings.TrimSpace(m[1])
		for _, ln := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(ln), "run:"); ok {
				units.runs[row.unit] = strings.TrimSpace(v)
			}
		}
	}
	return units
}

// rowAvailable reports whether the unit backing a canonical tool row is on
// disk and parsed.
func (u repairUnits) rowAvailable(canonical string) bool {
	row, ok := repairBashyToolRows[canonical]
	if !ok {
		return false
	}
	if row.unit == "" {
		return true // served by the shell itself, no unit needed
	}
	_, have := u.bodies[row.unit]
	return have
}

// repairMissingTool returns the first tool in needs the bashy runtime cannot
// provide (unknown name, or a wrapper unit missing/unparsable), or "".
func repairMissingTool(needs []string, units repairUnits) string {
	for _, name := range needs {
		_, canonical, ok := repairToolRowFor(name)
		if !ok || !units.rowAvailable(canonical) {
			return name
		}
	}
	return ""
}

// skippedRepairToolSuite is the S549.R4b skip record: the suite needs a tool
// the bashy runtime cannot provide — skipped, never pass, tool named.
func skippedRepairToolSuite(spec RepairSuiteSpec, tool string) RepairSuiteResult {
	return RepairSuiteResult{
		Name: spec.Name, Lane: spec.Lane, Command: repairDisplayCommand(spec),
		Skipped:  true,
		Complete: false, Pass: false,
		Reason: fmt.Sprintf("skipped (tool %s unavailable in bashy runtime)", tool),
	}
}

// --- bashy's POSIX/coreutils utilities on the suite PATH (S549.R4b rework) ---

// repairPosixUtilNames returns the POSIX/coreutils utility names the suite
// bin dir links to the running bashy. The source of record is the data
// behind `bashy commands --view posix`: the 116 POSIX-required utilities
// (atlas.PosixRequired) plus the GNU coreutils inventory
// (atlas.GNUCoreutilsUpstream) — never a second hardcoded list. Two filters:
// a name becomes a link only when the bashy binary dispatches it on argv[0]
// (the same multicall registry lookup cmd/bashy's route does, plus the
// sh shell route; shell builtins like cd/alias/wait are served in-process by
// any shell, and a name bashy cannot dispatch would be a broken link
// shadowing a working host binary on the suite PATH), and never a name a
// wrapper unit already provides — the wrappers stay authoritative
// (go/cc/c++ and their spellings).
func repairPosixUtilNames() []string {
	reserved := map[string]bool{}
	for _, row := range repairBashyToolRows {
		if row.unit == "" {
			continue
		}
		for _, a := range row.aliases {
			reserved[a] = true
		}
	}
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if seen[n] || reserved[n] {
			return
		}
		if n != "sh" && n != "bash" && tool.Lookup(n) == nil {
			return
		}
		seen[n] = true
		names = append(names, n)
	}
	for _, n := range atlas.PosixRequired() {
		add(n)
	}
	for _, n := range atlas.GNUCoreutilsUpstream() {
		add(n)
	}
	// bashy is the shell too: suites and the code under test exec "bash"
	// and "sh" by name, and on a bashy-only host those are bashy itself.
	add("sh")
	add("bash")
	sort.Strings(names)
	return names
}

// The POSIX-utility link kinds the evidence records.
const (
	repairLinkSymlink  = "symlink"
	repairLinkHardLink = "hard link"
	repairLinkCmdShim  = "cmd shim"
)

// repairOSLinks is the filesystem seam the POSIX-utility links go through.
// The production instance wraps the os package; tests substitute fakes to
// pin the Windows link-or-shim decision without a Windows host.
type repairOSLinks struct {
	goos      string
	symlink   func(target, link string) error
	hardlink  func(target, link string) error
	writeFile func(path string, body []byte) error
}

// repairUtilLinks is the production seam instance.
var repairUtilLinks = repairOSLinks{
	goos:      runtime.GOOS,
	symlink:   os.Symlink,
	hardlink:  os.Link,
	writeFile: func(path string, body []byte) error { return os.WriteFile(path, body, 0o644) },
}

// repairWriteUtilLink materializes one utility name that runs the pinned
// bashy's own userland, dispatching on argv[0]:
//
//   - unix: a symlink — free to create, and the executed link's argv[0]
//     names the utility (multicall dispatch; a `sh` link takes the
//     shell-only route);
//   - windows: a same-volume hard link with the .exe suffix — CreateProcess
//     runs it directly and argv[0] still names the utility; a hard link that
//     cannot be made (cross-volume temp dir, a link-less filesystem) falls
//     back to a small .cmd shim naming the utility (`bashy <name> %*`;
//     cmd.exe ships with the OS, nothing is installed).
//
// Never a copy: the binary is ~250 MB and every name shares one inode.
// Returns the evidence kind of the entry.
func repairWriteUtilLink(l repairOSLinks, dir, name, bashy string) (string, error) {
	if l.goos == "windows" {
		if err := l.hardlink(bashy, filepath.Join(dir, name+".exe")); err == nil {
			return repairLinkHardLink, nil
		}
		body := "@\"" + bashy + "\" " + name + " %*\r\n"
		return repairLinkCmdShim, l.writeFile(filepath.Join(dir, name+".cmd"), []byte(body))
	}
	return repairLinkSymlink, l.symlink(bashy, filepath.Join(dir, name))
}

// repairBinDir is the private bin dir the runner prepares for each suite:
// the Bash# wrapper units (go/cc/c++) PLUS bashy's own POSIX/coreutils
// utilities as links to the pinned bashy (S549.R4b rework) — on a host with
// nothing but bashy, every POSIX name the suite PATH needs is there.
type repairBinDir struct {
	path      string
	posix     int    // POSIX/coreutils utility links created
	posixKind string // evidence label: symlink · hard link · hard link + cmd shim
}

// materializeRepairShims writes the suite's private bin dir into a fresh
// temp dir. Each wrapper alias becomes a Bash# unit whose shebang runs it
// through the pinned bashy (`#!<bashy> --bashsharp` — the shell's own
// shebang probe executes it on every OS, Windows included; on Windows each
// also gets a .cmd twin so direct CreateProcess consumers (cgo's CC/CXX) can
// run it). Then every POSIX/coreutils name bashy dispatches becomes a link
// to the pinned bashy — the wrappers' names are never shadowed. The dir is
// returned for the caller to clean up.
func materializeRepairShims(units repairUnits, bashy string) (repairBinDir, error) {
	if strings.ContainsAny(bashy, " \t") {
		return repairBinDir{}, fmt.Errorf("pinned bashy path %q contains whitespace — the wrapper shebang cannot carry it", bashy)
	}
	dir, err := os.MkdirTemp("", "bashy-repair-bin-*")
	if err != nil {
		return repairBinDir{}, err
	}
	bin := repairBinDir{path: dir}
	bodyFor := func(row repairToolRow) (string, bool) {
		body, ok := units.bodies[row.unit]
		if !ok {
			return "", false
		}
		// The unit templates already quote the pin ("{{bashy}}"); a path with
		// whitespace was refused above, so the raw slash-spelled path is safe.
		return strings.ReplaceAll(body, "{{bashy}}", filepath.ToSlash(bashy)), true
	}
	writeUnit := func(name, body string) error {
		unit := "#!" + bashy + " --bashsharp\n" + body + "\n"
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(unit), 0o755); err != nil {
			return err
		}
		return os.Chmod(p, 0o755)
	}
	for _, row := range repairBashyToolRows {
		if row.unit == "" {
			continue
		}
		body, ok := bodyFor(row)
		if !ok {
			continue
		}
		for _, alias := range row.aliases {
			if err := writeUnit(alias, body); err != nil {
				return bin, err
			}
			if runtime.GOOS == "windows" {
				run := units.runs[row.unit]
				if run == "" {
					continue
				}
				cmdBody := "@\"" + bashy + "\" " + run + " %*\r\n"
				if err := os.WriteFile(filepath.Join(dir, alias+".cmd"), []byte(cmdBody), 0o644); err != nil {
					return bin, err
				}
			}
		}
	}
	// S549.R4b rework: bashy's POSIX/coreutils utilities, linked to the
	// pinned bashy. The name list never carries a wrapper-owned name, and an
	// existing file in the dir is never overwritten (belt and braces: the
	// wrappers stay authoritative).
	var kinds []string
	for _, name := range repairPosixUtilNames() {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			continue
		}
		kind, err := repairWriteUtilLink(repairUtilLinks, dir, name, bashy)
		if err != nil {
			return bin, fmt.Errorf("posix utility %s: %w", name, err)
		}
		bin.posix++
		if !slices.Contains(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}
	sort.Strings(kinds)
	bin.posixKind = strings.Join(kinds, " + ")
	return bin, nil
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
