// ztst is a shell-agnostic runner for zsh's .ztst test-file format
// (Test/*.ztst in the zsh source tree), used by scripts/zsh-scoreboard.sh
// to score bashy — and any reference shell — on zsh's own regression suite.
//
// zsh's native driver (Test/ztst.zsh) is itself a zsh script that evals each
// test chunk in the driver's own process, so it can only ever test the shell
// it runs under. This runner reimplements the file format: it parses the
// fixture, generates a portable driver script that evals each chunk in ONE
// persistent shell process (state carries across tests, like ztst.zsh),
// executes it under the shell given by -shell, and does the verdict
// comparison host-side.
//
// Deliberate Tier-0 approximations vs ztst.zsh (see docs/zsh-scoreboard.md):
//   - option save/restore between chunks is approximated by `set +e +u +f`
//     rather than the zsh/parameter $options round-trip;
//   - the `q` flag's ${(e)...} expansion of expected output is approximated
//     with an unquoted heredoc evaluated in the driver;
//   - `*>` pattern lines are matched by delegating to a real zsh binary
//     (-zsh), so both arms are judged by the same pattern engine.
//
// The same approximations apply to every arm, so the reference-shell arm
// defines which cases are valid: cases real zsh fails under this runner are
// runner/environment noise, excluded from the scoreboard denominator.
//
// Dev tool: not part of `make build`; run via `go run ./tools/ztst`.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type test struct {
	code    string
	xstatus string // numeric string, or "-" for any
	flags   string // f, q, d, D
	message string
	stdin   []string
	out     []string
	errOut  []string
	patOut  bool // any *> line → whole stdout compared as patterns
	patErr  bool // any *? line → whole stderr compared as patterns
	line    int  // status line number in the fixture, for reporting
}

type fixture struct {
	prep  []string // code chunks
	tests []test
	clean []string
}

var statusRe = regexp.MustCompile(`^([0-9-]+)([a-zA-Z]*)(:(.*))?$`)

// parse implements the read layer of ztst.zsh: column-0 '#' lines are
// dropped everywhere, '%name' lines at column 0 switch sections, indented
// runs form code chunks, and in %test each chunk is followed by a status
// line plus optional  < > ? *> *? F:  blocks, terminated by a blank line.
func parse(path string) (*fixture, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	type ln struct {
		text string
		num  int
	}
	var lines []ln
	for i, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(l, "#") {
			continue
		}
		lines = append(lines, ln{l, i + 1})
	}

	fx := &fixture{}
	sect := ""
	i := 0
	isBlank := func(s string) bool { return strings.TrimSpace(s) == "" }
	isIndented := func(s string) bool {
		return len(s) > 0 && (s[0] == ' ' || s[0] == '\t') && !isBlank(s)
	}
	readChunk := func() string {
		var code []string
		for i < len(lines) && isIndented(lines[i].text) {
			code = append(code, lines[i].text)
			i++
		}
		return strings.Join(code, "\n")
	}
	readRedir := func(ch byte) []string {
		var out []string
		for i < len(lines) && len(lines[i].text) > 0 && lines[i].text[0] == ch {
			out = append(out, lines[i].text[1:])
			i++
		}
		return out
	}

	for i < len(lines) {
		l := lines[i].text
		switch {
		case strings.HasPrefix(l, "%"):
			sect = strings.TrimRight(l[1:], " \t")
			i++
		case isBlank(l):
			i++
		case sect == "prep" || sect == "clean":
			chunk := readChunk()
			if chunk == "" {
				return nil, fmt.Errorf("%s:%d: bad line in %%%s section: %q", path, lines[i].num, sect, l)
			}
			if sect == "prep" {
				fx.prep = append(fx.prep, chunk)
			} else {
				fx.clean = append(fx.clean, chunk)
			}
		case sect == "test":
			t := test{xstatus: "-"}
			found := false
			hasCode := false
			for i < len(lines) {
				l = lines[i].text
				if strings.HasPrefix(l, "%") || isBlank(l) {
					// blank before any content: outer loop skips it and
					// re-enters; blank/section after content: test ends
					break
				}
				switch {
				case isIndented(l):
					if hasCode {
						return nil, fmt.Errorf("%s:%d: second code chunk in one test", path, lines[i].num)
					}
					t.code = readChunk()
					hasCode = true
					// the chunk must be followed directly by the status line
					if i >= len(lines) || !statusRe.MatchString(lines[i].text) {
						at := "EOF"
						if i < len(lines) {
							at = lines[i].text
						}
						return nil, fmt.Errorf("%s:%d: expecting test status at: %q", path, lines[min(i, len(lines)-1)].num, at)
					}
					m := statusRe.FindStringSubmatch(lines[i].text)
					t.xstatus, t.flags, t.line = m[1], m[2], lines[i].num
					if m[3] != "" {
						t.message = m[4]
					}
					i++
					found = true
				case strings.HasPrefix(l, "<"):
					t.stdin = append(t.stdin, readRedir('<')...)
					found = true
				case strings.HasPrefix(l, "*>"):
					t.patOut = true
					lines[i].text = l[1:]
					t.out = append(t.out, readRedir('>')...)
					found = true
				case strings.HasPrefix(l, ">"):
					t.out = append(t.out, readRedir('>')...)
					found = true
				case strings.HasPrefix(l, "*?"):
					t.patErr = true
					lines[i].text = l[1:]
					t.errOut = append(t.errOut, readRedir('?')...)
					found = true
				case strings.HasPrefix(l, "?"):
					t.errOut = append(t.errOut, readRedir('?')...)
					found = true
				case strings.HasPrefix(l, "F:"):
					i++ // informational only
					found = true
				default:
					return nil, fmt.Errorf("%s:%d: bad line in test block: %q", path, lines[i].num, l)
				}
			}
			if found {
				fx.tests = append(fx.tests, t)
			}
		default:
			return nil, fmt.Errorf("%s:%d: bad line before section: %q", path, lines[i].num, l)
		}
	}
	return fx, nil
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// heredocDelim returns a delimiter not present in the content.
func heredocDelim(content string) string {
	d := "ZTST_Q_EOF"
	for strings.Contains(content, d) {
		d += "_X"
	}
	return d
}

