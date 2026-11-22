// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/bashy/internal/agentos/adapterembed"
	"github.com/qiangli/yoke/pkg/shape/slot"
	"github.com/qiangli/yoke/pkg/src"
)

func TestResolveAdapterScriptFindsPilotStats(t *testing.T) {
	t.Setenv(adaptersRootEnv, "")
	got := resolveAdapterScript("yoke", "stats")
	if got == "" {
		t.Fatal("resolveAdapterScript(yoke, stats) returned empty path")
	}
	if !strings.HasSuffix(filepath.ToSlash(got), "/yoke/stats/stats.bsh") {
		t.Fatalf("resolveAdapterScript(yoke, stats) = %q, want .../yoke/stats/stats.bsh", got)
	}
}

func TestResolveAdapterScriptHonorsOverride(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "yoke", "stats", "stats.bsh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env -S bashy --bashsharp\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(adaptersRootEnv, tmp)
	if got := resolveAdapterScript("yoke", "stats"); got != script {
		t.Fatalf("resolveAdapterScript with override = %q, want %q", got, script)
	}
	if got := resolveAdapterScript("yoke", "missing"); got != "" {
		t.Fatalf("resolveAdapterScript for missing entry = %q, want empty", got)
	}
}

func TestAdapterScriptPathNormalizesWindowsSeparators(t *testing.T) {
	const in = `C:\Users\runner\AppData\Local\bashy\yoke\sprint\sprint.bsh`
	const want = `C:/Users/runner/AppData/Local/bashy/yoke/sprint/sprint.bsh`
	if got := adapterScriptPathForOS(in, "windows"); got != want {
		t.Fatalf("adapterScriptPathForOS(%q, windows) = %q, want %q", in, got, want)
	}
	if got := adapterScriptPathForOS(in, "linux"); got != in {
		t.Fatalf("non-Windows adapter path changed: %q", got)
	}
}

func TestResolveGoAdapterFetchesPinnedSourceBeforeEmbeddedFallback(t *testing.T) {
	origRoots := adapterCandidateRoots
	origFetch := adapterSourceFetch
	adapterCandidateRoots = func() []string { return nil }
	t.Cleanup(func() {
		adapterCandidateRoots = origRoots
		adapterSourceFetch = origFetch
	})

	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv(adaptersRootEnv, "")
	fetches := 0
	adapterSourceFetch = func(gotHome string) error {
		if gotHome != home {
			t.Fatalf("fetch home = %q, want %q", gotHome, home)
		}
		fetches++
		dir := filepath.Join(src.SrcDir(home), "bashy", "yoke", "sprint")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "sprint.bsh"), []byte("#!/usr/bin/env bashy\n"), 0o755)
	}

	want := filepath.Join(src.SrcDir(home), "bashy", "yoke", "sprint", "sprint.bsh")
	if got := resolveAdapterScript("yoke", "sprint"); got != adapterScriptPath(want) {
		t.Fatalf("resolveAdapterScript after source fetch = %q, want %q", got, adapterScriptPath(want))
	}
	if fetches != 1 {
		t.Fatalf("Go-backed embedded adapter source fetches = %d, want 1", fetches)
	}
	if got := resolveAdapterScript("examples", "notes"); got == "" {
		t.Fatal("shell-only embedded adapter did not resolve")
	}
	if fetches != 1 {
		t.Fatalf("shell-only embedded adapter triggered source fetch; total = %d", fetches)
	}
}

func captureCommandAdapterRun(t *testing.T, folder, entry, invokedAs string, args []string) (string, string, int) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW

	var outBuf, errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&outBuf, outR)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&errBuf, errR)
	}()

	code := runCommandAdapter(folder, entry, invokedAs, args)

	os.Stdout, os.Stderr = origOut, origErr
	_ = outW.Close()
	_ = errW.Close()
	wg.Wait()
	_ = outR.Close()
	_ = errR.Close()
	return outBuf.String(), errBuf.String(), code
}

