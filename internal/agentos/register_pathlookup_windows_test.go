//go:build windows

// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
)

// TestResolveCmdWindowsPathSpellings is the instrumented Windows gate that
// replaces the tour's `windows=todo:1ec1081071d7` marker for commands/register.
//
// The tour's `commands/register` case reported `command not found` on GitHub's
// Windows leg, where the checkout is on `D:\a\…`. Source reading on macOS could
// not explain it, because the interpreter's own lookup (interp.LookPathDir)
// converts every drive spelling correctly. The failing lookup was bashy's own
// resolveCmd, which re-implemented the scan and got three Windows details wrong
// (POSIX exec bit, missing PATHEXT, MSYS-form element fed straight to os.Stat).
//
// This test proves BOTH lookups now resolve a program the way the tour needs
// it, across every spelling a Windows shell can hold for one directory in PATH:
// native backslash, native forward slash, and the MSYS/Git-Bash drive form
// (`/c/…`) that `$PWD` hands scripts. The conversion is drive-agnostic, so the
// runner's temp dir (on C:) exercises the exact code path the D:\ checkout hit.
// It is instrumented (t.Logf per spelling) so a real Windows run localizes any
// residual failure to a spelling instead of leaving a bare todo marker.
func TestResolveCmdWindowsPathSpellings(t *testing.T) {
	dir := t.TempDir()
	const base = "zqfakeprog" // not a coreutils applet, not a registered word
	if err := os.WriteFile(filepath.Join(dir, base+".exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	spellings := map[string]string{
		"native-backslash": dir,                   // C:\Users\...\Temp\Tx
		"native-slash":     filepath.ToSlash(dir), // C:/Users/.../Temp/Tx
	}
	if len(dir) >= 2 && dir[1] == ':' {
		drive := strings.ToLower(dir[:1])
		// MSYS/Git-Bash drive form: /c/Users/.../Temp/Tx — the spelling a shell
		// hands scripts and the one behind the D:\ checkout "command not found".
		spellings["msys-drive"] = "/" + drive + filepath.ToSlash(dir)[2:]
	}

	for label, pathElem := range spellings {
		env := expand.ListEnviron("PATH="+pathElem, "PATHEXT=.COM;.EXE;.BAT;.CMD")

		// The shell's own lookup first: a miss here means the interpreter, not
		// bashy, cannot find the program on this PATH spelling.
		if p, err := interp.LookPathDir(dir, env, base); err != nil {
			t.Errorf("[%s] interp.LookPathDir(PATH=%q) not found: %v", label, pathElem, err)
		} else {
			t.Logf("[%s] interp.LookPathDir(PATH=%q) -> %q", label, pathElem, p)
		}

		// bashy's commands/register-side resolver (dry-run/verify/commands).
		if got, ok := resolveCmd(dir, base, env); !ok {
			t.Errorf("[%s] resolveCmd(PATH=%q) reported %q missing", label, pathElem, base)
		} else {
			t.Logf("[%s] resolveCmd(PATH=%q) -> %q", label, pathElem, got)
		}
	}
}
