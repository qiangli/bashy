// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
}
