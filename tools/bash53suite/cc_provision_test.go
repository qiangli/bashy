package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/qiangli/yoke/external/zigcc"
	"github.com/qiangli/yoke/pkg/binmgr"
)

// The provisioning path pulls the pinned zig toolchain over the network, so it
// runs only when asked (BASH53_CC_PROVISION_TEST=1). It proves the two things
// the Windows glob-bracket fixture needs and that this host cannot otherwise
// exercise: (1) the embedded compat fnmatch header lets a windows-gnu fnmatch
// program build, and its results are byte-identical to the fixture's recorded
// fnmatch column (checked by running a NATIVE build — fnmatch is pure string
// logic, so the host result is a valid proxy for the cross-built one); (2) the
// strmatch stub lets a -shared link produce a PE DLL despite the otherwise
// unresolved `strmatch` symbol.
func ensureZigForTest(t *testing.T) (string, context.Context) {
	t.Helper()
	if os.Getenv("BASH53_CC_PROVISION_TEST") == "" {
		t.Skip("set BASH53_CC_PROVISION_TEST=1 to exercise the zig cc provisioning path (downloads the pinned toolchain)")
	}
	if !zigcc.Supported(binmgr.Platform()) {
		t.Skipf("no pinned zig toolchain for %s", binmgr.Platform())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	zig, err := zigcc.Ensure(ctx)
	if err != nil {
		t.Skipf("zig unavailable: %v", err)
	}
	return zig, ctx
}

func isPE(t *testing.T, path string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 2 {
		return false
	}
	return data[0] == 'M' && data[1] == 'Z'
}

// TestCompatFnmatchHeaderBuildsAndMatches builds the fixture's fnmatch program
// for x86_64-windows-gnu using only the embedded compat header (proving the
// compile resolves where mingw has no <fnmatch.h>), then builds it NATIVELY and
// checks every pcheck case glob-bracket actually feeds the binary.
func TestCompatFnmatchHeaderBuildsAndMatches(t *testing.T) {
	zig, ctx := ensureZigForTest(t)
	dir := t.TempDir()
	compat := filepath.Join(dir, "compat")
	if err := os.MkdirAll(compat, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compat, "fnmatch.h"), fnmatchCompatHeader, 0o644); err != nil {
		t.Fatal(err)
	}
	// The fixture's own fnmatch.c (glob-bracket.tests), verbatim.
	prog := `#include <fnmatch.h>
#include <stdlib.h>
#include <stdio.h>
int main(int argc, char **argv) {
  if (2 >= argc) { fprintf(stderr, "usage: fnmatch string pattern\n"); exit(2); }
#ifdef FNM_EXTMATCH
  int flags = FNM_PATHNAME | FNM_PERIOD | FNM_EXTMATCH;
#else
  int flags = FNM_PATHNAME | FNM_PERIOD;
#endif
  if (fnmatch(argv[2], argv[1], flags) == 0) return 0;
  return 1;
}
`
	src := filepath.Join(dir, "fnmatch.c")
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	// (1) It must cross-compile for the Windows target using the compat header.
	winExe := filepath.Join(dir, "fnmatch.exe")
	build := exec.CommandContext(ctx, zig, "cc", "-target", zigWindowsTarget(), "-isystem", compat, "-O2", "-o", winExe, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("windows-gnu fnmatch build failed: %v\n%s", err, out)
	}
	if !isPE(t, winExe) {
		t.Fatalf("%s is not a PE image", winExe)
	}

	// (2) A native build must reproduce the fixture's recorded fnmatch column
	// on the only cases glob-bracket feeds the binary (the pcheck section: the
	// string is always ab/cd/efg, flags FNM_PATHNAME|FNM_PERIOD).
	nativeBin := filepath.Join(dir, "fnmatch_native")
	nb := exec.CommandContext(ctx, zig, "cc", "-isystem", compat, "-O2", "-o", nativeBin, src)
	if out, err := nb.CombinedOutput(); err != nil {
		t.Fatalf("native fnmatch build failed: %v\n%s", err, out)
	}
	cases := []struct {
		pat  string
		want bool // true => match (fnmatch==0), the "yes" column
	}{
		{"ab/cd/efg", true}, {"ab[/]cd/efg", false}, {"ab[/a]cd/efg", false},
		{"ab[a/]cd/efg", false}, {"ab[!a]cd/efg", false}, {"ab[.-0]cd/efg", false},
		{"*/*/efg", true}, {"*[/]*/efg", false}, {"*[/a]*/efg", false},
		{"*[a/]*/efg", false}, {"*[!a]*/efg", false}, {"*[.-0]*/efg", false},
		{"*[b]/*/efg", true}, {"*[ab]/*/efg", true}, {"*[ba]/*/efg", true},
		{"*[!a]/*/efg", true}, {"*[a-c]/*/efg", true}, {"*/cd/efg", true},
	}
	for _, c := range cases {
		// fixture runs: fnmatch <string> <pattern>; prog does fnmatch(argv2,argv1).
		cmd := exec.CommandContext(ctx, nativeBin, "ab/cd/efg", c.pat)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		matched := err == nil
		if matched != c.want {
			t.Errorf("fnmatch pat=%q: matched=%v want=%v (stderr=%q)", c.pat, matched, c.want, stderr.String())
		}
	}
}

