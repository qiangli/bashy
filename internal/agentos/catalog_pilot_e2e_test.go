// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/bashy/internal/agentos/cmdcatalog"
	"github.com/qiangli/bashy/internal/cmdgen"
	"github.com/qiangli/yoke/pkg/shape/slot"
)

// TestPilotCatalogDispatchE2E proves the Sprint 677 P3 mechanism end-to-end on
// one real command ("stats"):
//   1. Catalog lookup resolves stats -> github.com/qiangli/yoke/pkg/stats.NewCmd.
//   2. First `bashy stats ...` builds on demand via cmdgen into the tree-hash
//      build cache and stages/activates a yoke/pkg/shape/slot side slot.
//   3. Output, --help (classic + skill), shell shim (`bashy -c 'stats ...'`),
//      and non-zero exit codes match the in-process baseline byte-for-byte.
//   4. Warm dispatch hits the active slot with zero rebuild.
//   5. An isolated source edit -> targeted package test -> generate/build ->
//      Stage/Activate -> verified live dispatch cycle completes in seconds,
//      and slot Undo / Restore("release") recover cleanly.
func TestPilotCatalogDispatchE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pilot e2e build in -short mode")
	}

	entry, ok := cmdcatalog.Lookup("stats")
	if !ok {
		t.Fatal("pilot command stats not found in embedded cmdcatalog")
	}
	if entry.Package != "github.com/qiangli/yoke/pkg/stats" || entry.Constructor != "NewCmd" {
		t.Fatalf("unexpected stats catalog entry: %+v", entry)
	}

	bashyRoot := pilotFindBashyRoot(t)
	umbrellaRoot := filepath.Dir(bashyRoot)
	goWork := filepath.Join(umbrellaRoot, "go.work")
	if _, err := os.Stat(goWork); err != nil {
		t.Skipf("umbrella go.work not found at %s: %v", goWork, err)
	}

	// Build or reuse the real bashy binary.
	bashyBin := strings.TrimSpace(os.Getenv("BASHY_PILOT_BIN"))
	if bashyBin == "" {
		bashyBin = filepath.Join(t.TempDir(), "bashy")
		if runtime.GOOS == "windows" {
			bashyBin += ".exe"
		}
		buildCmd := exec.Command("go", "build", "-o", bashyBin, "./cmd/bashy")
		buildCmd.Dir = bashyRoot
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("build bashy for pilot e2e: %v\n%s", err, out)
		}
	}

	home := t.TempDir()
	baseEnv := append(os.Environ(),
		"BASHY_HOME="+home,
		"BASHY_CMD_GOWORK="+goWork,
		"BASHY_HINTS=off",
		"OTEL_SDK_DISABLED=true",
	)
	run := func(extraEnv []string, stdin string, args ...string) (stdout, stderr string, code int, elapsed time.Duration) {
		t.Helper()
		start := time.Now()
		cmd := exec.Command(bashyBin, args...)
		cmd.Env = append(append([]string(nil), baseEnv...), extraEnv...)
		cmd.Stdin = strings.NewReader(stdin)
		var outBuf, errBuf strings.Builder
		cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
		err := cmd.Run()
		elapsed = time.Since(start)
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("exec bashy %v: %v", args, err)
		}
		return outBuf.String(), errBuf.String(), code, elapsed
	}

	sampleJSONL := "{\"instance_id\":\"i1\",\"resolved\":true}\n{\"instance_id\":\"i2\",\"resolved\":false}\n"

	// 1. Cold on-demand slot build via `bashy stats summary --json`.
	slotOut, slotErr, slotCode, coldDur := run(nil, sampleJSONL, "stats", "summary", "--json")
	if slotCode != 0 {
		t.Fatalf("cold `bashy stats summary --json` exit %d\nstdout=%s\nstderr=%s", slotCode, slotOut, slotErr)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(slotOut), &doc); err != nil {
		t.Fatalf("decode stats json: %v\n%s", err, slotOut)
	}
	if doc["schema_version"] != "bashy-stats-summary-v1" {
		t.Fatalf("schema_version = %v, want bashy-stats-summary-v1", doc["schema_version"])
	}

	// Confirm the shape slot and tree-hash build cache were both populated.
	store := slot.Store{Home: home, App: "stats"}
	v1, found, err := store.Select()
	if err != nil || !found {
		t.Fatalf("slot.Store.Select() after cold dispatch = (%+v, %v, %v)", v1, found, err)
	}
	cacheEntries, err := os.ReadDir(filepath.Join(home, "shape-cache", "cmd", "stats"))
	if err != nil || len(cacheEntries) == 0 {
		t.Fatalf("tree-hash build cache missing under shape-cache/cmd/stats: %v", err)
	}
	t.Logf("pilot cold on-demand build+dispatch: %s (slot=%s, digest=%s)", coldDur.Round(time.Millisecond), v1.Path, v1.Digest[:12])

	// 2. Parity against the in-process switch (`BASHY_CMD_DISPATCH=inprocess`).
	inprocOut, inprocErr, inprocCode, _ := run([]string{"BASHY_CMD_DISPATCH=inprocess"}, sampleJSONL, "stats", "summary", "--json")
	if inprocCode != 0 || inprocOut != slotOut {
		t.Fatalf("parity mismatch vs inprocess:\ncode=%d stderr=%s\nslot:\n%s\ninproc:\n%s", inprocCode, inprocErr, slotOut, inprocOut)
	}

	// Help parity (classic + skill) and non-zero exit code parity.
	helpSlot, _, helpCode, _ := run([]string{"BASHY_HELP_FORMAT=classic"}, "", "stats", "--help")
	helpInproc, _, helpInprocCode, _ := run([]string{"BASHY_HELP_FORMAT=classic", "BASHY_CMD_DISPATCH=inprocess"}, "", "stats", "--help")
	if helpCode != 0 || helpInprocCode != 0 || helpSlot != helpInproc {
		t.Fatalf("help parity mismatch: codes=(%d,%d)\nslot:\n%s\ninproc:\n%s", helpCode, helpInprocCode, helpSlot, helpInproc)
	}
	_, _, emptyCode, _ := run(nil, "", "stats", "summary")
	_, _, emptyInprocCode, _ := run([]string{"BASHY_CMD_DISPATCH=inprocess"}, "", "stats", "summary")
	if emptyCode == 0 || emptyCode != emptyInprocCode {
		t.Fatalf("error exit code parity mismatch: slot=%d inproc=%d", emptyCode, emptyInprocCode)
	}

	// 3. Bare-name shell shim (`bashy -c 'stats summary --json'`) uses the active slot.
	shimOut, shimErr, shimCode, warmDur := run(nil, sampleJSONL, "-c", "stats summary --json")
	if shimCode != 0 || shimOut != slotOut {
		t.Fatalf("shell shim mismatch: code=%d stderr=%s\ngot:\n%s\nwant:\n%s", shimCode, shimErr, shimOut, slotOut)
	}
	t.Logf("pilot warm slot dispatch (via shell shim): %s", warmDur.Round(time.Millisecond))

	// 4. Measure full edit -> targeted test -> generate -> build -> Stage -> Activate -> verified active slot cycle.
	editedYoke := filepath.Join(t.TempDir(), "yoke-edited")
	copyYokeStatsSubset(t, filepath.Join(umbrellaRoot, "yoke"), editedYoke)

	cycleStart := time.Now()
	// 4a. Edit pkg/stats/cmd.go in the isolated copy.
	cmdFile := filepath.Join(editedYoke, "pkg", "stats", "cmd.go")
	origSrc, err := os.ReadFile(cmdFile)
	if err != nil {
		t.Fatal(err)
	}
	const oldMarker = `"schema_version": "bashy-stats-summary-v1", "arms": rows`
	const newMarker = `"schema_version": "bashy-stats-summary-v1", "pilot_shaped": true, "arms": rows`
	if !strings.Contains(string(origSrc), oldMarker) {
		t.Fatalf("expected marker %q in pkg/stats/cmd.go", oldMarker)
	}
	editedSrc := strings.Replace(string(origSrc), oldMarker, newMarker, 1)
	if err := os.WriteFile(cmdFile, []byte(editedSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4b. Run targeted unit tests on the edited package.
	testStart := time.Now()
	testCmd := exec.Command("go", "test", "./pkg/stats")
	testCmd.Dir = editedYoke
	if out, err := testCmd.CombinedOutput(); err != nil {
		t.Fatalf("edited pkg/stats unit test failed: %v\n%s", err, out)
	}
	testDur := time.Since(testStart)

	// 4c. Generate + tree-hash build + Stage + Activate v2 slot.
	buildStart := time.Now()
	res2, err := cmdgen.BuildAndActivateSlot(context.Background(), entry, cmdgen.BuildOptions{
		Home:         home,
		GoWork:       goWork,
		LocalModules: map[string]string{"github.com/qiangli/yoke": editedYoke},
		Revision:     "pilot-edit-v2",
		Evidence:     "go test ./pkg/stats PASS",
	})
	if err != nil {
		t.Fatalf("BuildAndActivateSlot v2: %v", err)
	}
	buildDur := time.Since(buildStart)

	// 4d. Verify live dispatch through the UNCHANGED bashy binary picks up v2.
	v2Out, v2Err, v2Code, verifyDur := run(nil, sampleJSONL, "stats", "summary", "--json")
	cycleDur := time.Since(cycleStart)
	if v2Code != 0 {
		t.Fatalf("v2 dispatch exit %d\nstdout=%s\nstderr=%s", v2Code, v2Out, v2Err)
	}
	var doc2 map[string]any
	if err := json.Unmarshal([]byte(v2Out), &doc2); err != nil {
		t.Fatalf("decode v2 json: %v\n%s", err, v2Out)
	}
	if doc2["pilot_shaped"] != true {
		t.Fatalf("v2 dispatch did not run activated slot binary: %s", v2Out)
	}
	if res2.Version.Digest == v1.Digest {
		t.Fatal("v2 slot digest identical to v1 despite source edit")
	}
	t.Logf("PILOT CYCLE edit->verified-active-slot: total=%s (%.3f min) [test=%s, gen+build+stage+activate=%s, live-verify=%s]",
		cycleDur.Round(time.Millisecond), cycleDur.Minutes(),
		testDur.Round(time.Millisecond), buildDur.Round(time.Millisecond), verifyDur.Round(time.Millisecond))

	// 5. Undo and Restore("release") recover v1 immediately with zero rebuild.
	if _, err := store.Undo(); err != nil {
		t.Fatalf("slot Undo: %v", err)
	}
	undoOut, _, undoCode, _ := run(nil, sampleJSONL, "stats", "summary", "--json")
	if undoCode != 0 || undoOut != slotOut {
		t.Fatalf("after Undo, expected v1 output:\ngot:\n%s\nwant:\n%s", undoOut, slotOut)
	}
	if _, err := store.Undo(); err != nil {
		t.Fatalf("slot second Undo (back to v2): %v", err)
	}
	if _, err := store.Restore("release"); err != nil {
		t.Fatalf("slot Restore(release): %v", err)
	}
	relOut, _, relCode, _ := run(nil, sampleJSONL, "stats", "summary", "--json")
	if relCode != 0 || relOut != slotOut {
		t.Fatalf("after Restore(release), expected v1 output:\ngot:\n%s\nwant:\n%s", relOut, slotOut)
	}
}

func pilotFindBashyRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	d := wd
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(d, "cmd", "bashy")); err == nil {
				return d
			}
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	t.Fatalf("could not locate bashy module root from %s", wd)
	return ""
}

