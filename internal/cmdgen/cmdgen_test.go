// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package cmdgen

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/shape/slot"
)

func TestGenerateMainGolden(t *testing.T) {
	cases := []struct {
		name   string
		entry  Entry
		golden string
	}{
		{
			name: "cobra_simple",
			entry: Entry{
				Name:        "stats",
				Kind:        KindCobra,
				Module:      "github.com/qiangli/yoke",
				Version:     "v0.0.0-20261121110346-da4b02a4d244",
				Package:     "github.com/qiangli/yoke/pkg/stats",
				Constructor: "NewCmd",
			},
			golden: "cobra_simple.golden",
		},
		{
			name: "cobra_wired",
			entry: Entry{
				Name:          "dag",
				Kind:          KindCobra,
				Module:        "github.com/qiangli/yoke",
				Version:       "v0.0.0-20261121110346-da4b02a4d244",
				Package:       "github.com/qiangli/yoke/pkg/dag",
				Constructor:   "NewDagCmd",
				Use:           "dag",
				ExitFunc:      "ExitCodeOf",
				PrependArgs:   []string{"--no-color"},
				ExtraImports:  []string{"github.com/qiangli/yoke/pkg/fleet"},
				Setup:         []string{"_ = fleet.New()"},
				PostConstruct: []string{"cmd.SilenceUsage = true"},
			},
			golden: "cobra_wired.golden",
		},
		{
			name: "tool_applet",
			entry: Entry{
				Name:    "tz",
				Kind:    KindTool,
				Module:  "github.com/qiangli/yoke",
				Version: "v0.0.0-20261121110346-da4b02a4d244",
				Package: "github.com/qiangli/yoke/cmds/tz",
			},
			golden: "tool_applet.golden",
		},
		{
			name: "func_entry",
			entry: Entry{
				Name:          "mod",
				Kind:          KindFunc,
				Module:        "github.com/qiangli/yoke",
				Version:       "v0.0.0-20261121110346-da4b02a4d244",
				Package:       "github.com/qiangli/yoke/pkg/gomod",
				Constructor:   "Run",
				ArgsSignature: SigArgsOutErr,
			},
			golden: "func_entry.golden",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GenerateMain(tc.entry)
			if err != nil {
				t.Fatalf("GenerateMain: %v", err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tc.golden))
			if err != nil {
				t.Fatalf("read golden %s: %v", tc.golden, err)
			}
			if string(got) != string(want) {
				t.Fatalf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", tc.name, got, want)
			}
		})
	}
}