// TestStrmatchStubLinksSharedObject proves the strmatch stub lets the
// glob-bracket loadable link into a PE DLL on the Windows target, despite the
// strmatch symbol that a real bash would export at load time.
func TestStrmatchStubLinksSharedObject(t *testing.T) {
	zig, ctx := ensureZigForTest(t)
	dir := t.TempDir()
	target := zigWindowsTarget()

	// The fixture's strmatch.c leaves `strmatch` undefined on purpose.
	strmatchC := `struct word_desc { char* word; int flags; };
struct word_list { struct word_list* next; struct word_desc* word; };
int strmatch(char *pattern, char *string, int flags);
#define FNM_PATHNAME (1<<0)
#define FNM_PERIOD   (1<<2)
#define FNM_EXTMATCH (1<<5)
static int strmatch_builtin(struct word_list* list) {
  char *str, *pat;
  if (!list || !list->word) return 2;
  str = list->word->word;
  if (!list->next || !list->next->word) return 2;
  pat = list->next->word->word;
  return strmatch(pat, str, FNM_PATHNAME|FNM_PERIOD|FNM_EXTMATCH) == 0 ? 0 : 1;
}
struct builtin { const char* name; int (*function)(struct word_list*); int flags; };
struct builtin strmatch_struct = { "strmatch", strmatch_builtin, 1 };
`
	strmatchSrc := filepath.Join(dir, "strmatch.c")
	strmatchObj := filepath.Join(dir, "strmatch.o")
	if err := os.WriteFile(strmatchSrc, []byte(strmatchC), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(ctx, zig, "cc", "-target", target, "-fPIC", "-c", "-o", strmatchObj, strmatchSrc).CombinedOutput(); err != nil {
		t.Fatalf("strmatch.o compile: %v\n%s", err, out)
	}

	stubObj := filepath.Join(dir, "strmatch_stub.o")
	if err := compileWith(ctx, zig, target, "-c", stubObj, strmatchStubSource, "strmatch_stub.c", ""); err != nil {
		t.Fatalf("stub compile: %v", err)
	}

	// Without the stub the link fails; with it, a valid DLL appears.
	so := filepath.Join(dir, "strmatch.so")
	if out, err := exec.CommandContext(ctx, zig, "cc", "-target", target, "-shared", "-o", so, strmatchObj, stubObj).CombinedOutput(); err != nil {
		t.Fatalf("strmatch.so link with stub: %v\n%s", err, out)
	}
	if !isPE(t, so) {
		t.Fatalf("%s is not a PE image", so)
	}
}
