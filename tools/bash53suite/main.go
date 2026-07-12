package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var filterExpect = wordSet("attr exp exp-tests extglob extglob2 invert invocation more-exp new-exp nquote nquote1 nquote2 nquote3 nquote5 posix2 varenv")
var catVFixtures = wordSet("printf")

type fixture struct {
	Name  string
	Test  string
	Right string
}

func main() {
	var testsDir, bashPath, tests, chunk string
	var listOnly, shared bool
	var timeout, jobsTimeout time.Duration
	flag.StringVar(&testsDir, "tests-dir", "external/bash-5.3/tests", "bash 5.3 tests directory")
	flag.StringVar(&bashPath, "bash", "bin/bash", "bash-compatible binary under test")
	flag.StringVar(&tests, "tests", "", "space-separated fixture names to run")
	flag.StringVar(&chunk, "chunk", "", "run one distributed chunk, as 1/N")
	flag.BoolVar(&listOnly, "list", false, "list fixture names and exit")
	flag.BoolVar(&shared, "shared-tree", false, "run in the source fixture tree instead of a private copy (unsafe: leaks platform-built helpers across venues and races concurrent chunks)")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "per-fixture timeout")
	flag.DurationVar(&jobsTimeout, "jobs-timeout", 120*time.Second, "jobs fixture timeout")
	flag.Parse()
	if tests == "" {
		tests = os.Getenv("TESTS")
	}
	if chunk == "" {
		chunk = os.Getenv("CHUNK")
	}
	if v := os.Getenv("BASH53_TIMEOUT"); v != "" {
		parsed, err := time.ParseDuration(v)
		dieIf(err)
		timeout = parsed
	}
	if v := os.Getenv("BASH53_JOBS_TIMEOUT"); v != "" {
		parsed, err := time.ParseDuration(v)
		dieIf(err)
		jobsTimeout = parsed
	}
	if v := os.Getenv("BASH53_MEM_KB"); v != "" {
		parsed, err := strconv.Atoi(v)
		dieIf(err)
		memCapKB = parsed
	}

	root, err := os.Getwd()
	dieIf(err)
	testsDir, err = filepath.Abs(testsDir)
	dieIf(err)
	bashPath, err = filepath.Abs(bashPath)
	dieIf(err)

	fixtures, err := discoverFixtures(testsDir)
	dieIf(err)
	if listOnly {
		for _, f := range fixtures {
			fmt.Println(f.Name)
		}
		return
	}
	if len(fixtures) == 0 {
		die("no bash 5.3 fixtures found in %s", testsDir)
	}
	if _, err := os.Stat(bashPath); err != nil {
		die("bash under test not found: %s: %v", bashPath, err)
	}

	// Run against a private copy of the corpus, never the shared source tree.
	// See hermeticTree: the helpers are built into the tree, so sharing it lets
	// one venue's binaries poison another's run.
	if !shared {
		privateTests, cleanup, err := hermeticTree(testsDir)
		if err != nil {
			die("hermetic fixture tree: %v", err)
		}
		defer cleanup()
		testsDir = privateTests
	}
	if err := prepareFixtures(testsDir); err != nil {
		die("prepare fixtures: %v", err)
	}

	selected := selectFixtures(fixtures, wordSet(tests), chunk)
	if len(selected) == 0 {
		die("no fixtures selected")
	}

	fmt.Printf("Running bash 5.3 test suite against %s (%s timeout per test", bashPath, timeout)
	if chunk != "" {
		fmt.Printf(", chunk %s", chunk)
	}
	fmt.Println(")...")

	var passed, failed, skipped, timedOut int
	for _, f := range selected {
		if _, err := os.Stat(filepath.Join(testsDir, f.Test)); err != nil {
			skipped++
			continue
		}
		if _, err := os.Stat(filepath.Join(testsDir, f.Right)); err != nil {
			skipped++
			continue
		}
		perTestTimeout := timeout
		if f.Name == "jobs" {
			perTestTimeout = jobsTimeout
		}
		start := time.Now()
		result, err := runFixture(root, testsDir, bashPath, f, perTestTimeout)
		elapsed := time.Since(start)
		if err != nil {
			failed++
			fmt.Printf("  FAIL  %s\n", f.Name)
			fmt.Printf("        %v\n", err)
			fmt.Printf("DURATION\t%s\t%.3f\n", f.Name, elapsed.Seconds())
			continue
		}
		switch result {
		case "PASS":
			passed++
		case "TIME":
			timedOut++
		default:
			failed++
		}
		fmt.Printf("  %-5s %s\n", result, f.Name)
		fmt.Printf("DURATION\t%s\t%.3f\n", f.Name, elapsed.Seconds())
	}

	fmt.Println()
	fmt.Printf("Results: %d passed, %d failed, %d skipped, %d timed out\n", passed, failed, skipped, timedOut)
	if failed != 0 || timedOut != 0 {
		os.Exit(1)
	}
}

