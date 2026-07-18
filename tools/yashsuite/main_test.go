package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoveryStability(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.p.tst", "alias-p.tst", "ignore-y.tst", "a.p.tst"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := discoverFixtures(dir)
	if err != nil {
		t.Fatal(err)
	}
	if names := fixtureNames(got); !reflect.DeepEqual(names, []string{"a.p", "alias-p", "z.p"}) {
		t.Fatalf("fixture names = %v", names)
	}
}

func TestManifestValidation(t *testing.T) {
	fixtures := fixtures("a", "b", "c")
	valid := &manifest{SchemaVersion: 1, Suite: "yash", ChunkCount: 2, Chunks: []manifestChunk{
		{ID: 1, Fixtures: []manifestFixture{{Name: "a"}, {Name: "c"}}},
		{ID: 2, Fixtures: []manifestFixture{{Name: "b"}}},
	}}
	if err := validateManifest(valid, fixtures); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	missing := *valid
	missing.Chunks = []manifestChunk{{ID: 1, Fixtures: []manifestFixture{{Name: "a"}}}, {ID: 2, Fixtures: []manifestFixture{{Name: "b"}}}}
	if err := validateManifest(&missing, fixtures); err == nil {
		t.Fatal("missing fixture accepted")
	}
	duplicate := *valid
	duplicate.Chunks = []manifestChunk{{ID: 1, Fixtures: []manifestFixture{{Name: "a"}, {Name: "b"}}}, {ID: 2, Fixtures: []manifestFixture{{Name: "b"}, {Name: "c"}}}}
	if err := validateManifest(&duplicate, fixtures); err == nil {
		t.Fatal("duplicate fixture accepted")
	}
}

func TestOfShardCorrectness(t *testing.T) {
	all := fixtures("a", "b", "c", "d", "e", "f", "g")
	want := [][]string{{"a", "d", "g"}, {"b", "e"}, {"c", "f"}}
	seen := map[string]int{}
	for shard := 1; shard <= 3; shard++ {
		got := fixtureNames(selectShard(all, 3, shard))
		if !reflect.DeepEqual(got, want[shard-1]) {
			t.Fatalf("shard %d = %v, want %v", shard, got, want[shard-1])
		}
		for _, name := range got {
			seen[name]++
		}
	}
	for _, fixture := range all {
		if seen[fixture.Name] != 1 {
			t.Fatalf("fixture %s assigned %d times", fixture.Name, seen[fixture.Name])
		}
	}
}

func fixtures(names ...string) []fixture {
	out := make([]fixture, 0, len(names))
	for _, name := range names {
		out = append(out, fixture{Name: name})
	}
	return out
}

func fixtureNames(fixtures []fixture) []string {
	out := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		out = append(out, fixture.Name)
	}
	return out
}
