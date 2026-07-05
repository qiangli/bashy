package main

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = `# leading comment, column 0: dropped everywhere

%prep

  mkdir foo.tmp
  cd foo.tmp

  touch a b

%test

  print one
0:simple case
>one

  print err >&2; false
1d:status only, stdout ignored
?err

# comment inside test section
  echo pat AAA
0:pattern stdout
*>pat A#

  read line; print "got $line"
0q:stdin + q-flag expected
<hello
>got $word

  false
1f:expected failure

%clean

  cd ..
`

func TestParse(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "S01sample.ztst")
	if err := os.WriteFile(p, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	fx, err := parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(fx.prep) != 2 {
		t.Fatalf("prep chunks = %d, want 2", len(fx.prep))
	}
	if fx.prep[0] != "  mkdir foo.tmp\n  cd foo.tmp" {
		t.Errorf("prep[0] = %q", fx.prep[0])
	}
	if len(fx.clean) != 1 {
		t.Fatalf("clean chunks = %d, want 1", len(fx.clean))
	}
	if len(fx.tests) != 5 {
		t.Fatalf("tests = %d, want 5", len(fx.tests))
	}
	tt := fx.tests
	if tt[0].xstatus != "0" || tt[0].message != "simple case" || len(tt[0].out) != 1 || tt[0].out[0] != "one" {
		t.Errorf("test0 = %+v", tt[0])
	}
	if tt[1].xstatus != "1" || tt[1].flags != "d" || len(tt[1].errOut) != 1 || tt[1].errOut[0] != "err" {
		t.Errorf("test1 = %+v", tt[1])
	}
	if !tt[2].patOut || tt[2].out[0] != "pat A#" {
		t.Errorf("test2 = %+v", tt[2])
	}
	if tt[3].flags != "q" || tt[3].stdin[0] != "hello" || tt[3].out[0] != "got $word" {
		t.Errorf("test3 = %+v", tt[3])
	}
	if tt[4].flags != "f" || tt[4].xstatus != "1" || len(tt[4].out) != 0 {
		t.Errorf("test4 = %+v", tt[4])
	}
}

func TestParseRejectsStrayStatus(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "S02bad.ztst")
	os.WriteFile(p, []byte("%test\n\n  echo hi\n\n0:status after blank\n"), 0o644)
	if _, err := parse(p); err == nil {
		t.Fatal("expected parse error for status line detached from its chunk")
	}
}

func TestJoinBlock(t *testing.T) {
	if joinBlock(nil) != "" {
		t.Error("empty block should be empty string")
	}
	if joinBlock([]string{"a", "b"}) != "a\nb\n" {
		t.Error("block should be newline-joined with trailing newline")
	}
}
