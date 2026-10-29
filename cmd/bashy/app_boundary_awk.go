//go:build !bashy_core && !bashy_cert_base

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	awkinterp "github.com/benhoyt/goawk/interp"
	"github.com/benhoyt/goawk/lexer"
	"github.com/benhoyt/goawk/parser"
	"github.com/qiangli/yoke/pkg/shape/boundary"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
)

// boundedAwk serves `awk` under an app boundary. awk is in-process in bashy
// (coreutils' GoAWK applet), but yoke's guard classes it as a spawner — its
// system() and pipes run sh — and refuses it as an undeclared external
// program; the applet's own getline/print redirections and file operands
// also open files directly, out of the guard's reach. So under the boundary
// bashy runs the same GoAWK engine here instead: every file open (operands,
// -f program files, getline <, print >/>>) is checked by the guard, and
// system()/pipes are disabled (GoAWK NoExec) — awk never starts a process.
// It sits outside the guard's exec middleware; an `awk` spelled with a path
// is still a program the guard decides.
func boundedAwk(g *boundary.Guard) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 || args[0] != "awk" {
				return next(ctx, args)
			}
			hc := interp.HandlerCtx(ctx)
			if code := runBoundedAwk(g, hc.Dir, hc.Env, hc.Stdin, hc.Stdout, hc.Stderr, args[1:]); code != 0 {
				return interp.ExitStatus(uint8(code))
			}
			return nil
		}
	}
}

var awkAssign = regexp.MustCompile(`^[_a-zA-Z][_a-zA-Z0-9]*=`)

// runBoundedAwk is POSIX awk's command line (-F fs, -v var=value, -f
// progfile, --) over GoAWK with the guard on every open. Locale is C.
func runBoundedAwk(g *boundary.Guard, dir string, env expand.Environ, stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	usage := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "awk: "+format+"\n", a...)
		fmt.Fprintln(stderr, "usage: awk [-F fs] [-v var=value] [-f progfile | 'program'] [file ...]")
		return 2
	}
	open := func(name string, flag int, perm os.FileMode) (*os.File, error) {
		op := boundary.OpRead
		if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_APPEND|os.O_TRUNC) != 0 {
			op = boundary.OpWrite
		}
		if err := g.Check(op, dir, name); err != nil {
			return nil, err
		}
		if !filepath.IsAbs(name) && !strings.HasPrefix(name, "/") && !strings.HasPrefix(name, `\`) {
			name = filepath.Join(dir, name)
		}
		return os.OpenFile(name, flag, perm)
	}

	var vars, progFiles []string
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			break
		}
		opt, val := a[:2], a[2:]
		if opt != "-F" && opt != "-v" && opt != "-f" {
			return usage("unknown option %s", a)
		}
		if val == "" {
			if i++; i >= len(args) {
				return usage("option %s requires an argument", opt)
			}
			val = args[i]
		}
		switch opt {
		case "-F":
			vars = append(vars, "FS", awkUnescape(val))
		case "-v":
			if !awkAssign.MatchString(val) {
				return usage("invalid -v assignment %q", val)
			}
			name, value, _ := strings.Cut(val, "=")
			vars = append(vars, name, awkUnescape(value))
		case "-f":
			progFiles = append(progFiles, val)
		}
	}
	operands := args[i:]

	var src strings.Builder
	if len(progFiles) == 0 {
		if len(operands) == 0 {
			return usage("missing program")
		}
		src.WriteString(operands[0])
		operands = operands[1:]
	}
	for _, name := range progFiles {
		var data []byte
		var err error
		if name == "-" {
			data, err = io.ReadAll(stdin)
		} else {
			var f *os.File
			if f, err = open(name, os.O_RDONLY, 0); err == nil {
				data, err = io.ReadAll(f)
				f.Close()
			}
		}
		if err != nil {
			fmt.Fprintf(stderr, "awk: %v\n", err)
			return 2
		}
		src.Write(data)
		src.WriteByte('\n')
	}

	prog, err := parser.ParseProgram([]byte(src.String()), nil)
	if err != nil {
		fmt.Fprintf(stderr, "awk: %v\n", err)
		return 2
	}
	var environ []string
	env.Each(func(name string, vr expand.Variable) bool {
		if vr.Exported {
			environ = append(environ, name, vr.String())
		}
		return true
	})
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	status, err := awkinterp.ExecProgram(prog, &awkinterp.Config{
		Stdin:         stdin,
		Output:        stdout,
		Error:         stderr,
		NewlineOutput: awkinterp.RawNewlineMode,
		Argv0:         "awk",
		Args:          operands,
		Vars:          vars,
		Environ:       environ,
		OpenFile:      open,
		NoExec:        true,
	})
	if err != nil {
		fmt.Fprintf(stderr, "awk: %v\n", err)
		return 2
	}
	return status
}

func awkUnescape(s string) string {
	if u, err := lexer.Unescape(s); err == nil {
		return u
	}
	return s
}
