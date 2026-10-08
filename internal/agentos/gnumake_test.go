package agentos

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeGNUMakeFixture(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func TestWantsGNUMake(t *testing.T) {
	gnuOnly := t.TempDir()
	writeGNUMakeFixture(t, filepath.Join(gnuOnly, "GNUmakefile"), "all:\n", 0o644)
	both := t.TempDir()
	writeGNUMakeFixture(t, filepath.Join(both, "GNUmakefile"), "all:\n", 0o644)
	writeGNUMakeFixture(t, filepath.Join(both, "Makefile"), "all:\n", 0o644)
	posix := t.TempDir()
	writeGNUMakeFixture(t, filepath.Join(posix, "Makefile"), "all:\n", 0o644)
	parent := filepath.Dir(gnuOnly)
	sub := filepath.Base(gnuOnly)

	for _, tc := range []struct {
		name string
		dir  string
		args []string
		want bool
	}{
		{"gnu only", gnuOnly, []string{"build"}, true},
		{"posix makefile wins", both, nil, false},
		{"posix only", posix, nil, false},
		{"explicit -f other", gnuOnly, []string{"-f", "other.mk"}, false},
		{"explicit -f GNUmakefile", posix, []string{"-f", "GNUmakefile"}, true},
		{"attached -fGNUmakefile", posix, []string{"-fGNUmakefile"}, true},
		{"--file=GNUmakefile", posix, []string{"--file=GNUmakefile"}, true},
		{"-C into gnu dir", parent, []string{"-C", sub, "all"}, true},
		{"-C attached", parent, []string{"-C" + sub}, true},
		{"--directory=", parent, []string{"--directory=" + sub}, true},
		{"missing dir", filepath.Join(gnuOnly, "nope"), nil, false},
	} {
		if got := wantsGNUMake(tc.dir, tc.args); got != tc.want {
			t.Errorf("%s: wantsGNUMake(%v) = %v, want %v", tc.name, tc.args, got, tc.want)
		}
	}
}

func TestFindGNUMake(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell-script stand-ins for make")
	}
	notGNU := t.TempDir()
	writeGNUMakeFixture(t, filepath.Join(notGNU, "make"), "#!/bin/sh\necho 'bashy make (POSIX)'\n", 0o755)
	gnu := t.TempDir()
	writeGNUMakeFixture(t, filepath.Join(gnu, "make"), "#!/bin/sh\necho 'GNU Make 4.4.1'\n", 0o755)
	gmake := t.TempDir()
	writeGNUMakeFixture(t, filepath.Join(gmake, "gmake"), "#!/bin/sh\necho 'GNU Make 4.4.1'\n", 0o755)

	list := func(dirs ...string) string { return strings.Join(dirs, string(os.PathListSeparator)) }
	if got := findGNUMake(list(notGNU, gnu)); got != filepath.Join(gnu, "make") {
		t.Errorf("skips a non-GNU make: got %q", got)
	}
	if got := findGNUMake(list(gnu, gmake)); got != filepath.Join(gmake, "gmake") {
		t.Errorf("prefers gmake anywhere on PATH: got %q", got)
	}
	if got := findGNUMake(notGNU); got != "" {
		t.Errorf("no GNU make: got %q", got)
	}
}