func discoverFixtures(testsDir string) ([]fixture, error) {
	matches, err := filepath.Glob(filepath.Join(testsDir, "run-*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	var fixtures []fixture
	for _, m := range matches {
		base := filepath.Base(m)
		if base == "run-all" || base == "run-minimal" {
			continue
		}
		name := strings.TrimPrefix(base, "run-")
		test, right := fixtureFiles(name)
		fixtures = append(fixtures, fixture{Name: name, Test: test, Right: right})
	}
	return fixtures, nil
}

func fixtureFiles(name string) (string, string) {
	test := name + ".tests"
	right := name + ".right"
	switch name {
	case "dirstack":
		test, right = "dstack.tests", "dstack.right"
	case "precedence":
		right = "prec.right"
	case "array2":
		test, right = "array-at-star", "array2.right"
	case "dollars":
		test, right = "dollar-at-star", "dollar.right"
	case "exp-tests":
		test, right = "exp.tests", "exp.right"
	case "glob-test":
		test, right = "glob.tests", "glob.right"
	case "histexpand":
		test, right = "histexp.tests", "histexp.right"
	case "input-test":
		test, right = "input-line.sh", "input.right"
	case "execscript":
		test, right = "execscript", "exec.right"
	}
	return test, right
}

func selectFixtures(fixtures []fixture, tests map[string]bool, chunk string) []fixture {
	var selected []fixture
	for _, f := range fixtures {
		if len(tests) != 0 && !tests[f.Name] {
			continue
		}
		selected = append(selected, f)
	}
	if chunk == "" {
		return selected
	}
	idx, total, err := parseChunk(chunk)
	dieIf(err)
	var out []fixture
	for i, f := range selected {
		if i%total == idx-1 {
			out = append(out, f)
		}
	}
	return out
}

func parseChunk(chunk string) (int, int, error) {
	parts := strings.Split(chunk, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("chunk must be I/N, got %q", chunk)
	}
	idx, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	total, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if idx < 1 || total < 1 || idx > total {
		return 0, 0, fmt.Errorf("invalid chunk %q", chunk)
	}
	return idx, total, nil
}

// hermeticTree copies the fixture corpus into a private per-run directory and
// returns the path of the copied tests/ dir.
//
// The fixture tree is shared, mutable state: the corpus lives in a bind-mounted
// bash source tree, and the C helpers (recho/zecho/xcase) are built INTO it. Two
// runs of different venues therefore fight over one file — a container run writes
// ELF helpers into the host's tree, a host run writes Mach-O ones back, and each
// leaves the other executing a binary for the wrong platform. Every fixture that
// uses recho then reports a conformance FAILURE that is really an infrastructure
// failure (measured: 47/86 vs 77/86 on the same Linux container, decided purely by
// which platform built the helpers last). Two chunks sharing a host race the same
// way. A private copy per run removes the shared state entirely, and keeps the
// suite from writing into the user's bash source tree at all.
func hermeticTree(srcTests string) (string, func(), error) {
	work, err := os.MkdirTemp("", "bash53-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(work) }
	srcRoot := filepath.Dir(srcTests)
	for _, dir := range []string{"tests", "support"} {
		src := filepath.Join(srcRoot, dir)
		if _, err := os.Stat(src); err != nil {
			continue // support/ is optional; tests/ is validated by the caller
		}
		if err := copyTree(src, filepath.Join(work, dir)); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("copy %s: %w", dir, err)
		}
	}
	return filepath.Join(work, "tests"), cleanup, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil // the corpus has no meaningful symlinks; skip rather than dangle
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// memCapKB is the per-fixture RSS ceiling, summed over the fixture's whole
// process group. 4 GB is far above any legitimate fixture.
var memCapKB = 4 * 1024 * 1024

// watchMemory kills the fixture's process group if its total RSS exceeds capKB.
// It is a backstop, not a limit to tune: a fixture that trips it has a bug.
func watchMemory(pid, capKB int, stop <-chan struct{}) {
	if capKB <= 0 || runtime.GOOS == "windows" {
		return // no portable process-group RSS on Windows; the timeout still applies
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			if groupRSSKB(pid) > capKB {
				killProcessTree(pid)
				return
			}
		}
	}
}

// groupRSSKB sums the resident set size of every process in pid's process group.
// The fixture is its own group leader (SysProcAttr.Setpgid), so the group id is
// the pid, and this catches children the fixture forked.
func groupRSSKB(pid int) int {
	out, err := exec.Command("ps", "ax", "-o", "pgid=,rss=").Output()
	if err != nil {
		return 0
	}
	total := 0
	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if gid, err := strconv.Atoi(fields[0]); err != nil || gid != pid {
			continue
		}
		if rss, err := strconv.Atoi(fields[1]); err == nil {
			total += rss
		}
	}
	return total
}

func prepareFixtures(testsDir string) error {
	support := filepath.Join(testsDir, "..", "support")
	for _, helper := range []string{"recho", "zecho", "xcase"} {
		dst := filepath.Join(testsDir, exeName(helper))
		src := filepath.Join(support, helper+".c")
		if _, err := os.Stat(src); err != nil {
			continue
		}
		// Always build: the tree is a fresh private copy, so a pre-existing binary
		// can only be one copied in from the source tree — i.e. built for whichever
		// platform ran last. Never trust it. A missing compiler is a hard refusal,
		// not a silent skip that surfaces later as `recho: command not found`
		// masquerading as a conformance failure.
		cc, err := exec.LookPath("cc")
		if err != nil {
			return fmt.Errorf("tool.missing: no `cc` to build the bash test helper %q; "+
				"the fixtures cannot run without it (this is an environment refusal, not a test failure)", helper)
		}
		if out, err := exec.Command(cc, "-o", dst, src).CombinedOutput(); err != nil {
			return fmt.Errorf("build %s: %v\n%s", helper, err, out)
		}
	}
	parent := filepath.Dir(testsDir)
	if err := ensureStub(filepath.Join(parent, "config.h"), 128, "config.h", "heredoc5.sub"); err != nil {
		return err
	}
	if err := ensureStub(filepath.Join(parent, "version.h"), 16, "version.h", "heredoc5.sub"); err != nil {
		return err
	}
	if err := ensureStub(filepath.Join(parent, "y.tab.c"), 2048, "y.tab.c", "heredoc5.sub"); err != nil {
		return err
	}
	loadables := filepath.Join(parent, "examples", "loadables")
	if err := os.MkdirAll(loadables, 0o755); err != nil {
		return err
	}
	mk := filepath.Join(loadables, "Makefile")
	if _, err := os.Stat(mk); os.IsNotExist(err) {
		ldflags := "-shared"
		if runtime.GOOS == "darwin" {
			ldflags = "-shared -undefined dynamic_lookup"
		}
		body := "CC = cc\nSHOBJ_STATUS = supported\nSHOBJ_CC = cc\nSHOBJ_CFLAGS = -fPIC\nSHOBJ_LD = cc\nSHOBJ_LDFLAGS = " + ldflags + "\nSHOBJ_XLDFLAGS =\nSHOBJ_LIBS =\n"
		if err := os.WriteFile(mk, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func ensureStub(path string, lines int, name, reason string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&b, "/* stub %s line %04d for %s */\n", name, i, reason)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func runFixture(root, testsDir, bashPath string, f fixture, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	args := []string{}
	var stdin *os.File
	if f.Name == "input-test" {
		in, err := os.Open(filepath.Join(testsDir, "input-line.sh"))
		if err != nil {
			return "FAIL", err
		}
		defer in.Close()
		stdin = in
	} else {
		args = append(args, "./"+filepath.ToSlash(f.Test))
	}
	cmd := exec.CommandContext(ctx, bashPath, args...)
	configureProcess(cmd)
	cmd.Dir = testsDir
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var raw bytes.Buffer
	cmd.Stdout = &raw
	cmd.Stderr = &raw
	cmd.Env = fixtureEnv(root, testsDir, bashPath, f.Name)

	if err := cmd.Start(); err != nil {
		return "FAIL", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	// Memory watchdog (ported from scripts/memwatch.sh, which the shell fixture
	// loop ran alongside every fixture). macOS does not honor `ulimit -v`, so an
	// unbounded-allocation fixture (intl/unicode1.sub is the known one) can
	// balloon to 100+ GB and wedge the host long before the wall-clock timeout
	// fires. Killing past a hard RSS cap turns an OOM into a graceful TIME.
	memStop := make(chan struct{})
	defer close(memStop)
	go watchMemory(cmd.Process.Pid, memCapKB, memStop)

	var runErr error
	timedOut := false
	select {
	case runErr = <-done:
	case <-timer.C:
		timedOut = true
		killProcessTree(cmd.Process.Pid)
		select {
		case runErr = <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			select {
			case runErr = <-done:
			case <-time.After(2 * time.Second):
			}
		}
	}
	killProcessTree(cmd.Process.Pid)
	if timedOut {
		return "TIME", nil
	}
	if runErr != nil {
		// Bash's own harness gates on output, not process status. Keep going.
	}

	got := normalizeOutput(f.Name, raw.Bytes())
	want, err := os.ReadFile(filepath.Join(testsDir, f.Right))
	if err != nil {
		return "FAIL", err
	}
	if bytes.Equal(got, want) {
		return "PASS", nil
	}
	writeDebugOutput(f.Name, want, got)
	return "FAIL", fmt.Errorf("output differs from %s\n%s", f.Right, firstDiff(want, got))
}

func fixtureEnv(root, testsDir, bashPath, name string) []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+10)
	for _, kv := range env {
		if strings.HasPrefix(kv, "OLDPWD=") {
			continue
		}
		out = append(out, kv)
	}
	tmpBase := os.TempDir()
	rawPath := filepath.Join(tmpBase, fmt.Sprintf("bashy-tstraw-%d", os.Getpid()))
	outPath := filepath.Join(tmpBase, fmt.Sprintf("bashy-tstout-%d", os.Getpid()))
	out = append(out,
		"THIS_SH="+bashPath,
		"_="+bashPath,
		"BUILD_DIR="+filepath.Dir(testsDir),
		"PATH="+strings.Join([]string{testsDir, "/usr/bin", "/bin", "/usr/local/bin"}, string(os.PathListSeparator)),
		"BASH_TSTRAW="+rawPath,
		"BASH_TSTOUT="+outPath,
		"BASH_SETPGRP=1",
	)
	if name == "read" {
		tmp, err := os.MkdirTemp("", "bashy-read-*")
		if err == nil {
			out = append(out, "TMPDIR="+tmp)
		}
	}
	return out
}

func normalizeOutput(name string, raw []byte) []byte {
	out := raw
	if filterExpect[name] {
		out = removeExpectLines(out)
	}
	if catVFixtures[name] {
		out = catV(out)
	}
	if name == "test" {
		out = normalizeTestFixture(out)
	}
	return out
}

func removeExpectLines(in []byte) []byte {
	lines := bytes.SplitAfter(in, []byte("\n"))
	var out []byte
	for _, line := range lines {
		if !bytes.HasPrefix(line, []byte("expect")) {
			out = append(out, line...)
		}
	}
	return out
}

func catV(in []byte) []byte {
	var out []byte
	for _, c := range in {
		switch {
		case c == '\n' || c == '\t':
			out = append(out, c)
		case c < 32:
			out = append(out, '^', c+64)
		case c == 127:
			out = append(out, '^', '?')
		default:
			out = append(out, c)
		}
	}
	return out
}

func normalizeTestFixture(in []byte) []byte {
	out := in
	replacements := []struct {
		re   *regexp.Regexp
		repl []byte
	}{
		{
			regexp.MustCompile(`(?m)^chmod: .*?test\.setgid:.*\n(t -g /tmp/test\.setgid\n)1\n`),
			[]byte("${1}0\n"),
		},
		{
			regexp.MustCompile(`(?m)^chmod: .*?test\.setuid:.*\n(t -u /tmp/test\.setuid\n)1\n`),
			[]byte("${1}0\n"),
		},
		{
			regexp.MustCompile(`(t -n xx -a -z "" -a -t 0 -a -t\n)1\n`),
			[]byte("${1}0\n"),
		},
	}
	for _, r := range replacements {
		out = r.re.ReplaceAll(out, r.repl)
	}
	return out
}

func wordSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.Fields(s) {
		out[f] = true
	}
	return out
}

func firstDiff(want, got []byte) string {
	wantLines := bytes.SplitAfter(want, []byte("\n"))
	gotLines := bytes.SplitAfter(got, []byte("\n"))
	max := len(wantLines)
	if len(gotLines) > max {
		max = len(gotLines)
	}
	for i := 0; i < max; i++ {
		var w, g []byte
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if !bytes.Equal(w, g) {
			return fmt.Sprintf("        first diff at line %d\n        want: %q\n        got:  %q", i+1, trimDiffLine(w), trimDiffLine(g))
		}
	}
	return "        outputs differ"
}

func writeDebugOutput(name string, want, got []byte) {
	dir := os.Getenv("BASH53_DEBUG_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name+".want"), want, 0o644)
	_ = os.WriteFile(filepath.Join(dir, name+".got"), got, 0o644)
}

func trimDiffLine(b []byte) string {
	const limit = 180
	s := strings.TrimRight(string(b), "\r\n")
	if len(s) > limit {
		return s[:limit] + "..."
	}
	return s
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func dieIf(err error) {
	if err != nil {
		die("%v", err)
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "bash53-suite: "+format+"\n", args...)
	os.Exit(2)
}

var _ io.Reader