func joinBlock(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// driver generates the per-fixture driver script. All bookkeeping paths are
// absolute (test chunks cd freely, like B01cd), and every harness statement
// is an `if`/simple-command that exits 0 so a leaked `set -e` can't kill the
// run between chunks.
func driver(fx *fixture, work, testDir string, fpath []string) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	st := filepath.Join(work, "st.log")
	w("ZTST_testdir=%s", shq(testDir))
	if len(fpath) > 0 {
		// zsh-meaningful, bash-parseable array assignment (ztst.zsh sets
		// fpath to the source tree's Functions/Completion dirs)
		q := make([]string, len(fpath))
		for i, d := range fpath {
			q[i] = shq(d)
		}
		w("fpath=( %s )", strings.Join(q, " "))
	}
	w("ZTST_unimplemented=")
	w("ZTST_skip=")
	w("ZTST_prepfail=")
	// like ztst.zsh's ZTST_execchunk: chunks eval inside a function, so
	// typeset/declare create locals while plain assignments stay global
	// and persist across chunks
	w("ZTST_execchunk() { eval \"$ZTST_code\"; }")
	for pi, chunk := range fx.prep {
		w("ZTST_code=%s", shq(chunk))
		w("ZTST_execchunk >/dev/null")
		w("if [ $? -ne 0 ]; then ZTST_prepfail=1; fi")
		w("set +e +u +f")
		w(`if [ -n "$ZTST_unimplemented" ]; then echo "U %d" >> %s; exit 91; fi`, pi, st)
		w(`if [ -n "$ZTST_prepfail" ]; then echo "P %d" >> %s; exit 90; fi`, pi, st)
	}
	for n, t := range fx.tests {
		in := filepath.Join(work, fmt.Sprintf("in.%d", n))
		out := filepath.Join(work, fmt.Sprintf("out.%d", n))
		errf := filepath.Join(work, fmt.Sprintf("err.%d", n))
		w("ZTST_skip=")
		w("ZTST_code=%s", shq(t.code))
		w("ZTST_execchunk < %s > %s 2> %s", in, out, errf)
		w(`echo "T %d $?" >> %s`, n, st)
		w("set +e +u +f")
		w(`if [ -n "$ZTST_skip" ]; then echo "S %d" >> %s; fi`, n, st)
		if strings.Contains(t.flags, "q") {
			// approximate ${(e)expected}: expand via unquoted heredoc,
			// in-driver, after the chunk has set its variables
			for _, s := range []struct {
				lines []string
				ext   string
			}{{t.out, "out"}, {t.errOut, "err"}} {
				if len(s.lines) == 0 {
					continue
				}
				content := joinBlock(s.lines)
				d := heredocDelim(content)
				qf := filepath.Join(work, fmt.Sprintf("qexp.%d.%s", n, s.ext))
				w("cat > %s <<%s", qf, d)
				b.WriteString(content)
				w("%s", d)
				w("set +e +u +f")
			}
		}
	}
	for _, chunk := range fx.clean {
		w("ZTST_code=%s", shq(chunk))
		w("ZTST_execchunk >/dev/null")
		w("set +e +u +f")
	}
	w("exit 0")
	return b.String()
}