func TestEmbeddedAdaptersMatchSourceTree(t *testing.T) {
	root, ok := findBashySourceRoot(".")
	if !ok {
		t.Fatal("cannot locate bashy source root")
	}

	embedded := map[string][]byte{}
	err := fs.WalkDir(adapterembed.FS, ".", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".bsh") {
			t.Errorf("non-.bsh file embedded in adapterembed.FS: %s (only .bsh adapters may be embedded)", p)
			return nil
		}
		raw, err := fs.ReadFile(adapterembed.FS, p)
		if err != nil {
			return err
		}
		embedded[p] = raw
		return nil
	})
	if err != nil {
		t.Fatalf("walk adapterembed.FS: %v", err)
	}
	if len(embedded) == 0 {
		t.Fatal("adapterembed.FS contains zero embedded .bsh files")
	}

	onDisk := map[string][]byte{}
	for _, folder := range adapterembed.Folders {
		srcDir := filepath.Join(root, folder)
		err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			if !adapterembed.ShouldEmbed(relSlash) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			onDisk[relSlash] = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", srcDir, err)
		}
	}

	for p, want := range onDisk {
		got, exists := embedded[p]
		if !exists {
			t.Errorf("missing embedded adapter %s (run `go generate ./internal/agentos/adapterembed`)", p)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("embedded adapter %s drifted from source tree (run `go generate ./internal/agentos/adapterembed`)", p)
		}
	}
	for p := range embedded {
		if _, exists := onDisk[p]; !exists {
			t.Errorf("stale embedded adapter %s no longer in source tree (run `go generate ./internal/agentos/adapterembed`)", p)
		}
	}

	// Every adaptedVerbs and customAgentCLIEntries route must be backed by an embedded .bsh.
	for verb, fe := range adaptedVerbs {
		p := fe[0] + "/" + fe[1] + "/" + fe[1] + ".bsh"
		if _, exists := embedded[p]; !exists {
			t.Errorf("adaptedVerbs[%q] -> %s is missing from adapterembed.FS", verb, p)
		}
	}
	for tool := range customAgentCLIEntries {
		p := "custom/" + tool + "/" + tool + ".bsh"
		if _, exists := embedded[p]; !exists {
			t.Errorf("customAgentCLIEntries[%q] -> %s is missing from adapterembed.FS", tool, p)
		}
	}
	if _, exists := embedded["custom/agent-cli/agent-cli.bsh"]; !exists {
		t.Error("custom/agent-cli/agent-cli.bsh is missing from adapterembed.FS")
	}
}

