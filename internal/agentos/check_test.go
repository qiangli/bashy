package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckRecursiveInventory(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.sh")
	lib := filepath.Join(dir, "lib.sh")
	nested := filepath.Join(dir, "nested.sh")
	bin := filepath.Join(dir, "hostcmd")

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
	if err := os.WriteFile(script, []byte("timeout 1 echo ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	report := newCheckAnalyzer(checkOptions{mode: "bashy", allowContainer: true, maxDepth: 8}).run([]string{script})
	if report.Summary.Container != 1 || !invHas(report.Inventory.Container, "timeout", "") {
		t.Fatalf("timeout should be container-resolvable: summary=%#v inventory=%#v", report.Summary, report.Inventory)
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

func invHas(items []checkInvItem, name, path string) bool {
	for _, item := range items {
		if item.Name == name && (path == "" || item.Path == path) {
			return true
		}
	}
	return false
}