func TestCatalogDirectoryAndDisjointVerbLists(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"stats.yaml": `name: stats
kind: cobra
batch: pilot
module: github.com/qiangli/yoke
package: github.com/qiangli/yoke/pkg/stats
constructor: NewCmd
`,
		"tz.yaml": `name: tz
kind: tool
batch: p4a
module: github.com/qiangli/yoke
package: github.com/qiangli/yoke/cmds/tz
`,
		"mod.yaml": `name: mod
aliases: [gomod]
kind: func
batch: p4b
module: github.com/qiangli/yoke
package: github.com/qiangli/yoke/pkg/gomod
constructor: Run
args_signature: args_out_err
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := LoadCatalog(dir)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}

	// Disjoint verb-list files as parallel P4 workers consume them.
	listA := filepath.Join(dir, "worker-a.txt")
	listB := filepath.Join(dir, "worker-b.txt")
	if err := os.WriteFile(listA, []byte("# P4a batch\ntz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(listB, []byte("# P4b batch\nstats, gomod\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verbsA, err := ReadVerbListFile(listA)
	if err != nil {
		t.Fatal(err)
	}
	verbsB, err := ReadVerbListFile(listB)
	if err != nil {
		t.Fatal(err)
	}
	selA, err := SelectVerbs(entries, verbsA)
	if err != nil || len(selA) != 1 || selA[0].Name != "tz" {
		t.Fatalf("SelectVerbs(A) = %v, %v", selA, err)
	}
	selB, err := SelectVerbs(entries, verbsB)
	if err != nil || len(selB) != 2 || selB[0].Name != "mod" || selB[1].Name != "stats" {
		t.Fatalf("SelectVerbs(B) = %v, %v", selB, err)
	}

	// Missing verb fails loudly.
	if _, err := SelectVerbs(entries, []string{"tz", "nonexistent"}); err == nil || !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("expected error naming missing verb, got %v", err)
	}

	// Duplicate alias across files is rejected.
	if err := os.WriteFile(filepath.Join(dir, "dup.yaml"), []byte("name: other\naliases: [gomod]\nkind: tool\nmodule: m\npackage: m/p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalog(dir); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestGenerateGoModResolvesVersionsAndReplaces(t *testing.T) {
	goMod := ParseGoMod([]byte(`module github.com/qiangli/bashy

go 1.27.0

toolchain go1.27.4

require (
	github.com/qiangli/coreutils v0.0.0-20261119135715-c656935287fa
	github.com/qiangli/yoke v0.0.0-20261121110346-da4b02a4d244
)

replace github.com/ergochat/readline => github.com/qiangli/readline v0.1.4-20261013004858-df59d4e258e4
`))
	entry := Entry{
		Name:    "tz",
		Kind:    KindTool,
		Module:  "github.com/qiangli/yoke",
		Package: "github.com/qiangli/yoke/cmds/tz",
	}
	got, err := GenerateGoMod(entry, GenerateOptions{GoMod: goMod})
	if err != nil {
		t.Fatalf("GenerateGoMod: %v", err)
	}
	s := string(got)
	for _, want := range []string{
		"module bashy-cmd/tz",
		"go 1.27.0",
		"toolchain go1.27.4",
		"github.com/qiangli/yoke v0.0.0-20261121110346-da4b02a4d244",
		"github.com/qiangli/coreutils v0.0.0-20261119135715-c656935287fa",
		"replace github.com/ergochat/readline => github.com/qiangli/readline v0.1.4-20261013004858-df59d4e258e4",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("go.mod missing %q:\n%s", want, s)
		}
	}
}

func TestBuildCacheAndShapeSlotLifecycle(t *testing.T) {
	// Build a tiny synthetic Go module in t.TempDir() so this test runs fast,
	// offline, and proves: generate -> tree-hash build cache -> slot Stage +
	// Activate -> cache hit -> source edit -> new tree-hash + Activate v2 ->
	// Undo -> Restore("release").
	root := t.TempDir()
	modDir := filepath.Join(root, "demo")
	pkgDir := filepath.Join(modDir, "pkg", "hello")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module example.com/demo\n\ngo 1.27.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writePkg := func(msg string) {
		t.Helper()
		body := "package hello\n\nimport (\n\t\"fmt\"\n\t\"io\"\n\t\"strings\"\n)\n\nfunc Run(args []string, stdout, stderr io.Writer) int {\n\tfmt.Fprintf(stdout, \"" + msg + ":%s\\n\", strings.Join(args, \",\"))\n\treturn 0\n}\n"
		if err := os.WriteFile(filepath.Join(pkgDir, "hello.go"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writePkg("v1")

	home := filepath.Join(root, "bashy-home")
	entry := Entry{
		Name:          "hello",
		Kind:          KindFunc,
		Module:        "example.com/demo",
		Version:       "v0.0.0",
		Package:       "example.com/demo/pkg/hello",
		Constructor:   "Run",
		ArgsSignature: SigArgsOutErr,
	}
	opts := BuildOptions{
		Home:         home,
		LocalModules: map[string]string{"example.com/demo": modDir},
	}

	// 1. First EnsureSlot: cache miss -> build -> Stage -> Activate v1.
	v1, res1, err := EnsureSlot(context.Background(), entry, opts)
	if err != nil {
		t.Fatalf("EnsureSlot v1: %v", err)
	}
	if res1.CacheHit {
		t.Fatal("first build reported CacheHit=true")
	}
	out1, err := exec.Command(v1.Path, "a", "b").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out1)) != "v1:a,b" {
		t.Fatalf("run v1 = (%q, %v), want v1:a,b", out1, err)
	}

	// 2. Second BuildToCache with unchanged tree -> cache hit, identical tree hash.
	resHit, err := BuildToCache(context.Background(), entry, opts)
	if err != nil {
		t.Fatalf("BuildToCache hit: %v", err)
	}
	if !resHit.CacheHit || resHit.TreeHash != res1.TreeHash {
		t.Fatalf("expected cache hit with hash %s, got hit=%v hash=%s", res1.TreeHash, resHit.CacheHit, resHit.TreeHash)
	}

	// 3. Edit source -> Rebuild & Activate v2.
	writePkg("v2")
	res2, err := BuildAndActivateSlot(context.Background(), entry, opts)
	if err != nil {
		t.Fatalf("BuildAndActivateSlot v2: %v", err)
	}
	if res2.CacheHit || res2.TreeHash == res1.TreeHash {
		t.Fatalf("edit did not change tree hash: hit=%v hash1=%s hash2=%s", res2.CacheHit, res1.TreeHash, res2.TreeHash)
	}
	out2, err := exec.Command(res2.Version.Path, "x").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out2)) != "v2:x" {
		t.Fatalf("run v2 = (%q, %v), want v2:x", out2, err)
	}

	// 4. Undo swaps back to v1; EnsureSlot honors the Undo selection.
	store := slot.Store{Home: home, App: "hello"}
	if _, err := store.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	afterUndo, _, err := EnsureSlot(context.Background(), entry, opts)
	if err != nil {
		t.Fatalf("EnsureSlot after Undo: %v", err)
	}
	if afterUndo.Digest != v1.Digest {
		t.Fatalf("after Undo digest = %s, want v1 %s", afterUndo.Digest, v1.Digest)
	}
	outUndo, err := exec.Command(afterUndo.Path, "u").CombinedOutput()
	if err != nil || strings.TrimSpace(string(outUndo)) != "v1:u" {
		t.Fatalf("run after Undo = (%q, %v), want v1:u", outUndo, err)
	}

	// 5. Undo again swaps back to v2; Restore("release") returns to v1.
	if _, err := store.Undo(); err != nil {
		t.Fatalf("second Undo: %v", err)
	}
	if st, err := store.Restore("release"); err != nil || st.Active == nil || st.Active.Digest != v1.Digest {
		t.Fatalf("Restore(release) = (%+v, %v), want v1 %s", st, err, v1.Digest)
	}
}
