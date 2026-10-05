package agentos

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/polyglot"
)

// The table is the whole answer: a name with no row is refused with the
// names that have one, never silently handed to PATH.
func TestIslandToolResolverRefusesUnknownNames(t *testing.T) {
	_, _, err := islandToolResolver("gfortran")
	if err == nil || !strings.Contains(err.Error(), "provisions no toolchain named \"gfortran\"") || !strings.Contains(err.Error(), "python3") {
		t.Fatalf("err = %v", err)
	}
	for _, name := range islandToolchainNames() {
		if _, ok := islandToolchains[name]; !ok {
			t.Fatalf("%s is advertised but has no row", name)
		}
	}
}

// Under the certification profile the engine keeps its PATH lookup: the
// resolver is never installed.
func TestInstallIslandToolResolverOffUnderCert(t *testing.T) {
	polyglot.ToolResolver = nil
	t.Cleanup(func() { polyglot.ToolResolver = nil })
	t.Setenv("VSC_PROFILE", "cert")
	installIslandToolResolver()
	if polyglot.ToolResolver != nil {
		t.Fatal("resolver installed under VSC_PROFILE=cert")
	}
	t.Setenv("VSC_PROFILE", "")
	installIslandToolResolver()
	if polyglot.ToolResolver == nil {
		t.Fatal("resolver not installed")
	}
}

func TestRustProvisionerHomesNormalizeWindowsCachePath(t *testing.T) {
	cargo, rustup := rustProvisionerHomesMode(`/c/Users/runneradmin/AppData/Local/bashy/bin`, true)
	if cargo != `C:\Users\runneradmin\AppData\Local\bashy\rust\cargo` {
		t.Fatalf("cargo home = %q", cargo)
	}
	if rustup != `C:\Users\runneradmin\AppData\Local\bashy\rust\rustup` {
		t.Fatalf("rustup home = %q", rustup)
	}
	if strings.Contains(cargo, "/c/") || strings.Contains(cargo, "/") {
		t.Fatalf("cargo home retained an MSYS path spelling: %q", cargo)
	}
}

// --prepare reads the fences a script opens and maps each to its rows.
func TestFenceLanguagesAndTools(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "p.bsh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env -S bashy --bashsharp\n~~~py as py\ndef f(): pass\n~~~\n~~~ts as ts\nexport const x = 1\n~~~\n~~~cxx as cxx\nint f() { return 1; }\n~~~\n~~~bash\necho hi\n~~~\necho done\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	langs, err := fenceLanguages(script)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(langs, ",") != "python,typescript,cpp" {
		t.Fatalf("languages = %q", langs)
	}
	if got := strings.Join(islandToolsFor("typescript"), ","); got != "node,typescript" {
		t.Fatalf("typescript rows = %q", got)
	}
	if langs, _ := fenceLanguages(filepath.Join(dir, "x.go")); strings.Join(langs, ",") != "go" {
		t.Fatalf("a .go program is a Go island, got %q", langs)
	}
}

func TestPowerShellAndCSharpShareManagedRuntime(t *testing.T) {
	for _, language := range []string{"powershell", "csharp"} {
		if got := strings.Join(islandToolsFor(language), ","); got != "pwsh" {
			t.Errorf("%s tools = %q, want pwsh", language, got)
		}
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "windows-user.bsh")
	if err := os.WriteFile(script, []byte("~~~powershell as ps\nfunction Get-X {}\n~~~\n~~~csharp as cs\nclass X {}\n~~~\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	langs, err := fenceLanguages(script)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(langs, ","); got != "powershell,csharp" {
		t.Fatalf("languages = %q, want powershell,csharp", got)
	}
}

func TestCheckPrepareDeduplicatesSharedPwshRuntime(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "both.bsh")
	if err := os.WriteFile(script, []byte("~~~powershell as ps\nfunction Get-X {}\n~~~\n~~~csharp as cs\nclass X {}\n~~~\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := islandToolchains["pwsh"]
	t.Cleanup(func() { islandToolchains["pwsh"] = original })
	calls := 0
	islandToolchains["pwsh"] = func(context.Context) ([]string, string, error) {
		calls++
		return []string{"/managed/pwsh", "-NoProfile"}, "shared runtime", nil
	}
	var stdout, stderr bytes.Buffer
	if rc := checkPrepare([]string{script}, &stdout, &stderr); rc != 0 {
		t.Fatalf("checkPrepare rc=%d stderr=%q", rc, stderr.String())
	}
	if calls != 1 {
		t.Fatalf("shared pwsh provisioned %d times, want once", calls)
	}
	if got := stdout.String(); !strings.Contains(got, "pwsh") || !strings.Contains(got, "shared runtime") {
		t.Fatalf("prepare output = %q", got)
	}
}
