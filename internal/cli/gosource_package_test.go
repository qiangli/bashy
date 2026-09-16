package cli

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/gosource"
)

func TestSprint198PackageToken(t *testing.T) {
	for _, tc := range []struct {
		source   string
		selected bool
	}{
		{"package main", true}, {" \t\n//go:build linux\n/* unchanged */\npackage main", true},
		{"//line original.go:12\npackage", true}, {"package ! invalid", true},
		{"packaged main", false}, {"\"package\" main", false}, {"command package main", false},
		{"#!/bin/bash\npackage main", false}, {"# shell comment\npackage main", false},
		{"echo first\npackage main", false}, {"/* unterminated package", false},
	} {
		if got := leadingGoPackage([]byte(tc.source)); got != tc.selected {
			t.Errorf("%q: selected=%v", tc.source, got)
		}
	}
}

func TestSprint198PackageInputCollection(t *testing.T) {
	const source = "//go:build linux\n/* prefix */\npackage main\nfunc main(){ x:=`$literal`; x=\"$unchanged\"; _=x }\n"
	for _, form := range []string{"command", "file", "stdin"} {
		t.Run(form, func(t *testing.T) {
			operand, cmd := "", ""
			var stdin io.Reader
			switch form {
			case "command":
				cmd = source
			case "file":
				operand = filepath.Join(t.TempDir(), "source.any-extension")
				if err := os.WriteFile(operand, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			case "stdin":
				stdin = strings.NewReader(source)
			}
			in, _, selected, err := collectPackageGoSource(operand, cmd, stdin)
			if err != nil || !selected || len(in.Files) != 1 || string(in.Files[0].Data) != source {
				t.Fatalf("selected=%v input=%+v err=%v", selected, in, err)
			}
		})
	}
	const shell = "echo '$literal'\n"
	_, replay, selected, err := collectPackageGoSource("", "", strings.NewReader(shell))
	data, readErr := io.ReadAll(replay)
	if err != nil || readErr != nil || selected || string(data) != shell {
		t.Fatalf("shell replay %q selected=%v errors=%v/%v", data, selected, err, readErr)
	}
}

func TestSprint198PackageBeforeShellPreflight(t *testing.T) {
	for _, form := range []string{"command", "file", "stdin", "command-posix", "command-bash"} {
		for _, source := range []string{
			"//line original.go:10\n/* prefix */\npackage main\nfunc main(){x:=`$literal`;x=\"$unchanged\";_=x}\n",
			"package ! malformed; echo shell-must-not-run\n",
		} {
			t.Run(form+"/"+source, func(t *testing.T) {
				oldArgs, oldOriginal := os.Args, originalArgs
				oldCommand, oldRead, oldForce := *command, *readStdin, *forceI
				oldSel, oldRes, oldErr := startupGoSourceSel, startupGoSource, startupGoSourceErr
				oldBashPP, oldDefault := startupBashPP, AgentOSBashPPDefault
				oldStdin := os.Stdin
				t.Cleanup(func() {
					os.Args, originalArgs = oldArgs, oldOriginal
					*command, *readStdin, *forceI = oldCommand, oldRead, oldForce
					startupGoSourceSel, startupGoSource, startupGoSourceErr = oldSel, oldRes, oldErr
					startupBashPP, AgentOSBashPPDefault = oldBashPP, oldDefault
					os.Stdin = oldStdin
					_ = flag.CommandLine.Parse(nil)
				})
				AgentOSBashPPDefault = true
				startupGoSourceSel, startupGoSource, startupGoSourceErr = GoSourceSelection{}, GoSourceResolution{}, nil
				*command, *readStdin, *forceI = "", false, false
				os.Args = []string{"bashy"}
				originalArgs = []string{"bashy", "--bashpp"}
				switch form {
				case "command", "command-posix", "command-bash":
					*command = source
				case "file":
					os.Args = append(os.Args, writeGoFixture(t, source))
				case "stdin":
					f, err := os.CreateTemp(t.TempDir(), "stdin")
					if err != nil {
						t.Fatal(err)
					}
					if _, err = f.WriteString(source); err != nil {
						t.Fatal(err)
					}
					_, _ = f.Seek(0, 0)
					os.Stdin = f
					t.Cleanup(func() { f.Close() })
				}
				if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
					t.Fatal(err)
				}
				want := "selected Go frontend diagnostic"
				if form == "command-posix" {
					os.Args = append(os.Args, "--posix")
					originalArgs = append(originalArgs, "--posix")
					want = "bashy: --source=go is not available in POSIX mode"
				}
				if form == "command-bash" {
					AgentOSBashPPDefault = false
					want = "bashy: --source=go requires the bashy front door"
				}
				called := false
				withGoSourceHook(t, func(files []GoSourceFile, _ GoSourceOptions) (*GoSourceProgram, error) {
					called = true
					if len(files) != 1 || string(files[0].Data) != source {
						t.Fatalf("source changed: %+v", files)
					}
					return nil, errors.New("selected Go frontend diagnostic")
				})
				var err error
				diagnostic := captureStderr(t, func() { err = runAll() })
				wantCalled := form != "command-posix" && form != "command-bash"
				if called != wantCalled || err == nil || strings.TrimSpace(diagnostic) != want {
					t.Fatalf("called=%v err=%v diagnostic=%q", called, err, diagnostic)
				}
			})
		}
	}
}

func TestSprint198PackageGoLexicalSemantics(t *testing.T) {
	oldDialect := startupBashPP
	startupBashPP = BashPPResolution{Enabled: true}
	t.Cleanup(func() { startupBashPP = oldDialect })
	const source = "package main\nfunc main(){ x:=`$HOME`; x=x+\" $literal\"; println(x) }\n"
	withGoSourceSelection(t, GoSourceResolution{Enabled: true})
	withGoSourceHook(t, func(files []GoSourceFile, opts GoSourceOptions) (*GoSourceProgram, error) {
		p, err := gosource.Load([]gosource.Source{{Name: files[0].Name, Data: files[0].Data}}, gosource.Options{RunMain: opts.RunMain})
		if err != nil {
			return nil, err
		}
		return &GoSourceProgram{File: p.File, Package: p.Package, Main: p.Main, InitFunctions: p.InitFunctions}, nil
	})
	in, _, selected, err := collectPackageGoSource("", source, nil)
	if err != nil || !selected {
		t.Fatalf("selection: %v %v", selected, err)
	}
	var runErr error
	out := captureStderr(t, func() { runErr = runGoSourceInput(in, "") })
	if runErr != nil || out != "$HOME $literal\n" {
		t.Fatalf("Go literals/assignment: output=%q error=%v", out, runErr)
	}
}
