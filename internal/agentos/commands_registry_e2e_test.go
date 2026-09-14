//go:build e2e

package agentos

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

// registryEnv is scratchStoreEnv plus an isolated binmgr cache and no cert
// profile, so a download record provisions into the test's own directory.
func registryEnv(t *testing.T) []string {
	t.Helper()
	return append(scratchStoreEnv(t), "BASHY_BIN_CACHE="+t.TempDir(), "VSC_PROFILE=", "BASHY_AGENTIC=")
}

// A script record re-enters the built bashy: $0 is the name, "$@" the
// args, in-shell and at the front door, --posix included; a non-zero body
// under `agentic` yields (exit 6) while an unregistered external keeps its
// exact status.
func TestE2ERegisteredScriptDispatch(t *testing.T) {
	bin := bashyBinary(t)
	env := registryEnv(t)
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "who0", "--set", `script=printf '%s:%s\n' "$0" "$1"`, "--set", "effects.0=pure"); code != 0 {
		t.Fatalf("add (exit %d): %s", code, stderr)
	}
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "fails", "--set", "script=exit 3", "--set", "effects.0=pure"); code != 0 {
		t.Fatalf("add fails (exit %d): %s", code, stderr)
	}
	for _, args := range [][]string{{"-c", "who0 x"}, {"--posix", "-c", "who0 x"}, {"who0", "x"}} {
		stdout, stderr, code := runBashyStdEnv(bin, env, args...)
		if code != 0 || strings.TrimSpace(stdout) != "who0:x" {
			t.Errorf("%v: exit %d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
	stdout, _, code := runBashyStdEnv(bin, env, "agentic", "fails")
	if code != 6 || !strings.Contains(stdout, `"input_required"`) {
		t.Errorf("agentic on a failing registered command: exit %d stdout=%s", code, stdout)
	}
	if _, _, code := runBashyStdEnv(bin, env, "agentic", "who0", "y"); code != 0 {
		t.Errorf("agentic on a passing registered command: exit %d", code)
	}
	if _, _, code := runBashyStdEnv(bin, env, "agentic", "/bin/sh", "-c", "exit 7"); code != 7 {
		t.Errorf("agentic on an unregistered external: exit %d, want 7", code)
	}
	// A certification run never sees the ring.
	if _, _, code := runBashyStdEnv(bin, append(env, "VSC_PROFILE=cert"), "--posix", "-c", "who0 x"); code != 127 {
		t.Errorf("cert profile: exit %d, want 127", code)
	}
	// rm unregisters in every mode.
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "rm", "who0"); code != 0 {
		t.Fatalf("rm (exit %d): %s", code, stderr)
	}
	for _, args := range [][]string{{"-c", "who0 x"}, {"who0", "x"}} {
		if _, _, code := runBashyStdEnv(bin, env, args...); code != 127 {
			t.Errorf("%v after rm: exit %d, want 127", args, code)
		}
	}
}

// A download record provisions on first use from the record's own digest,
// hits the cache afterwards, and is refused — nothing cached — when the
// digest is wrong or absent.
func TestE2ERegisteredDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the served artifact is a #!/bin/sh script")
	}
	bin := bashyBinary(t)
	env := registryEnv(t)
	body := []byte("#!/bin/sh\nprintf 'dl:%s\\n' \"$1\"\n")
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tool" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.Write(body)
	}))
	defer srv.Close()
	plat := binmgr.Platform()

	// No digest → add refuses, nothing is fetched.
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "dl0", "--set", "download.url="+srv.URL+"/{version}/tool", "--set", "download.version=v1"); code == 0 || !strings.Contains(stderr, "sha256") {
		t.Errorf("add without digest: exit %d stderr=%s", code, stderr)
	}
	if hits.Load() != 0 {
		t.Fatalf("add fetched %d times; a record is validated offline", hits.Load())
	}
	// Wrong digest → refused at first use, exit 126, nothing cached.
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "dlbad", "--set", "download.url="+srv.URL+"/{version}/tool", "--set", "download.version=v1", "--set", "download.sha256."+plat+"="+strings.Repeat("0", 64)); code != 0 {
		t.Fatalf("add dlbad (exit %d): %s", code, stderr)
	}
	if _, stderr, code := runBashyStdEnv(bin, env, "-c", "dlbad x"); code != 126 || !strings.Contains(strings.ToLower(stderr), "sha256") {
		t.Errorf("wrong digest: exit %d stderr=%s", code, stderr)
	}
	// Right digest → provisioned once, then served from the cache.
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "dl1", "--set", "download.url="+srv.URL+"/{version}/tool", "--set", "download.version=v1", "--set", "download.sha256."+plat+"="+digest); code != 0 {
		t.Fatalf("add dl1 (exit %d): %s", code, stderr)
	}
	before := hits.Load()
	stdout, stderr, code := runBashyStdEnv(bin, env, "-c", "dl1 first")
	if code != 0 || strings.TrimSpace(stdout) != "dl:first" {
		t.Fatalf("first run: exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, _, code = runBashyStdEnv(bin, env, "dl1", "second")
	if code != 0 || strings.TrimSpace(stdout) != "dl:second" {
		t.Fatalf("second run (front door): exit %d stdout=%q", code, stdout)
	}
	if got := hits.Load() - before; got != 1 {
		t.Errorf("server hits across two runs = %d, want 1 (cache)", got)
	}
	// The record's os defaults to the platforms it carries a digest for.
	stdout, _, _ = runBashyStdEnv(bin, env, "commands", "show", "dl1", "--json")
	var rec map[string]any
	if err := json.Unmarshal([]byte(stdout), &rec); err != nil {
		t.Fatal(err)
	}
	if osList, _ := rec["os"].([]any); len(osList) != 1 || osList[0] != runtime.GOOS {
		t.Errorf("os = %v, want [%s]", rec["os"], runtime.GOOS)
	}
	// verify names the provisioned path.
	if stdout, _, code := runBashyStdEnv(bin, env, "commands", "verify", "dl1"); code != 0 || !strings.Contains(stdout, "provisioned") {
		t.Errorf("verify dl1: exit %d %s", code, stdout)
	}
	// The scratch cache holds exactly the good binary.
	cacheDir := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, "BASHY_BIN_CACHE=") {
			cacheDir = strings.TrimPrefix(kv, "BASHY_BIN_CACHE=")
		}
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "dl1", "v1", "dl1")); err != nil {
		t.Errorf("good binary not cached: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "dlbad", "v1", "dlbad")); err == nil {
		t.Error("a refused download left an executable in the cache")
	}
}