func TestResolveAndRunEmbeddedAdapterEmptyHomeAndCwd(t *testing.T) {
	origRoots := adapterCandidateRoots
	origFetch := adapterSourceFetch
	adapterCandidateRoots = func() []string { return nil }
	adapterSourceFetch = func(string) error { return errors.New("offline embedded-fallback fixture") }
	t.Cleanup(func() {
		adapterCandidateRoots = origRoots
		adapterSourceFetch = origFetch
	})

	t.Run("empty-dirs", func(t *testing.T) {
		emptyHome := t.TempDir()
		emptyCwd := t.TempDir()
		adapterCache := filepath.Join(t.TempDir(), "adapter-cache")

		t.Setenv("HOME", emptyHome)
		t.Setenv("USERPROFILE", emptyHome)
		t.Setenv("BASHY_HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv(adaptersRootEnv, "")
		t.Setenv("BASHY_ADAPTER_CACHE", adapterCache)
		t.Setenv("GOWORK", "off")
		t.Setenv("BASHY_HINTS", "off")
		t.Chdir(emptyCwd)
		t.Setenv("PWD", emptyCwd)

		// Verify no source pin exists under HOME.
		if _, err := os.Stat(filepath.Join(emptyHome, ".bashy", "src", "bashy")); !os.IsNotExist(err) {
			t.Fatalf("expected no source pin under empty HOME, stat err = %v", err)
		}

		// Resolve across all 5 embedded folders and confirm content-addressed caching.
		for _, tc := range [][2]string{
			{"yoke", "dag"},
			{"optional", "jq"},
			{"toolchain", "cmake"},
			{"custom", "gh"},
			{"examples", "desktop"},
			{"examples", "desk/notepad"},
		} {
			got1 := resolveAdapterScript(tc[0], tc[1])
			if got1 == "" {
				t.Fatalf("resolveAdapterScript(%q, %q) returned empty path", tc[0], tc[1])
			}
			if !isRegularFile(got1) {
				t.Fatalf("resolveAdapterScript(%q, %q) = %q is not a regular file", tc[0], tc[1], got1)
			}
			wantPrefix := filepath.ToSlash(filepath.Join(adapterCache, ".embedded")) + "/"
			if !strings.HasPrefix(filepath.ToSlash(got1), wantPrefix) {
				t.Fatalf("resolveAdapterScript(%q, %q) = %q, want under %q", tc[0], tc[1], got1, wantPrefix)
			}
			base := filepath.Base(filepath.FromSlash(tc[1]))
			wantSuffix := "/" + tc[0] + "/" + tc[1] + "/" + base + ".bsh"
			if !strings.HasSuffix(filepath.ToSlash(got1), wantSuffix) {
				t.Fatalf("resolveAdapterScript(%q, %q) = %q, want suffix %q", tc[0], tc[1], got1, wantSuffix)
			}
			got2 := resolveAdapterScript(tc[0], tc[1])
			if got2 != got1 {
				t.Fatalf("content-addressed path changed across calls: %q vs %q", got1, got2)
			}
		}

		if got := resolveAdapterScript("yoke", "definitely-missing"); got != "" {
			t.Fatalf("resolveAdapterScript for missing entry = %q, want empty", got)
		}

		// Run a pure Bash# embedded adapter (examples/notes/notes.bsh, which also
		// sources ../desk/kit/outpost.bsh) with empty HOME/cwd and no source pin.
		notesOut, notesErr, notesCode := captureCommandAdapterRun(t, "examples", "notes", "notes", []string{"status"})
		if notesCode != 0 || !strings.Contains(notesOut, "notes: not installed") {
			t.Fatalf("embedded examples/notes status: code=%d stderr=%q stdout=%q", notesCode, notesErr, notesOut)
		}

		// Run the Go-backed yoke/dag adapter resolved from the embedded .bsh
		// (matching test-bash-container-mode in the official container: cwd outside
		// repo, no source pin, cached binary present in BASHY_ADAPTER_CACHE).
		selfExe, err := os.Executable()
		if err != nil {
			t.Fatalf("os.Executable: %v", err)
		}
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		cachedDag := filepath.Join(adapterCache, "yoke", "dag"+ext)
		if err := os.MkdirAll(filepath.Dir(cachedDag), 0o755); err != nil {
			t.Fatal(err)
		}
		rawExe, err := os.ReadFile(selfExe)
		if err != nil {
			t.Fatalf("read test executable: %v", err)
		}
		if err := os.WriteFile(cachedDag, rawExe, 0o755); err != nil {
			t.Fatalf("stage cached dag binary: %v", err)
		}

		dagOut, dagErr, dagCode := captureCommandAdapterRun(t, "yoke", "dag", "dag", []string{"--embedded-probe", "ok"})
		if dagCode != 0 || strings.TrimSpace(dagOut) != "--embedded-probe ok" {
			t.Fatalf("embedded yoke/dag execution: code=%d stderr=%q stdout=%q", dagCode, dagErr, dagOut)
		}
	})

	t.Run("unset-home-env", func(t *testing.T) {
		emptyCwd := t.TempDir()
		tmpDir := t.TempDir()

		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("BASHY_HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("BASHY_ADAPTER_CACHE", "")
		t.Setenv(adaptersRootEnv, "")
		t.Setenv("TMPDIR", tmpDir)
		t.Setenv("TMP", tmpDir)
		t.Setenv("TEMP", tmpDir)
		t.Setenv("GOWORK", "off")
		t.Setenv("BASHY_HINTS", "off")
		t.Chdir(emptyCwd)
		t.Setenv("PWD", emptyCwd)

		got := resolveAdapterScript("examples", "collab-save")
		if got == "" || !isRegularFile(got) {
			t.Fatalf("resolveAdapterScript with empty HOME env = %q, want materialized file", got)
		}
		out, errStr, code := captureCommandAdapterRun(t, "examples", "collab-save", "collab-save", nil)
		if code != 2 || !strings.Contains(errStr, "collab-save save PATH...") {
			t.Fatalf("embedded examples/collab-save with empty HOME env: code=%d stderr=%q stdout=%q", code, errStr, out)
		}
	})
}

func TestResolveAdapterScriptPrecedence(t *testing.T) {
	origRoots := adapterCandidateRoots
	origFetch := adapterSourceFetch
	origSelect := shapeSlotSelect
	origLaunch := shapeSlotLaunch
	t.Cleanup(func() {
		adapterCandidateRoots = origRoots
		adapterSourceFetch = origFetch
		shapeSlotSelect = origSelect
		shapeSlotLaunch = origLaunch
	})

	emptyHome := t.TempDir()
	emptyCwd := t.TempDir()
	bashyHome := filepath.Join(emptyHome, ".bashy")

	t.Setenv("HOME", emptyHome)
	t.Setenv("USERPROFILE", emptyHome)
	t.Setenv("BASHY_HOME", bashyHome)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("BASHY_ADAPTER_CACHE", "")
	t.Setenv(adaptersRootEnv, "")
	t.Setenv("GOWORK", "off")
	t.Setenv("BASHY_HINTS", "off")
	t.Chdir(emptyCwd)
	t.Setenv("PWD", emptyCwd)

	adapterCandidateRoots = func() []string { return nil }
	adapterSourceFetch = func(string) error { return errors.New("offline precedence fixture") }
	shapeSlotSelect = func(string) (slot.Version, bool, error) { return slot.Version{}, false, nil }

	writeMarkerAdapter := func(root, label string) string {
		p := filepath.Join(root, "yoke", "dag", "dag.bsh")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "#!/usr/bin/env -S bashy --bashsharp\nprintf 'winner=" + label + ":%s\\n' \"$*\"\n"
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// 1. Embedded fallback when nothing exists on disk.
	embeddedPath := resolveAdapterScript("yoke", "dag")
	if embeddedPath == "" || !strings.Contains(filepath.ToSlash(embeddedPath), "/embedded-adapters/") {
		t.Fatalf("step 1 (embedded fallback): got %q, want embedded-adapters cache path", embeddedPath)
	}
	// Companion helper materialization check for examples/notes -> examples/desk/kit/outpost.bsh.
	notesPath := resolveAdapterScript("examples", "notes")
	if notesPath == "" {
		t.Fatal("resolveAdapterScript(examples, notes) returned empty path")
	}
	companionPath := filepath.Join(filepath.Dir(notesPath), "..", "desk", "kit", "outpost.bsh")
	if !isRegularFile(companionPath) {
		t.Fatalf("expected companion helper materialized at %q alongside %q", companionPath, notesPath)
	}

	// 2. On-disk candidate root beats embedded.
	diskRoot := t.TempDir()
	diskScript := writeMarkerAdapter(diskRoot, "disk")
	adapterCandidateRoots = func() []string { return []string{diskRoot} }
	if got := resolveAdapterScript("yoke", "dag"); got != diskScript {
		t.Fatalf("step 2 (on-disk > embedded): got %q, want %q", got, diskScript)
	}
	out, errStr, code := captureCommandAdapterRun(t, "yoke", "dag", "dag", []string{"arg1"})
	if code != 0 || strings.TrimSpace(out) != "winner=disk:arg1" {
		t.Fatalf("step 2 run: code=%d stderr=%q stdout=%q", code, errStr, out)
	}
	// Other verbs absent from diskRoot still fall back to embedded.
	if got := resolveAdapterScript("optional", "jq"); !strings.Contains(filepath.ToSlash(got), "/embedded-adapters/") {
		t.Fatalf("step 2 fallback for optional/jq: got %q, want embedded-adapters path", got)
	}

	// 3. Source pin (~/.bashy/src/bashy) beats on-disk root and embedded.
	pinRoot := filepath.Join(bashyHome, "src", "bashy")
	pinScript := writeMarkerAdapter(pinRoot, "source-pin")
	if got := resolveAdapterScript("yoke", "dag"); got != pinScript {
		t.Fatalf("step 3 (source pin > on-disk > embedded): got %q, want %q", got, pinScript)
	}
	out, errStr, code = captureCommandAdapterRun(t, "yoke", "dag", "dag", []string{"arg2"})
	if code != 0 || strings.TrimSpace(out) != "winner=source-pin:arg2" {
		t.Fatalf("step 3 run: code=%d stderr=%q stdout=%q", code, errStr, out)
	}

	// 4. BASHY_ADAPTERS_ROOT override beats source pin, on-disk root, and embedded.
	overrideRoot := t.TempDir()
	overrideScript := writeMarkerAdapter(overrideRoot, "override")
	t.Setenv(adaptersRootEnv, overrideRoot)
	if got := resolveAdapterScript("yoke", "dag"); got != overrideScript {
		t.Fatalf("step 4 (override > source pin > on-disk > embedded): got %q, want %q", got, overrideScript)
	}
	out, errStr, code = captureCommandAdapterRun(t, "yoke", "dag", "dag", []string{"arg3"})
	if code != 0 || strings.TrimSpace(out) != "winner=override:arg3" {
		t.Fatalf("step 4 run: code=%d stderr=%q stdout=%q", code, errStr, out)
	}
	// Explicit override remains authoritative when an entry is absent from it.
	if got := resolveAdapterScript("optional", "jq"); got != "" {
		t.Fatalf("step 4 missing in override: got %q, want empty", got)
	}

	// 5. Active shape slot beats override, source pin, on-disk root, and embedded.
	slotBin := filepath.Join(t.TempDir(), "dag-slot")
	if err := os.WriteFile(slotBin, []byte("slot"), 0o755); err != nil {
		t.Fatal(err)
	}
	var launchedPath string
	var launchedArgs []string
	shapeSlotSelect = func(name string) (slot.Version, bool, error) {
		if name == "dag" {
			return slot.Version{Path: slotBin}, true, nil
		}
		return slot.Version{}, false, nil
	}
	shapeSlotLaunch = func(path string, args []string, _ []string) int {
		launchedPath = path
		launchedArgs = append([]string(nil), args...)
		return 0
	}
	code = runCommandAdapter("yoke", "dag", "dag", []string{"arg4"})
	if code != 0 || launchedPath != slotBin || strings.Join(launchedArgs, " ") != "dag arg4" {
		t.Fatalf("step 5 (shape slot > all): code=%d path=%q args=%v", code, launchedPath, launchedArgs)
	}
}

func TestStage0StatsAdapterEndToEndParity(t *testing.T) {
	root, ok := findBashySourceRoot(".")
	if !ok {
		t.Fatal("cannot locate bashy source root")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bashy")
	if filepath.Separator == '\\' {
		bin += ".exe"
	}
	buildCmd := exec.Command("go", "build", "-o", bin, "./cmd/bashy")
	buildCmd.Dir = root
	buildCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build cmd/bashy: %v\n%s", err, out)
	}

	cacheDir := filepath.Join(tmp, "adapter-cache")
	run := func(stdin string, args ...string) (string, string, int) {
		c := exec.Command(bin, args...)
		c.Dir = tmp // outside the source tree to prove location independence
		c.Env = append(os.Environ(),
			"BASHY_ADAPTER_CACHE="+cacheDir,
			"BASHY_HINTS=off",
			"BASHY_HOME="+filepath.Join(tmp, "home"),
		)
		c.Stdin = strings.NewReader(stdin)
		var out, errBuf bytes.Buffer
		c.Stdout = &out
		c.Stderr = &errBuf
		err := c.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("exec %v: %v", args, err)
		}
		return out.String(), errBuf.String(), code
	}

	helpOut, helpErr, helpCode := run("", "stats", "--help")
	if helpCode != 0 || !strings.Contains(helpOut, "stats [command]") || helpErr != "" {
		t.Fatalf("stats --help: code=%d stderr=%q stdout=%q", helpCode, helpErr, helpOut)
	}

	subHelpOut, subHelpErr, subHelpCode := run("", "stats", "summary", "--help")
	if subHelpCode != 0 || !strings.Contains(subHelpOut, "stats summary") || subHelpErr != "" {
		t.Fatalf("stats summary --help: code=%d stderr=%q stdout=%q", subHelpCode, subHelpErr, subHelpOut)
	}

	sample := "{\"instance_id\":\"i1\",\"agent\":\"a1\",\"resolved\":true,\"cost_usd\":0.10}\n{\"instance_id\":\"i2\",\"agent\":\"a1\",\"resolved\":false,\"cost_usd\":0.20}\n"
	sumOut, sumErr, sumCode := run(sample, "stats", "summary", "--arm", "agent", "--cost", "cost_usd", "--json")
	if sumCode != 0 || sumErr != "" || !strings.Contains(sumOut, "\"bashy-stats-summary-v1\"") || !strings.Contains(sumOut, "\"a1\"") {
		t.Fatalf("stats summary --json: code=%d stderr=%q stdout=%q", sumCode, sumErr, sumOut)
	}

	expOut, expErr, expCode := run("", "stats", "summary", "--explain")
	if expCode != 0 || expErr != "" || !strings.Contains(expOut, "explain: stats summary (not executed)") {
		t.Fatalf("stats summary --explain: code=%d stderr=%q stdout=%q", expCode, expErr, expOut)
	}

	_, errStderr, errCode := run("", "stats", "summary")
	if errCode != 1 || !strings.Contains(errStderr, "bashy stats:") {
		t.Fatalf("stats summary on empty stdin: code=%d stderr=%q", errCode, errStderr)
	}

	_, fldStderr, fldCode := run("", "stats", "--fields=x", "summary")
	if fldCode != 2 || !strings.Contains(fldStderr, "--fields unsupported for stats summary") {
		t.Fatalf("stats --fields=x summary: code=%d stderr=%q", fldCode, fldStderr)
	}

	jqOut, jqErr, jqCode := run("{\"x\":42}\n", "jq", "-r", ".x")
	if jqCode != 0 || jqErr != "" || strings.TrimSpace(jqOut) != "42" {
		t.Fatalf("jq -r .x: code=%d stderr=%q stdout=%q", jqCode, jqErr, jqOut)
	}

	calOut, calErr, calCode := run("", "ncal", "1", "2026")
	if calCode != 0 || calErr != "" || !strings.Contains(calOut, "2026") {
		t.Fatalf("ncal 1 2026: code=%d stderr=%q stdout=%q", calCode, calErr, calOut)
	}

	b64Out, b64Err, b64Code := run("hello\n", "base64")
	if b64Code != 0 || b64Err != "" || strings.TrimSpace(b64Out) != "aGVsbG8K" {
		t.Fatalf("base64: code=%d stderr=%q stdout=%q", b64Code, b64Err, b64Out)
	}

	dirHelp, dirErr, dirCode := run("", "dir", "--help")
	if dirCode != 0 || dirErr != "" || !strings.Contains(dirHelp, "dir [OPTION]") {
		t.Fatalf("dir --help: code=%d stderr=%q stdout=%q", dirCode, dirErr, dirHelp)
	}

	cmakeOut, cmakeErr, cmakeCode := run("", "cmake", "--explain")
	if cmakeCode != 0 || cmakeErr != "" || !strings.Contains(cmakeOut, "explain: cmake (not executed)") {
		t.Fatalf("cmake --explain: code=%d stderr=%q stdout=%q", cmakeCode, cmakeErr, cmakeOut)
	}

	astOut, astErr, astCode := run("", "ast", "--help")
	if astCode != 0 || astErr != "" || !strings.Contains(astOut, "ast") {
		t.Fatalf("ast --help: code=%d stderr=%q stdout=%q", astCode, astErr, astOut)
	}

	todoOut, todoErr, todoCode := run("", "todo", "--help")
	if todoCode != 0 || todoErr != "" || !strings.Contains(todoOut, "todo") {
		t.Fatalf("todo --help: code=%d stderr=%q stdout=%q", todoCode, todoErr, todoOut)
	}

	dagOut, dagErr, dagCode := run("", "dag", "--help")
	if dagCode != 0 || dagErr != "" || !strings.Contains(dagOut, "dag") {
		t.Fatalf("dag --help: code=%d stderr=%q stdout=%q", dagCode, dagErr, dagOut)
	}

	// custom/ Group 1: binmgr-managed external CLIs
	for _, verb := range []string{"gh", "kubectl", "helm"} {
		out, errStr, code := run("", verb, "--explain")
		if code != 0 || errStr != "" || !strings.Contains(out, "explain: "+verb+" (not executed)") {
			t.Fatalf("%s --explain: code=%d stderr=%q stdout=%q", verb, code, errStr, out)
		}
	}

	// custom/ Group 2: declarative registry CLIs
	for _, verb := range []string{"aws", "gcloud", "doctl"} {
		out, errStr, code := run("", verb, "--explain")
		if code != 0 || errStr != "" || !strings.Contains(out, "explain: "+verb+" (not executed)") {
			t.Fatalf("%s --explain: code=%d stderr=%q stdout=%q", verb, code, errStr, out)
		}
	}

	// custom/ Group 3: lean otel
	otelOut, otelErr, otelCode := run("", "otel", "--help")
	if otelCode != 0 || otelErr != "" || !strings.Contains(otelOut, "serve") {
		t.Fatalf("otel --help: code=%d stderr=%q stdout=%q", otelCode, otelErr, otelOut)
	}

	// custom/ Group 4: steerable agent CLIs
	for _, tool := range []string{"claude", "codex", "agy"} {
		out, errStr, code := run("", tool, "--help")
		if code != 0 || errStr != "" || !strings.Contains(out, "session:  "+tool) {
			t.Fatalf("%s --help: code=%d stderr=%q stdout=%q", tool, code, errStr, out)
		}
	}
}

func TestAppendAdapterEnvOnlyPassesBashySelfToAdapterScript(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"BASHY_SELF=/stale/bashy",
		adapterExecEnv + "=1",
		adapterInprocEnv + "=1",
		adapterSelfEnv + "=/stale/bashy",
		invokedAsEnv + "=old",
	}
	slotEnv := appendAdapterEnv(base, "go", false)
	for _, kv := range slotEnv {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "BASHY_SELF", adapterExecEnv, adapterInprocEnv, adapterSelfEnv:
			t.Fatalf("non-script adapter env unexpectedly retained %q", kv)
		}
	}

	scriptEnv := appendAdapterEnv(base, "go", true)
	sawSelf := false
	sawExec := false
	for _, kv := range scriptEnv {
		switch {
		case strings.HasPrefix(kv, "BASHY_SELF=") && kv != "BASHY_SELF=/stale/bashy":
			sawSelf = true
		case kv == adapterExecEnv+"=1":
			sawExec = true
		case strings.HasPrefix(kv, adapterInprocEnv+"="), strings.HasPrefix(kv, adapterSelfEnv+"="):
			t.Fatalf("script adapter env unexpectedly retained %q", kv)
		}
	}
	if !sawSelf || !sawExec {
		t.Fatalf("script adapter env missing BASHY_SELF or %s: %v", adapterExecEnv, scriptEnv)
	}
}