type verdict struct {
	file string
	n    int
	line int
	code string // OK FAIL SKIP PREPFAIL UNIMPL TIMEOUT NOSTATUS PARSEFAIL
	msg  string
}

type runner struct {
	shell   []string
	zsh     string
	srcdir  string
	timeout time.Duration
}

func (r *runner) patMatch(pat, line string) bool {
	c := exec.Command(r.zsh, "-fc", `setopt extendedglob; [[ $2 == ${~1} ]]`, "ztst-pat", pat, line)
	return c.Run() == nil
}

func (r *runner) patDiff(expected []string, actual string) bool {
	al := strings.Split(strings.TrimSuffix(actual, "\n"), "\n")
	if actual == "" {
		al = nil
	}
	if len(al) != len(expected) {
		return false
	}
	for i := range expected {
		if !r.patMatch(expected[i], al[i]) {
			return false
		}
	}
	return true
}

func (r *runner) runFile(path string) []verdict {
	name := filepath.Base(path)
	fx, err := parse(path)
	if err != nil {
		return []verdict{{file: name, n: -1, code: "PARSEFAIL", msg: err.Error()}}
	}
	work, err := os.MkdirTemp("", "ztst-*")
	if err != nil {
		return []verdict{{file: name, n: -1, code: "PARSEFAIL", msg: err.Error()}}
	}
	defer os.RemoveAll(work)

	// Mirror the zsh build-tree layout tests assume: cwd = <work>/Test with
	// ../Src/zsh being the shell under test (many fixtures spawn fresh
	// instances via $ZTST_testdir/../Src/zsh or $ZTST_exe). Bookkeeping
	// files stay in the work root, out of reach of tests' `rm -rf *.tmp`.
	testDir := filepath.Join(work, "Test")
	os.MkdirAll(testDir, 0o755)
	os.MkdirAll(filepath.Join(work, "Src"), 0o755)
	// build-tree shim: D03procsubst gates on these config.h defines
	os.WriteFile(filepath.Join(work, "config.h"),
		[]byte("#define PATH_DEV_FD \"/dev/fd\"\n#define HAVE_FIFOS 1\n"), 0o644)
	exe, err := filepath.Abs(r.shell[0])
	if err == nil {
		if p, err2 := exec.LookPath(r.shell[0]); err2 == nil {
			exe, _ = filepath.Abs(p)
		}
		os.Symlink(exe, filepath.Join(work, "Src", "zsh"))
	}

	for n, t := range fx.tests {
		os.WriteFile(filepath.Join(work, fmt.Sprintf("in.%d", n)), []byte(joinBlock(t.stdin)), 0o644)
	}
	// fpath per ztst.zsh: Functions subdirs + Completion trees of the
	// source checkout (empty when running against a bare Test/ dir)
	var fpath []string
	for _, pat := range []string{"../Functions", "../Functions/*", "../Completion", "../Completion/*/*"} {
		m, _ := filepath.Glob(filepath.Join(r.srcdir, pat))
		for _, d := range m {
			if fi, err := os.Stat(d); err == nil && fi.IsDir() {
				fpath = append(fpath, d)
			}
		}
	}

	dpath := filepath.Join(work, "driver.sh")
	os.WriteFile(dpath, []byte(driver(fx, work, testDir, fpath)), 0o644)

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	args := append(r.shell[1:], dpath)
	cmd := exec.CommandContext(ctx, r.shell[0], args...)
	cmd.Dir = testDir
	cmd.Env = []string{
		"PATH=/bin:/usr/bin:/usr/sbin:/sbin",
		"HOME=" + work,
		"LANG=C", "LC_ALL=C",
		"TMPPREFIX=" + filepath.Join(work, "ztmp"),
		"ZTST_srcdir=" + r.srcdir,
		"ZTST_exe=" + exe,
		"TERM=dumb",
	}
	cmd.Stdout = nil
	derr, _ := os.Create(filepath.Join(work, "driver.err"))
	cmd.Stderr = derr
	runErr := cmd.Run()
	derr.Close()
	timedOut := ctx.Err() == context.DeadlineExceeded

	// harvest st.log
	status := map[int]string{}
	skipped := map[int]bool{}
	prepFail, unimpl := false, false
	if raw, err := os.ReadFile(filepath.Join(work, "st.log")); err == nil {
		for l := range strings.SplitSeq(string(raw), "\n") {
			f := strings.Fields(l)
			if len(f) < 2 {
				continue
			}
			n, _ := strconv.Atoi(f[1])
			switch f[0] {
			case "T":
				if len(f) > 2 {
					status[n] = f[2]
				}
			case "S":
				skipped[n] = true
			case "P":
				prepFail = true
			case "U":
				unimpl = true
			}
		}
	}
	_ = runErr

	var vs []verdict
	for n, t := range fx.tests {
		v := verdict{file: name, n: n, line: t.line, msg: t.message}
		switch {
		case unimpl:
			v.code = "UNIMPL"
		case prepFail:
			v.code = "PREPFAIL"
		case skipped[n]:
			v.code = "SKIP"
		case status[n] == "":
			if timedOut {
				v.code = "TIMEOUT"
			} else {
				v.code = "NOSTATUS"
			}
		default:
			v.code = r.judge(&fx.tests[n], n, work, status[n])
		}
		vs = append(vs, v)
	}
	return vs
}

