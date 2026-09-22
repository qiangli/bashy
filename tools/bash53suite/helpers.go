package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The GNU bash test corpus ships three tiny C helpers under support/ —
// recho (echo argv bracketed, control bytes made visible), zecho (bare echo)
// and xcase (case-fold stdin) — which the fixtures call as external commands.
// On Unix the harness builds them with the host C compiler, exactly as
// bash's own `make tests` does. On Windows the mingw toolchain on a hosted
// runner opens stdout in text mode and writes CRLF for every "\n"; the
// `_CRT_fmode` unit that fixes msvcrt is ignored by UCRT, so 30 fixtures
// diverged by a byte the C runtime added, not the shell (Sprint 245).
//
// So on Windows the helpers are these Go implementations of the same
// contracts, dispatched by argv[0]: prepareFixtures hard-links the harness
// binary into the private tests tree as recho.exe / zecho.exe / xcase.exe,
// and main() routes there before the harness flag set is even parsed. Go
// writes os.Stdout in binary mode, so the measured bytes are the helper's
// contract and nothing else. The Unix legs are untouched: their helpers are
// still the corpus's C sources built with cc, so the canonical 86/86 keeps
// measuring the same binaries it always has.

// helperNames are the corpus helpers that have a Go implementation here.
var helperNames = []string{"recho", "zecho", "xcase"}

// runAsHelper runs the helper named by argv0 when the harness binary was
// invoked under one of the helper names (a hard link or a copy), and reports
// whether it did. It never triggers on the harness's own name.
func runAsHelper(argv0 string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, bool) {
	base := strings.TrimSuffix(filepath.Base(argv0), ".exe")
	if runtime.GOOS == "windows" {
		// argv[0] keeps whatever case the caller used (rEcHo.exe still runs).
		base = strings.ToLower(base)
	}
	switch base {
	case "recho":
		return helperRecho(args, stdout), true
	case "zecho":
		return helperZecho(args, stdout), true
	case "xcase":
		return helperXcase(args, stdin, stdout, stderr), true
	}
	return 0, false
}

// helperRecho prints each argument as `argv[N] = <...>` with bytes below
// 0x20 shown as ^X (X = byte+64) and DEL as ^?, one line per argument.
func helperRecho(args []string, stdout io.Writer) int {
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	for i, arg := range args {
		fmt.Fprintf(w, "argv[%d] = <", i+1)
		for j := 0; j < len(arg); j++ {
			b := arg[j]
			switch {
			case b < ' ':
				w.WriteByte('^')
				w.WriteByte(b + 64)
			case b == 127:
				w.WriteString("^?")
			default:
				w.WriteByte(b)
			}
		}
		w.WriteString(">\n")
	}
	return 0
}

// helperZecho prints its arguments separated by single spaces, then a
// newline — no option parsing, no escape processing.
func helperZecho(args []string, stdout io.Writer) int {
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	w.WriteString(strings.Join(args, " "))
	w.WriteByte('\n')
	return 0
}

// helperXcase copies stdin (or the one file operand; `-` means stdin) to
// stdout, upper-casing ASCII letters under -u and lower-casing under -l;
// -n asks for unbuffered output. Unknown options exit 2 with the corpus's
// usage line; an unopenable file exits 1.
func helperXcase(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	const (
		asIs = iota
		lower
		upper
	)
	op := asIs
	unbuffered := false
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		for _, c := range a[1:] {
			switch c {
			case 'n':
				unbuffered = true
			case 'u':
				op = upper
			case 'l':
				op = lower
			default:
				fmt.Fprintln(stderr, "casemod: usage: casemod [-lnu] [file]")
				return 2
			}
		}
	}
	in := stdin
	if i < len(args) && args[i] != "-" {
		f, err := os.Open(args[i])
		if err != nil {
			fmt.Fprintf(stderr, "casemod: %s: cannot open: %v\n", args[i], err)
			return 1
		}
		defer f.Close()
		in = f
	}
	var out io.Writer = stdout
	var flush func()
	if !unbuffered {
		bw := bufio.NewWriter(stdout)
		out, flush = bw, func() { bw.Flush() }
		defer flush()
	}
	buf := make([]byte, 32*1024)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			switch op {
			case upper:
				for k, b := range chunk {
					if 'a' <= b && b <= 'z' {
						chunk[k] = b - 'a' + 'A'
					}
				}
			case lower:
				for k, b := range chunk {
					if 'A' <= b && b <= 'Z' {
						chunk[k] = b - 'A' + 'a'
					}
				}
			}
			if _, werr := out.Write(chunk); werr != nil {
				return 1
			}
		}
		if err != nil {
			break
		}
	}
	return 0
}

// installGoHelpers places the harness binary into testsDir under each helper
// name so argv[0] dispatch serves the fixtures. A hard link is the normal
// case (same volume, no privilege needed on NTFS); a copy is the fallback
// when the private tree lives on another volume.
func installGoHelpers(testsDir string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate harness binary for the Go helpers: %v", err)
	}
	for _, helper := range helperNames {
		dst := filepath.Join(testsDir, exeName(helper))
		_ = os.Remove(dst)
		if err := os.Link(self, dst); err == nil {
			continue
		}
		if err := copyFile(self, dst, 0o755); err != nil {
			return fmt.Errorf("install Go helper %s: %v", helper, err)
		}
	}
	return nil
}

// copyFile copies src to dst with the given mode, replacing dst.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
