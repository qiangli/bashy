package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(dir)
}

func runProfile(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"scripts/vsc-profile.sh"}, args...)...)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "PATH="+t.TempDir()+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func requireCertRejected(t *testing.T, flag, value, want string) {
	t.Helper()
	out, err := runProfile(t, "validate", "--profile", "cert", flag, value)
	if err == nil {
		t.Fatalf("cert %s %s unexpectedly passed:\n%s", flag, value, out)
	}
	if !strings.Contains(out, want) {
		t.Fatalf("cert %s %s output %q, want substring %q", flag, value, out, want)
	}
}

func TestVSCCertProfileRejectsForbiddenConfigurationBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		flag string
		val  string
		want string
	}{
		{"--workers", "2", "workers=1"},
		{"--chunks", "2", "rejects chunking"},
		{"--cache", "on", "rejects cache"},
		{"--retries", "1", "rejects retries"},
		{"--repeat", "1", "repeat>=2"},
		{"--shard", "0", "rejects shard"},
		{"--of", "2", "rejects shard"},
		{"--sut-command", "bash", "resolve as sh"},
	} {
		requireCertRejected(t, tc.flag, tc.val, tc.want)
	}
}

func TestVSCReferenceProfilePreservesSingleAtomOrder(t *testing.T) {
	root := repoRoot(t)
	atomFile := filepath.Join(t.TempDir(), "atoms.txt")
	if err := os.WriteFile(atomFile, []byte("sh_1/assertion-001\nsh_1/assertion-002\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runProfile(t,
		"validate",
		"--profile", "reference",
		"--workers", "1",
		"--chunks", "1",
		"--atom-file", atomFile,
	)
	if err != nil {
		t.Fatalf("reference profile failed in %s: %v\n%s", root, err, out)
	}
	for _, want := range []string{
		"vsc_profile=reference",
		"certifiable=true",
		"workers=1",
		"chunks=1",
		"sut_command=sh",
		"sut_resolved=true",
		"atoms_begin\nsh_1/assertion-001\nsh_1/assertion-002\natoms_end",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, leaked := range []string{"sut_path=", "context_start=", "context_end="} {
		if strings.Contains(out, leaked) {
			t.Fatalf("output leaked private field %q:\n%s", leaked, out)
		}
	}
}

func TestVSCCampaignProfileDeclaresNonCertifiable(t *testing.T) {
	out, err := runProfile(t,
		"validate",
		"--profile", "campaign",
		"--workers", "4",
		"--chunks", "8",
		"--cache", "on",
		"--retries", "1",
		"--shard", "3",
		"--of", "8",
		"--sut-command", "sh",
	)
	if err != nil {
		t.Fatalf("campaign profile failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"vsc_profile=campaign",
		"certifiable=false",
		"NOT CERTIFIABLE",
		"workers=4",
		"chunks=8",
		"sut_resolved=true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, leaked := range []string{"sut_path=", "context_start=", "context_end="} {
		if strings.Contains(out, leaked) {
			t.Fatalf("output leaked private field %q:\n%s", leaked, out)
		}
	}
}
