package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckBashPPNullSafetyOracle(t *testing.T) {
	tests := []struct {
		name   string
		source string
		rc     int
		stderr string
	}{
		{
			name: "flow-narrow",
			source: `func deref(p *int) int {
    if p == nil {
        return 0
    }
    return *p
}
`,
		},
		{
			name: "false-positive-guards",
			source: `func positive(p *int) bool {
    return p != nil && *p > 0
}
func first(xs []string) string {
    if xs == nil || len(xs) == 0 {
        return ""
    }
    return xs[0]
}
`,
		},
		{
			name: "reassign-after-narrow",
			source: `func deref(p *int, replacement *int) int {
    if p == nil { return 0 }
    p = replacement
    return *p
}
`,
			rc: 2, stderr: "BASHPP-ENULL-DEREF: p may be nil after reassignment\n",
		},
		{
			name:   "unsafe-deref",
			source: "func deref(p *int) int {\n    return *p\n}\n",
			rc:     2, stderr: "BASHPP-ENULL-DEREF: p may be nil when dereferenced\n",
		},
		{
			name:   "unsafe-index",
			source: "func first(xs []string) string {\n    return xs[0]\n}\n",
			rc:     2, stderr: "BASHPP-ENULL-INDEX: xs may be nil when indexed\n",
		},
		{
			name:   "unsafe-call",
			source: "func invoke(fn func() int) int {\n    return fn()\n}\n",
			rc:     2, stderr: "BASHPP-ENULL-CALL: fn may be nil when called\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name+".bpp")
			if err := os.WriteFile(path, []byte(test.source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if got := dispatchCheckTo([]string{"--bashpp", path}, &stdout, &stderr); got != test.rc {
				t.Fatalf("exit = %d, want %d; stdout=%q stderr=%q", got, test.rc, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if got := stderr.String(); got != test.stderr {
				t.Fatalf("stderr = %q, want %q", got, test.stderr)
			}

			stdout.Reset()
			stderr.Reset()
			dispatchCheckTo([]string{path}, &stdout, &stderr)
			if strings.Contains(stderr.String(), "BASHPP-ENULL-") || strings.Contains(stdout.String(), "BASHPP-ENULL-") {
				t.Fatalf("Bash# diagnostic leaked with selector off: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestCheckBashPPHelpAndReportMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if rc := dispatchCheckTo([]string{"--help"}, &stdout, &stderr); rc != 0 {
		t.Fatalf("help exit = %d", rc)
	}
	if !strings.Contains(stdout.String(), "--bashpp") || !strings.Contains(stdout.String(), "null safety") {
		t.Fatalf("help omits Bash# checker contract:\n%s", stdout.String())
	}
	if record := verbAtlasRecord("check", false); !strings.Contains(record.Synopsis, "--bashpp null safety") {
		t.Fatalf("check atlas synopsis omits Bash# null safety: %#v", record)
	}

	path := filepath.Join(t.TempDir(), "unsafe.bpp")
	if err := os.WriteFile(path, []byte("func deref(p *int) int { return *p }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := newCheckAnalyzer(checkOptions{mode: "bashy", bashpp: true, maxDepth: 8}).run([]string{path})
	if report.Mode != "bashy+bashpp" || report.Summary.Errors != 1 || len(report.Diagnostics) != 1 {
		t.Fatalf("unexpected Bash++ report: mode=%q summary=%#v diagnostics=%#v", report.Mode, report.Summary, report.Diagnostics)
	}
	if report.Diagnostics[0].Code != "BASHPP-ENULL-DEREF" {
		t.Fatalf("diagnostic = %#v", report.Diagnostics[0])
	}
}

func TestCheckRecursiveInventory(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.sh")
	lib := filepath.Join(dir, "lib.sh")
	nested := filepath.Join(dir, "nested.sh")
	hostName := "hostcmd"
	if runtime.GOOS == "windows" {
		hostName += ".cmd"
	}
	bin := filepath.Join(dir, hostName)

	write := func(path, body string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(bin, "#!/bin/sh\nexit 0\n", 0o755)
	write(nested, "#!/usr/bin/env bash\necho nested\nmissing_tool_xyz\n", 0o755)
	write(lib, "cp a b\nhostcmd --flag\n./nested.sh\n", 0o644)
	write(main, "echo main\n. ./lib.sh\n", 0o644)

	t.Setenv("PATH", dir)
	report := newCheckAnalyzer(checkOptions{mode: "bashy", maxDepth: 8}).run([]string{main})
	if report.Summary.FilesAnalyzed != 3 {
		t.Fatalf("files analyzed = %d, want 3; files=%#v diagnostics=%#v", report.Summary.FilesAnalyzed, report.Files, report.Diagnostics)
	}
	if report.Summary.NotFound != 1 {
		t.Fatalf("not found = %d, want 1; inventory=%#v", report.Summary.NotFound, report.Inventory)
	}
	if !invHas(report.Inventory.BashyNative, "echo", "") || !invHas(report.Inventory.BashyNative, "cp", "") {
		t.Fatalf("missing bashy native commands: %#v", report.Inventory.BashyNative)
	}
	if !invHas(report.Inventory.System, "hostcmd", bin) {
		t.Fatalf("system inventory missing hostcmd full path: %#v", report.Inventory.System)
	}
	if !invHas(report.Inventory.NotFound, "missing_tool_xyz", "") {
		t.Fatalf("not_found inventory missing command: %#v", report.Inventory.NotFound)
	}
	if !invHas(report.Inventory.Scripts, "./nested.sh", nested) {
		t.Fatalf("scripts inventory missing nested script: %#v", report.Inventory.Scripts)
	}
}

func TestCheckStrictSystemTurnsSystemResolutionIntoError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH executable suffix behavior is tested through integration on Windows")
	}
	dir := t.TempDir()
	host := filepath.Join(dir, "hostcmd")
	if err := os.WriteFile(host, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte("hostcmd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	report := newCheckAnalyzer(checkOptions{mode: "bashy", strictSystem: true, maxDepth: 8}).run([]string{script})
	if report.Summary.Errors != 1 {
		t.Fatalf("errors = %d, want 1; diagnostics=%#v", report.Summary.Errors, report.Diagnostics)
	}
	if got := report.Diagnostics[0].Code; got != "BASHY0301" {
		t.Fatalf("diagnostic code = %s, want BASHY0301", got)
	}
}

func TestCheckAllowContainerClassifiesMissingGNUCoreutil(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte("coreutils --version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	report := newCheckAnalyzer(checkOptions{mode: "bashy", allowContainer: true, maxDepth: 8}).run([]string{script})
	if report.Summary.Container != 1 || !invHas(report.Inventory.Container, "coreutils", "") {
		t.Fatalf("coreutils should be container-resolvable: summary=%#v inventory=%#v", report.Summary, report.Inventory)
	}
	if report.Summary.NotFound != 0 {
		t.Fatalf("not_found = %d, want 0", report.Summary.NotFound)
	}
}

func TestCheckDynamicCommandJSON(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte("$tool --version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := newCheckAnalyzer(checkOptions{mode: "bashy", maxDepth: 8}).run([]string{script})
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded checkReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, data)
	}
	if decoded.SchemaVersion != checkSchemaVersion {
		t.Fatalf("schema = %q", decoded.SchemaVersion)
	}
	if decoded.Summary.Dynamic != 1 || len(decoded.Inventory.Dynamic) != 1 {
		t.Fatalf("dynamic summary/inventory mismatch: %#v %#v", decoded.Summary, decoded.Inventory)
	}
}

func TestCheckSyntaxError(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bad.sh")
	if err := os.WriteFile(script, []byte("if true; then echo ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := newCheckAnalyzer(checkOptions{mode: "bashy", maxDepth: 8}).run([]string{script})
	if report.Summary.Errors == 0 {
		t.Fatalf("expected syntax error, diagnostics=%#v", report.Diagnostics)
	}
	if !strings.Contains(report.Diagnostics[0].Code, "BASHY0001") {
		t.Fatalf("unexpected diagnostics: %#v", report.Diagnostics)
	}
}

func TestCheckPosixModeRejectsBashSyntax(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bashism.sh")
	if err := os.WriteFile(script, []byte("values=(one two)\n[[ -n ${values[0]} ]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	posix := newCheckAnalyzer(checkOptions{mode: "posix", maxDepth: 8}).run([]string{script})
	if posix.Summary.Errors == 0 {
		t.Fatalf("POSIX mode accepted Bash-only syntax: %#v", posix)
	}
	bash := newCheckAnalyzer(checkOptions{mode: "bash53", maxDepth: 8}).run([]string{script})
	if bash.Summary.Errors != 0 {
		t.Fatalf("bash53 mode rejected valid Bash syntax: %#v", bash.Diagnostics)
	}
}

func TestCheckAgentModeSetsJSONFriendlyMode(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte("echo ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := newCheckAnalyzer(checkOptions{mode: "bashy", agent: true, maxDepth: 8}).run([]string{script})
	if report.Mode != "bashy+agent" {
		t.Fatalf("mode = %q, want bashy+agent", report.Mode)
	}
	if report.Summary.Errors != 0 || report.Summary.BashyNative == 0 {
		t.Fatalf("unexpected report: %#v", report.Summary)
	}
}

func invHas(items []checkInvItem, name, path string) bool {
	for _, item := range items {
		if item.Name == name && (path == "" || item.Path == path) {
			return true
		}
	}
	return false
}
