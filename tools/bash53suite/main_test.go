package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

var bash53FixtureNames = []string{
	"alias", "appendop", "arith", "arith-for", "array", "array2", "assoc", "attr",
	"braces", "builtins", "case", "casemod", "complete", "comsub", "comsub-eof",
	"comsub-posix", "comsub2", "cond", "coproc", "cprint", "dbg-support",
	"dbg-support2", "dirstack", "dollars", "dynvar", "errors", "execscript",
	"exp-tests", "exportfunc", "extglob", "extglob2", "extglob3", "func",
	"getopts", "glob-bracket", "glob-test", "globstar", "heredoc", "herestr",
	"histexpand", "history", "ifs", "ifs-posix", "input-test", "intl", "invert",
	"invocation", "iquote", "jobs", "lastpipe", "mapfile", "more-exp", "nameref",
	"new-exp", "nquote", "nquote1", "nquote2", "nquote3", "nquote4", "nquote5",
	"parser", "posix2", "posixexp", "posixexp2", "posixpat", "posixpipe",
	"precedence", "printf", "procsub", "quote", "quotearray", "read", "redir",
	"rhs-exp", "rsh", "set-e", "set-x", "shopt", "strip", "test", "tilde",
	"tilde2", "trap", "type", "varenv", "vredir",
}

func TestCommittedChunkManifestCoversEveryBash53FixtureExactlyOnce(t *testing.T) {
	root := repoRoot(t)
	manifest, err := loadChunkManifest(filepath.Join(root, "chunks.json"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures := fixturesFromNames(bash53FixtureNames)
	if err := validateChunkManifest(manifest, fixtures); err != nil {
		t.Fatal(err)
	}

	seen := map[string]int{}
	for _, chunk := range manifest.Chunks {
		if chunk.Seconds <= 0 {
			t.Fatalf("chunk %d duration = %v, want > 0", chunk.ID, chunk.Seconds)
		}
		for _, f := range chunk.Fixtures {
			if f.Seconds <= 0 {
				t.Fatalf("fixture %s duration = %v, want > 0", f.Name, f.Seconds)
			}
			seen[f.Name]++
		}
	}
	if got, want := len(seen), len(bash53FixtureNames); got != want {
		t.Fatalf("manifest names = %d, want %d", got, want)
	}
}

func TestChunkManifestValidationRejectsCoverageLoss(t *testing.T) {
	fixtures := fixturesFromNames([]string{"a", "b", "c"})
	valid := &chunkManifest{
		SchemaVersion: 1,
		Suite:         "bash-5.3",
		ChunkCount:    2,
		Chunks: []manifestChunk{
			{ID: 1, Fixtures: []manifestFixture{{Name: "a"}}},
			{ID: 2, Fixtures: []manifestFixture{{Name: "b"}, {Name: "c"}}},
		},
	}
	if err := validateChunkManifest(valid, fixtures); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	missing := *valid
	missing.Chunks = []manifestChunk{
		{ID: 1, Fixtures: []manifestFixture{{Name: "a"}}},
		{ID: 2, Fixtures: []manifestFixture{{Name: "b"}}},
	}
	if err := validateChunkManifest(&missing, fixtures); err == nil {
		t.Fatal("missing fixture accepted")
	}

	duplicate := *valid
	duplicate.Chunks = []manifestChunk{
		{ID: 1, Fixtures: []manifestFixture{{Name: "a"}, {Name: "b"}}},
		{ID: 2, Fixtures: []manifestFixture{{Name: "b"}, {Name: "c"}}},
	}
	if err := validateChunkManifest(&duplicate, fixtures); err == nil {
		t.Fatal("duplicate fixture accepted")
	}

	unknown := *valid
	unknown.Chunks = []manifestChunk{
		{ID: 1, Fixtures: []manifestFixture{{Name: "a"}}},
		{ID: 2, Fixtures: []manifestFixture{{Name: "b"}, {Name: "c"}, {Name: "d"}}},
	}
	if err := validateChunkManifest(&unknown, fixtures); err == nil {
		t.Fatal("unknown fixture accepted")
	}
}

func TestManifestChunkSelectionIgnoresFleetSizeModulo(t *testing.T) {
	fixtures := fixturesFromNames([]string{"a", "b", "c", "d"})
	manifest := &chunkManifest{
		SchemaVersion: 1,
		Suite:         "bash-5.3",
		ChunkCount:    2,
		Chunks: []manifestChunk{
			{ID: 1, Fixtures: []manifestFixture{{Name: "d"}, {Name: "b"}}},
			{ID: 2, Fixtures: []manifestFixture{{Name: "a"}, {Name: "c"}}},
		},
	}
	selected := selectFixtures(fixtures, nil, "1/2", manifest)
	got := fixtureNames(selected)
	want := []string{"b", "d"}
	if !sameStrings(got, want) {
		t.Fatalf("selected fixtures = %v, want %v", got, want)
	}
}

func fixturesFromNames(names []string) []fixture {
	out := make([]fixture, 0, len(names))
	for _, name := range names {
		out = append(out, fixture{Name: name})
	}
	return out
}

func fixtureNames(fixtures []fixture) []string {
	out := make([]string, 0, len(fixtures))
	for _, f := range fixtures {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatal("could not find repo root")
		}
		dir = next
	}
}