func TestToolchainGoAdapterStripsBashySelfFromGoTestChild(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go executable unavailable")
	}
	gorootOut, err := exec.Command(goBin, "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	realGoroot := strings.TrimSpace(string(gorootOut))
	if realGoroot == "" {
		realGoroot = runtime.GOROOT()
	}

	binCache := t.TempDir()
	const ver = "99.0.0-e2e"
	stageParent := filepath.Join(binCache, "go", ver)
	if err := os.MkdirAll(stageParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realGoroot, filepath.Join(stageParent, "go")); err != nil {
		t.Skipf("symlink GOROOT unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageParent, ".binmgr-tree-complete"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	modDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module example.com/gotestchild\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testSrc := `package gotestchild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoAdapterEnvLeaksIntoGoTestBinary(t *testing.T) {
	for _, k := range []string{
		"BASHY_SELF",
		"BASHY_ADAPTER_SELF",
		"BASHY_ADAPTER_EXEC",
		"BASHY_ADAPTER_INPROC",
		"BASHY_INVOKED_AS",
	} {
		if v, ok := os.LookupEnv(k); ok {
			t.Fatalf("adapter env %s=%q leaked into go test child", k, v)
		}
	}
	if got := os.Getenv("BASHY_HINTS"); got == "off" {
		t.Fatalf("BASHY_HINTS=off leaked from adapter script into go test child")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSuffix(filepath.Base(exe), ".exe"), ".test") {
		t.Fatalf("expected .test binary, got %q", exe)
	}
}
`
	if err := os.WriteFile(filepath.Join(modDir, "child_test.go"), []byte(testSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BASHY_ADAPTER_CACHE", filepath.Join(t.TempDir(), "adapter-cache"))
	t.Setenv("BASHY_BIN_CACHE", binCache)
	t.Setenv("BASHY_GO_VERSION", ver)
	t.Setenv(adaptersRootEnv, "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("BASHY_SELF", "")
	t.Setenv("BASHY_HINTS", "on")
	t.Chdir(modDir)

	out, errStr, code := captureCommandAdapterRun(t, "toolchain", "go", "go", []string{"test", "-v", "-count=1", "."})
	if code != 0 {
		t.Fatalf("runCommandAdapter(toolchain, go, test): code=%d\nstdout:\n%s\nstderr:\n%s", code, out, errStr)
	}
	if !strings.Contains(out, "TestNoAdapterEnvLeaksIntoGoTestBinary") {
		t.Fatalf("expected child test execution in stdout, got:\n%s", out)
	}
}
