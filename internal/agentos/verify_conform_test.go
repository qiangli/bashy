package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhnt/conform"
	conformcli "github.com/dhnt/conform/cli"
)

func TestConformSuiteSubcommandsShipInProcess(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := runConform([]string{"version"}, &out, &stderr); code != 0 || strings.TrimSpace(out.String()) != conform.Version {
		t.Fatalf("version: exit %d, stdout %q, stderr %q", code, &out, &stderr)
	}

	dir := t.TempDir()
	contract := "### C-01\nEcho a fixed line.\n```console\n$ echo hello\nhello\n```\nexit: 0\n"
	path := filepath.Join(dir, "CONTRACT.md")
	if err := os.WriteFile(path, []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := runConform([]string{"run", path, "--dir", dir}, &out, &stderr); code != 0 || !strings.Contains(out.String(), "PASS C-01") {
		t.Fatalf("run: exit %d, stdout %q, stderr %q", code, &out, &stderr)
	}

	for _, s := range suiteRegistry() {
		for _, name := range append([]string{s.Name}, s.Aliases...) {
			stderr.Reset()
			conformcli.Run([]string{name, "--help"}, &out, &stderr)
			if !strings.Contains(stderr.String(), "unknown command") {
				t.Errorf("battery %q collides with a judge command", name)
			}
		}
	}
}

func TestConformMatchesJudgeCLI(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"run", "--help"}, {"--list=false"}, {"run", "--unknown"}, {"yash"}, {"spec", "check", filepath.Join("..", "..", "examples", "desk", "form")}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr, wantOut, wantErr bytes.Buffer
			want := conformcli.Run(args, &wantOut, &wantErr)
			code := runConform(args, &out, &stderr)
			if code != want || out.String() != wantOut.String() || stderr.String() != wantErr.String() {
				t.Fatalf("got (%d, %q, %q), want (%d, %q, %q)", code, &out, &stderr, want, &wantOut, &wantErr)
			}
			if len(args) > 0 && args[0] == "spec" && code != 0 {
				t.Fatalf("spec check failed: %s%s", &out, &stderr)
			}
		})
	}
}

func TestSelftestMigration(t *testing.T) {
	const notice = "conform: the test batteries moved to \"bashy selftest\"; this forward will be removed\n"
	for _, args := range [][]string{{"--list", "--json"}, {"compat", "--help"}, {"compliance", "--help"}, {"conformance", "--help"}, {"benchmark", "--help"}, {"--list", "--bad-flag"}} {
		var out, stderr, wantOut, wantErr bytes.Buffer
		want := runSelftest(args, &wantOut, &wantErr)
		code := runConform(args, &out, &stderr)
		if code != want || out.String() != wantOut.String() || stderr.String() != notice+wantErr.String() {
			t.Errorf("%v: got (%d, %q, %q), want (%d, %q, %q)", args, code, &out, &stderr, want, &wantOut, notice+wantErr.String())
		}
		if args[0] == "compat" && (code != 0 || !strings.Contains(out.String(), "GNU Bash")) {
			t.Errorf("compat help: exit %d, stdout %q", code, &out)
		}
	}
	for _, name := range []string{"selftest", "verify"} {
		out := string(skillHelpProcess(t, "classic", name, "--help"))
		if !strings.Contains(out, "compat") || strings.Contains(out, "CONTRACT.md") || strings.Contains(out, "judge") {
			t.Errorf("%s --help: %s", name, out)
		}
	}
}

func TestConformHelpBypassesBashyFormatter(t *testing.T) {
	var want, stderr bytes.Buffer
	conformcli.Run([]string{"--help"}, &want, &stderr)
	got := skillHelpProcess(t, "skill", "conform", "--help")
	if string(got) != want.String() {
		t.Errorf("conform --help = %q, want %q", got, &want)
	}
	got = skillHelpProcess(t, "skill", "selftest", "--help")
	if !bytes.Contains(got, []byte("compat")) || bytes.Contains(got, []byte("CONTRACT.md")) {
		t.Errorf("selftest --help = %q", got)
	}
}
