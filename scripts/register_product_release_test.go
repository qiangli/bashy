package scripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Regression for the rc.41 finding (story a0f0c4aadb31): the registration
// step required outpost-<tag>-darwin-amd64, which is no longer built, and
// looked up the outpost commit on the GitHub API without a token.
func TestRegisterProductReleaseRegistersBuiltPlatforms(t *testing.T) {
	for _, tool := range []string{"bash", "jq", "shasum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not on PATH", tool)
		}
	}
	root := repoRoot(t)
	built := []string{"darwin_arm64", "linux_amd64", "linux_arm64", "windows_amd64", "windows_arm64"}
	dist := t.TempDir()
	for _, p := range built {
		name := "outpost-v9.9.9-" + strings.ReplaceAll(p, "_", "-")
		if strings.HasPrefix(p, "windows") {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(dist, name), []byte(p), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func() ([]byte, error) {
		cmd := exec.Command("bash", filepath.Join(root, "scripts/register-product-release.sh"))
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "VALIDATE_ONLY=1", "REPO=example/bashy", "TAG=v9.9.9-dev",
			"PRERELEASE=true", "DIST="+dist, "OUTPOST_COMMIT="+strings.Repeat("a", 40))
		return cmd.Output()
	}
	out, err := run()
	if err != nil {
		t.Fatalf("register-product-release.sh: %v", err)
	}
	var payload struct {
		Artifacts map[string]struct{ URL, SHA256 string } `json:"artifacts"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("payload: %v\n%s", err, out)
	}
	var keys []string
	for k := range payload.Artifacts {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, built) {
		t.Fatalf("artifacts = %v, want %v", keys, built)
	}

	// A built platform that is missing still fails closed.
	if err := os.Remove(filepath.Join(dist, "outpost-v9.9.9-linux-arm64")); err != nil {
		t.Fatal(err)
	}
	if _, err := run(); err == nil {
		t.Fatal("missing linux/arm64 outpost accepted")
	}

	// Every workflow step that runs the script passes a token for the commit
	// lookup. promote.yml has two: the promotion itself, and the register_only
	// retry, which runs this ref's script (a fix made after promotion applies).
	step := regexp.MustCompile(`(?s)- name: register (stable|development) product release\n(.*?)run: (\.\.?)/scripts/register-product-release\.sh`)
	for wf, want := range map[string]int{".github/workflows/release.yml": 1, ".github/workflows/promote.yml": 2} {
		data, err := os.ReadFile(filepath.Join(root, wf))
		if err != nil {
			t.Fatal(err)
		}
		ms := step.FindAllSubmatch(data, -1)
		if len(ms) != want {
			t.Fatalf("%s: %d registration steps, want %d", wf, len(ms), want)
		}
		for _, m := range ms {
			if !strings.Contains(string(m[2]), "GITHUB_TOKEN: ${{ github.token }}") {
				t.Errorf("%s: registration step passes no GITHUB_TOKEN", wf)
			}
		}
	}
}
