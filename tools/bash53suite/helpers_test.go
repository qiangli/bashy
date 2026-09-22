package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The Go helpers must emit byte-for-byte what the corpus's C helpers emit
// on a Unix host: the fixtures' .right files were recorded against those.
func TestHelperRechoMatchesCorpusContract(t *testing.T) {
	var out bytes.Buffer
	code := helperRecho([]string{"a b", "\x01x\x7f", "", "tab\there"}, &out)
	if code != 0 {
		t.Fatalf("recho exit %d", code)
	}
	want := "argv[1] = <a b>\nargv[2] = <^Ax^?>\nargv[3] = <>\nargv[4] = <tab^Ihere>\n"
	if got := out.String(); got != want {
		t.Fatalf("recho = %q, want %q", got, want)
	}
}

func TestHelperZechoIsBareEcho(t *testing.T) {
	var out bytes.Buffer
	helperZecho([]string{"-n", "a", "b\\n", ""}, &out)
	if got, want := out.String(), "-n a b\\n \n"; got != want {
		t.Fatalf("zecho = %q, want %q", got, want)
	}
	out.Reset()
	helperZecho(nil, &out)
	if got := out.String(); got != "\n" {
		t.Fatalf("zecho with no args = %q, want newline", got)
	}
}

func TestHelperXcaseFoldsAndRejectsBadOptions(t *testing.T) {
	var out, errb bytes.Buffer
	if code := helperXcase([]string{"-u"}, strings.NewReader("aBc\n\x00z"), &out, &errb); code != 0 {
		t.Fatalf("xcase -u exit %d: %s", code, errb.String())
	}
	if got := out.String(); got != "ABC\n\x00Z" {
		t.Fatalf("xcase -u = %q", got)
	}
	out.Reset()
	helperXcase([]string{"-ln"}, strings.NewReader("ÀBC"), &out, &errb)
	if got := out.String(); got != "Àbc" {
		t.Fatalf("xcase -l leaves non-ASCII alone: %q", got)
	}
	out.Reset()
	errb.Reset()
	if code := helperXcase([]string{"-x"}, strings.NewReader(""), &out, &errb); code != 2 || !strings.Contains(errb.String(), "usage") {
		t.Fatalf("xcase -x: exit %d, stderr %q", code, errb.String())
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "in.txt")
	os.WriteFile(f, []byte("Mixed Case"), 0o644)
	out.Reset()
	helperXcase([]string{"-u", f}, strings.NewReader("ignored"), &out, &errb)
	if got := out.String(); got != "MIXED CASE" {
		t.Fatalf("xcase -u file = %q", got)
	}
	if code := helperXcase([]string{filepath.Join(dir, "missing")}, strings.NewReader(""), &out, &errb); code != 1 {
		t.Fatalf("xcase missing file exit %d", code)
	}
}

// argv[0] dispatch: the harness name never routes to a helper; a helper name
// does, with or without .exe, and (on Windows) in any case.
func TestRunAsHelperDispatchesByArgv0(t *testing.T) {
	var out bytes.Buffer
	if _, ok := runAsHelper("bash53suite", []string{"recho"}, strings.NewReader(""), &out, &out); ok {
		t.Fatal("harness name must not dispatch to a helper")
	}
	if _, ok := runAsHelper("/tmp/x/bash53suite.exe", nil, strings.NewReader(""), &out, &out); ok {
		t.Fatal("harness .exe name must not dispatch")
	}
	names := []string{"recho", "./recho", "/tmp/x/zecho", "xcase.exe"}
	if runtime.GOOS == "windows" {
		names = append(names, `C:\t\recho.exe`)
	}
	for _, argv0 := range names {
		out.Reset()
		if _, ok := runAsHelper(argv0, []string{"q"}, strings.NewReader("q"), &out, &out); !ok {
			t.Fatalf("%q did not dispatch", argv0)
		}
	}
	if runtime.GOOS == "windows" {
		if _, ok := runAsHelper("RECHO.EXE", nil, strings.NewReader(""), &out, &out); !ok {
			t.Fatal("Windows argv[0] is case-insensitive")
		}
	}
}

func TestPosixSpellingWindowsMode(t *testing.T) {
	cases := map[string]string{
		`D:\a\bashy\bashy\bin\bash.exe`: "/d/a/bashy/bashy/bin/bash.exe",
		`C:\`:                           "/c",
		`c:`:                            "/c",
		`C:\Users\r~1\Temp\bash53-1`:    "/c/Users/r~1/Temp/bash53-1",
		`\\server\share\x`:              "//server/share/x",
		`relative\path`:                 "relative/path",
		"":                              "",
	}
	for in, want := range cases {
		if got := posixSpelling(in, true); got != want {
			t.Errorf("posixSpelling(%q) = %q, want %q", in, got, want)
		}
	}
	if got := posixSpelling(`C:\x`, false); got != `C:\x` {
		t.Errorf("non-windows mode must be identity, got %q", got)
	}
}

func TestInstallGoHelpersLinksTheHarness(t *testing.T) {
	dir := t.TempDir()
	if err := installGoHelpers(dir); err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	selfInfo, _ := os.Stat(self)
	for _, h := range helperNames {
		p := filepath.Join(dir, exeName(h))
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("%s: %v", h, err)
		}
		if info.Size() != selfInfo.Size() {
			t.Fatalf("%s is not the harness binary (size %d vs %d)", h, info.Size(), selfInfo.Size())
		}
	}
}

// The userland root: every applet the multicall binary lists becomes
// <root>/usr/bin/<name>[.exe], sh and bash are the testee, /etc/passwd has
// root. A tiny script stands in for the multicall binary on Unix; the real
// yoke.exe is exercised by the Windows CI leg.
func TestPrepareUserlandLaysOutRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub multicall is a POSIX shell script; the CI leg covers the real binary")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "yoke")
	os.WriteFile(stub, []byte("#!/bin/sh\nprintf 'cat\\nsed\\n[\\nbad/name\\n'\n"), 0o755)
	testee := filepath.Join(dir, "bash")
	os.WriteFile(testee, []byte("#!/bin/sh\necho testee\n"), 0o755)
	root, bin, n, err := prepareUserland(dir, stub, testee)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("applets linked = %d, want 3 (bad/name skipped)", n)
	}
	if root != filepath.Join(dir, "root") || bin != filepath.Join(root, "usr", "bin") {
		t.Fatalf("root %q bin %q", root, bin)
	}
	for _, name := range []string{"cat", "sed", "[", "yoke", "bash", "sh"} {
		if _, err := os.Stat(filepath.Join(bin, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(bin, "bad")); err == nil {
		t.Error("bad/name must not be materialized")
	}
	pw, _ := os.ReadFile(filepath.Join(root, "etc", "passwd"))
	if !strings.HasPrefix(string(pw), "root:") {
		t.Errorf("passwd = %q", pw)
	}
	sh, _ := os.ReadFile(filepath.Join(bin, "sh"))
	if string(sh) != "#!/bin/sh\necho testee\n" {
		t.Errorf("sh is not the testee: %q", sh)
	}
}