func copyYokeStatsSubset(t *testing.T, srcYoke, dstYoke string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dstYoke, "pkg", "stats"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Symlink top-level entries except pkg/ (so nested replace targets like
	// ./external/* resolve without copying the whole repo), and symlink pkg/*
	// entries except pkg/stats (which we copy so the test can edit it safely).
	topEntries, err := os.ReadDir(srcYoke)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range topEntries {
		if e.Name() == "pkg" || e.Name() == ".git" {
			continue
		}
		if err := os.Symlink(filepath.Join(srcYoke, e.Name()), filepath.Join(dstYoke, e.Name())); err != nil {
			t.Fatal(err)
		}
	}
	pkgEntries, err := os.ReadDir(filepath.Join(srcYoke, "pkg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range pkgEntries {
		if e.Name() == "stats" {
			continue
		}
		if err := os.Symlink(filepath.Join(srcYoke, "pkg", e.Name()), filepath.Join(dstYoke, "pkg", e.Name())); err != nil {
			t.Fatal(err)
		}
	}
	statsEntries, err := os.ReadDir(filepath.Join(srcYoke, "pkg", "stats"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range statsEntries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(srcYoke, "pkg", "stats", e.Name())
		dst := filepath.Join(dstYoke, "pkg", "stats", e.Name())
		in, err := os.Open(src)
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.Create(dst)
		if err != nil {
			_ = in.Close()
			t.Fatal(err)
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = in.Close()
			_ = out.Close()
			t.Fatal(err)
		}
		_ = in.Close()
		if err := out.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