func (r *runner) judge(t *test, n int, work, got string) string {
	pass := true
	if t.xstatus != "-" && got != t.xstatus {
		pass = false
	}
	readF := func(f string) string {
		b, _ := os.ReadFile(filepath.Join(work, f))
		return string(b)
	}
	expOut, expErr := joinBlock(t.out), joinBlock(t.errOut)
	if strings.Contains(t.flags, "q") {
		if len(t.out) > 0 {
			expOut = readF(fmt.Sprintf("qexp.%d.out", n))
		}
		if len(t.errOut) > 0 {
			expErr = readF(fmt.Sprintf("qexp.%d.err", n))
		}
	}
	if pass && !strings.Contains(t.flags, "d") {
		actual := readF(fmt.Sprintf("out.%d", n))
		if t.patOut {
			pass = r.patDiff(t.out, actual)
		} else {
			pass = actual == expOut
		}
	}
	if pass && !strings.Contains(t.flags, "D") {
		actual := readF(fmt.Sprintf("err.%d", n))
		if t.patErr {
			pass = r.patDiff(t.errOut, actual)
		} else {
			pass = actual == expErr
		}
	}
	if strings.Contains(t.flags, "f") { // expected to fail: XFAIL passes, XPASS fails
		pass = !pass
	}
	if pass {
		return "OK"
	}
	return "FAIL"
}

func main() {
	shell := flag.String("shell", "", "shell under test (command line, split on spaces)")
	zsh := flag.String("zsh", "zsh", "real zsh binary for *> pattern matching")
	testdir := flag.String("testdir", "", "zsh Test/ directory holding *.ztst")
	timeout := flag.Duration("timeout", 60*time.Second, "per-fixture timeout")
	flag.Parse()
	if *shell == "" || *testdir == "" {
		fmt.Fprintln(os.Stderr, "usage: ztst -shell <cmd> -testdir <zsh>/Test [-zsh zsh] [classes-or-files...]")
		os.Exit(2)
	}

	var files []string
	for _, a := range flag.Args() {
		if strings.HasSuffix(a, ".ztst") {
			files = append(files, filepath.Join(*testdir, filepath.Base(a)))
			continue
		}
		m, _ := filepath.Glob(filepath.Join(*testdir, a+"*.ztst"))
		files = append(files, m...)
	}
	if flag.NArg() == 0 {
		files, _ = filepath.Glob(filepath.Join(*testdir, "*.ztst"))
	}
	sort.Strings(files)
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "ztst: no fixtures matched")
		os.Exit(2)
	}

	src, _ := filepath.Abs(*testdir)
	r := &runner{shell: strings.Fields(*shell), zsh: *zsh, srcdir: src, timeout: *timeout}
	for _, f := range files {
		fmt.Fprintf(os.Stderr, "ztst: %s\n", filepath.Base(f))
		for _, v := range r.runFile(f) {
			fmt.Printf("%s\t%d\t%d\t%s\t%s\n", v.file, v.n, v.line, v.code, v.msg)
		}
	}
}
