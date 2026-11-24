// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Hermetic tests for bashy/examples/trove (trove.bsh + register.sh + embedded
// skill). Drives trove.bsh through a freshly built bashy binary against local
// directories, a local git repository, a fake outpost CLI on PATH, a local
// HTTP fake cloudbox vault, and a local HTTP HTML server. No live account or
// external network access is used.
package troveexample

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce sync.Once
	buildBin  string
	buildErr  error
	buildDir  string
)

func TestMain(m *testing.M) {
	var err error
	buildDir, err = os.MkdirTemp("", "bashy-trove-*")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "bashy")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("bashy repository root not found")
		}
		dir = parent
	}
}

func bashyBin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub outpost fixture is a POSIX sh script")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH")
	}
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bin := filepath.Join(buildDir, "bashy")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build ./cmd/bashy: %w\n%s", err, string(out))
			return
		}
		buildBin = bin
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return buildBin
}

// providerNode mirrors yoke/pkg/lens/provider.Node field-for-field so
// strict JSON decoding enforces the exact bashy-node-v1 contract checked by
// yoke/pkg/lens/provider/recipe_test.go.
type providerNode struct {
	SchemaVersion string         `json:"schema_version"`
	Ref           string         `json:"ref"`
	Path          string         `json:"path"`
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Type          string         `json:"type"`
	Size          *int64         `json:"size,omitempty"`
	Mode          string         `json:"mode,omitempty"`
	Mtime         string         `json:"mtime,omitempty"`
	Rev           string         `json:"rev,omitempty"`
	Children      *uint64        `json:"children,omitempty"`
	Attrs         map[string]any `json:"attrs"`
}

// validateNodeRows applies the exact validation from
// yoke/pkg/lens/provider/recipe.go (used by recipe_test.go) plus strict
// unknown-field checking and trove attrs verification.
func validateNodeRows(t *testing.T, jsonl string) []providerNode {
	t.Helper()
	var rows []providerNode
	dec := json.NewDecoder(strings.NewReader(jsonl))
	dec.DisallowUnknownFields()
	for {
		var n providerNode
		err := dec.Decode(&n)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid bashy-node-v1 JSONL: %v\nraw:\n%s", err, jsonl)
		}
		if n.SchemaVersion != "bashy-node-v1" || n.Ref == "" || n.Kind == "" || n.Type == "" || n.Attrs == nil {
			t.Fatalf("row failed provider.Recipe bashy-node-v1 validation: %+v", n)
		}
		if _, ok := n.Attrs["mount"].(string); !ok {
			t.Fatalf("row missing string attrs.mount: %+v", n)
		}
		if _, ok := n.Attrs["owner"].(string); !ok {
			t.Fatalf("row missing string attrs.owner: %+v", n)
		}
		prov, ok := n.Attrs["provenance"].(map[string]any)
		if !ok {
			t.Fatalf("row missing object attrs.provenance: %+v", n)
		}
		if _, ok := prov["producer"].(string); !ok {
			t.Fatalf("row missing attrs.provenance.producer: %+v", n)
		}
		if _, ok := prov["source"].(string); !ok {
			t.Fatalf("row missing attrs.provenance.source: %+v", n)
		}
		rows = append(rows, n)
	}
	return rows
}

func runTrove(t *testing.T, dir string, extraEnv map[string]string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	bin := bashyBin(t)
	script := filepath.Join(moduleRoot(t), "examples", "trove", "trove.bsh")
	cmd := exec.CommandContext(ctx, bin, append([]string{"--bashsharp", script}, args...)...)
	cmd.Dir = dir

	envMap := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		envMap[k] = v
	}
	// Scrub inherited ambient vault/trove state so tests stay hermetic.
	for _, k := range []string{
		"TROVE_LAYOUT_FILE", "TROVE_VAULT_ENTRY", "TROVE_OWNER", "TROVE_OUTPOST_BIN", "TROVE_GH_BIN",
		"BASHY_SECRETS_TOKEN", "BASHY_CLOUDBOX_URL", "BASHY_API_KEY", "BASHY_PRINCIPAL",
		"BASHY_AGENT", "BASHY_AGENT_ID", "BASHY_AGENTIC",
	} {
		delete(envMap, k)
	}
	envMap["BASHY_BIN"] = bin
	envMap["BASHY_HINTS"] = "off"
	envMap["BASHY_TELEMETRY_QUIET"] = "1"
	for k, v := range extraEnv {
		if v == "" {
			delete(envMap, k)
		} else {
			envMap[k] = v
		}
	}
	// The secret client defaults to the production service when its URL is unset.
	if envMap["BASHY_CLOUDBOX_URL"] == "" {
		envMap["BASHY_CLOUDBOX_URL"] = "http://127.0.0.1:1"
		if envMap["BASHY_SECRETS_TOKEN"] == "" {
			envMap["BASHY_SECRETS_TOKEN"] = "trove-test-offline"
		}
	}
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run trove %v: %v", args, err)
	}
	return out.String(), errb.String(), code
}

// fakeVault serves the cloudbox /api/v1/secrets wire protocol in memory.
type fakeVault struct {
	mu    sync.Mutex
	token string
	items map[string]string
	srv   *httptest.Server
}

func newFakeVault(t *testing.T, token string) *fakeVault {
	t.Helper()
	fv := &fakeVault{token: token, items: map[string]string{}}
	fv.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fv.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/secrets") {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets")
		fv.mu.Lock()
		defer fv.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && (rest == "" || rest == "/"):
			type item struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			}
			out := struct {
				Secrets []item `json:"secrets"`
			}{}
			for k, v := range fv.items {
				out.Secrets = append(out.Secrets, item{Name: k, Value: v})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodGet && strings.HasPrefix(rest, "/"):
			name := strings.TrimPrefix(rest, "/")
			val, ok := fv.items[name]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "value": val})
		case r.Method == http.MethodPost && (rest == "" || rest == "/"):
			var body struct {
				Secrets []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"secrets"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for _, it := range body.Secrets {
				fv.items[it.Name] = it.Value
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(fv.srv.Close)
	return fv
}

func TestSkillHelpAndRegister(t *testing.T) {
	out, errs, code := runTrove(t, t.TempDir(), nil, "--skill")
	if code != 0 || !strings.HasPrefix(out, "---\nname: trove\n") {
		t.Fatalf("--skill: code=%d err=%q out=%.120q", code, errs, out)
	}
	for _, want := range []string{"trove/layout", "bashy-trove-v1", "bashy-node-v1", "mount add"} {
		if !strings.Contains(out, want) {
			t.Fatalf("--skill missing %q", want)
		}
	}

	out, _, code = runTrove(t, t.TempDir(), nil, "--help")
	if code != 0 || !strings.Contains(out, "name: trove") {
		t.Fatalf("--help: code=%d out=%.120q", code, out)
	}

	// Verify register.sh installs both the command and skill in an isolated home.
	isolated := t.TempDir()
	reg := filepath.Join(moduleRoot(t), "examples", "trove", "register.sh")
	bin := bashyBin(t)
	cmd := exec.Command("bash", reg)
	cmd.Env = append(os.Environ(),
		"BASHY_BIN="+bin,
		"HOME="+isolated,
		"BASHY_HOME="+filepath.Join(isolated, ".bashy"),
		"XDG_CONFIG_HOME="+filepath.Join(isolated, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(isolated, ".cache"),
		"BASHY_HINTS=off",
		"BASHY_TELEMETRY_QUIET=1",
	)
	if outb, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("register.sh failed: %v\n%s", err, string(outb))
	}

	show := exec.Command(bin, "skill", "show", "trove")
	show.Env = cmd.Env
	if outb, err := show.CombinedOutput(); err != nil || !strings.Contains(string(outb), "name: trove") {
		t.Fatalf("skill show trove failed: %v\n%s", err, string(outb))
	}
}

func TestMountLifecycleAndVaultRoundTrip(t *testing.T) {
	fv := newFakeVault(t, "tok-vault-test")
	dir := t.TempDir()
	env := map[string]string{
		"BASHY_CLOUDBOX_URL":  fv.srv.URL,
		"BASHY_SECRETS_TOKEN": "tok-vault-test",
		"TROVE_OWNER":         "owner@example.com",
	}

	// Empty vault -> empty bashy-trove-v1 layout.
	out, errs, code := runTrove(t, dir, env, "mount", "ls", "--json")
	if code != 0 {
		t.Fatalf("empty mount ls: code=%d err=%q", code, errs)
	}
	var layout map[string]any
	if err := json.Unmarshal([]byte(out), &layout); err != nil {
		t.Fatalf("invalid layout json: %v (%q)", err, out)
	}
	if layout["schema_version"] != "bashy-trove-v1" || len(layout) != 1 {
		t.Fatalf("unexpected empty layout: %v", layout)
	}

	// --dry-run does not mutate vault.
	out, errs, code = runTrove(t, dir, env, "mount", "add", "/work", "dir:"+dir, "--dry-run", "--json")
	if code != 0 || !strings.Contains(out, `"dry_run":true`) {
		t.Fatalf("mount add dry-run: code=%d out=%q err=%q", code, out, errs)
	}
	if _, exists := fv.items["trove/layout"]; exists {
		t.Fatalf("dry-run wrote to vault")
	}

	// Add dir:, dir:@host, drive:, and gh: mounts matching the spec layout.
	for _, args := range [][]string{
		{"mount", "add", "/work", "dir:" + dir, "--json"},
		{"mount", "add", "/home", "dir:/srv/home@studio-host", "--json"},
		{"mount", "add", "/drive", "drive:My Drive/bashy", "account=me@example.com", "--json"},
		{"mount", "add", "/repo", "gh:owner/repo", "account=owner", "--json"},
	} {
		out, errs, code = runTrove(t, dir, env, args...)
		if code != 0 {
			t.Fatalf("%v: code=%d out=%q err=%q", args, code, out, errs)
		}
	}

	// Verify exact bashy-trove-v1 structure stored in vault entry trove/layout.
	rawVault := fv.items["trove/layout"]
	var stored map[string]any
	if err := json.Unmarshal([]byte(rawVault), &stored); err != nil {
		t.Fatalf("vault trove/layout invalid JSON: %v (%q)", err, rawVault)
	}
	if stored["schema_version"] != "bashy-trove-v1" {
		t.Fatalf("wrong schema_version in vault: %v", stored)
	}
	driveEntry := stored["/drive"].(map[string]any)
	if driveEntry["locator"] != "drive:My Drive/bashy" || driveEntry["account"] != "me@example.com" {
		t.Fatalf("unexpected /drive entry: %v", driveEntry)
	}

	// Cloud mounts require their named vault credentials.
	for _, p := range []string{"/drive", "/repo"} {
		_, errs, code = runTrove(t, dir, env, "ls", p, "--json")
		if code != 1 || !strings.Contains(errs, "no such secret") {
			t.Fatalf("ls %s without vault credential: code=%d err=%q", p, code, errs)
		}
	}

	// mount rm removes a mount; removing missing mount exits 1.
	_, errs, code = runTrove(t, dir, env, "mount", "rm", "/repo", "--json")
	if code != 0 {
		t.Fatalf("mount rm /repo: code=%d err=%q", code, errs)
	}
	_, errs, code = runTrove(t, dir, env, "mount", "rm", "/repo")
	if code != 1 || !strings.Contains(errs, "not found") {
		t.Fatalf("mount rm missing: code=%d err=%q, want exit 1 not found", code, errs)
	}
}

func TestDirMountVerbsAndWrites(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "hello.txt"), []byte("hello trove\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "sub", "deep.md"), []byte("# Deep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layoutFile := filepath.Join(root, "layout.json")
	env := map[string]string{
		"TROVE_LAYOUT_FILE": layoutFile,
		"TROVE_OWNER":       "alice@example.com",
	}

	_, errs, code := runTrove(t, root, env, "mount", "add", "/work", "dir:"+work)
	if code != 0 {
		t.Fatalf("mount add: code=%d err=%q", code, errs)
	}

	// ls / lists configured mounts as valid bashy-node-v1 rows.
	out, errs, code := runTrove(t, root, env, "ls", "/", "--json")
	if code != 0 {
		t.Fatalf("ls /: code=%d err=%q", code, errs)
	}
	rootRows := validateNodeRows(t, out)
	if len(rootRows) != 1 || rootRows[0].Path != "/work" || rootRows[0].Ref != "trove:/work" {
		t.Fatalf("unexpected ls / rows: %+v", rootRows)
	}

	// ls /work lists immediate children as valid bashy-node-v1 rows with trove attrs.
	out, errs, code = runTrove(t, root, env, "ls", "/work", "--json")
	if code != 0 {
		t.Fatalf("ls /work: code=%d err=%q", code, errs)
	}
	rows := validateNodeRows(t, out)
	if len(rows) != 2 {
		t.Fatalf("ls /work want 2 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Path != "/work/hello.txt" || rows[0].Attrs["mount"] != "/work" || rows[0].Attrs["owner"] != "alice@example.com" {
		t.Fatalf("unexpected row[0]: %+v", rows[0])
	}

	// find /work walks recursively.
	out, errs, code = runTrove(t, root, env, "find", "/work", "--json")
	if code != 0 {
		t.Fatalf("find /work: code=%d err=%q", code, errs)
	}
	found := validateNodeRows(t, out)
	var sawDeep bool
	for _, r := range found {
		if r.Path == "/work/sub/deep.md" {
			sawDeep = true
		}
	}
	if !sawDeep {
		t.Fatalf("find /work missed /work/sub/deep.md: %+v", found)
	}

	// stat /work/hello.txt returns one validated node.
	out, errs, code = runTrove(t, root, env, "stat", "/work/hello.txt", "--json")
	if code != 0 {
		t.Fatalf("stat: code=%d err=%q", code, errs)
	}
	st := validateNodeRows(t, out)
	if len(st) != 1 || st[0].Path != "/work/hello.txt" || st[0].Kind != "file" {
		t.Fatalf("unexpected stat: %+v", st)
	}

	// cat raw and --json.
	out, errs, code = runTrove(t, root, env, "cat", "/work/hello.txt")
	if code != 0 || out != "hello trove\n" {
		t.Fatalf("cat raw: code=%d out=%q err=%q", code, out, errs)
	}
	out, errs, code = runTrove(t, root, env, "cat", "/work/hello.txt", "--json")
	if code != 0 {
		t.Fatalf("cat --json: code=%d err=%q", code, errs)
	}
	catRows := validateNodeRows(t, out)
	if len(catRows) != 1 || catRows[0].Attrs["content"] != "hello trove\n" {
		t.Fatalf("unexpected cat --json: %+v", catRows)
	}

	// Unknown path -> not-found (exit 1).
	for _, bad := range []string{"/no-such-mount/x", "/work/missing.txt"} {
		_, errs, code = runTrove(t, root, env, "stat", bad, "--json")
		if code != 1 || !strings.Contains(strings.ToLower(errs), "not found") {
			t.Fatalf("stat %s: code=%d err=%q, want exit 1 not found", bad, code, errs)
		}
		_, errs, code = runTrove(t, root, env, "cat", bad)
		if code != 1 || !strings.Contains(strings.ToLower(errs), "not found") {
			t.Fatalf("cat %s: code=%d err=%q, want exit 1 not found", bad, code, errs)
		}
	}

	// put --dry-run writes nothing; put writes file and returns validated node with provenance.
	src := filepath.Join(root, "upload.txt")
	if err := os.WriteFile(src, []byte("uploaded bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errs, code = runTrove(t, root, env, "put", src, "/work/notes/u.txt", "--dry-run", "--json")
	if code != 0 || !strings.Contains(out, `"dry_run":true`) {
		t.Fatalf("put --dry-run: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(work, "notes", "u.txt")); !os.IsNotExist(err) {
		t.Fatalf("put --dry-run created file")
	}

	out, errs, code = runTrove(t, root, env, "put", src, "/work/notes/u.txt", "--json")
	if code != 0 {
		t.Fatalf("put: code=%d err=%q", code, errs)
	}
	putRows := validateNodeRows(t, out)
	if len(putRows) != 1 || putRows[0].Path != "/work/notes/u.txt" {
		t.Fatalf("unexpected put row: %+v", putRows)
	}
	gotBytes, err := os.ReadFile(filepath.Join(work, "notes", "u.txt"))
	if err != nil || string(gotBytes) != "uploaded bytes\n" {
		t.Fatalf("put file content mismatch: %q err=%v", string(gotBytes), err)
	}

	// add URL PATH fetches local HTTP HTML via bashy fetch --md.
	htmlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><body><h1>Trove Article</h1><p>Hello from test server.</p></body></html>")
	}))
	defer htmlSrv.Close()

	// add --dry-run does not write.
	out, errs, code = runTrove(t, root, env, "add", htmlSrv.URL+"/article", "/work/inbox/article.md", "--dry-run", "--json")
	if code != 0 || !strings.Contains(out, `"dry_run":true`) {
		t.Fatalf("add --dry-run: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(work, "inbox", "article.md")); !os.IsNotExist(err) {
		t.Fatalf("add --dry-run created file")
	}

	out, errs, code = runTrove(t, root, env, "add", htmlSrv.URL+"/article", "/work/inbox/article.md", "--json")
	if code != 0 {
		t.Fatalf("add: code=%d err=%q", code, errs)
	}
	addRows := validateNodeRows(t, out)
	if len(addRows) != 1 || addRows[0].Path != "/work/inbox/article.md" {
		t.Fatalf("unexpected add row: %+v", addRows)
	}
	mdBytes, err := os.ReadFile(filepath.Join(work, "inbox", "article.md"))
	if err != nil || !strings.Contains(string(mdBytes), "Trove Article") || !strings.Contains(string(mdBytes), "Hello from test server.") {
		t.Fatalf("add markdown content unexpected: %q err=%v", string(mdBytes), err)
	}
}

func TestPeerHostMountAndOfflineExit3(t *testing.T) {
	root := t.TempDir()
	stubDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Stub outpost CLI serving one online peer (studio-host) and one offline peer
	// (sleepy-host), mirroring the exact outpost CLI contract used by
	// yoke/cmds/lens/hostscope.go.
	peerBody := "peer host file contents\nline two\n"
	stub := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "peers" ] && [ "$2" = "status" ]; then
  printf '[{"host":"studio-host.example","alias":"studio-host","online":true},{"host":"sleepy-host.example","alias":"sleepy-host","online":false}]\n'
  exit 0
fi
if [ "$1" = "ssh" ] && [ "$2" = "list" ]; then
  printf '[{"name":"studio-ssh","host":"studio-host.example"},{"name":"sleepy-ssh","host":"sleepy-host.example"}]\n'
  exit 0
fi
if [ "$1" = "ssh" ] && [ "$2" = "sftp" ] && [ "$3" = "ls" ]; then
  target="$4"; dir="$5"
  if [ "$target" != "studio-ssh" ]; then
    echo "unexpected target $target" >&2; exit 1
  fi
  case "$dir" in
    .|"") printf 'drwxr-xr-x\t0\tdocs\n' ;;
    docs) printf -- '-rw-r--r--\t%d\thello.txt\n' %d ;;
    *) echo "no such dir $dir" >&2; exit 1 ;;
  esac
  exit 0
fi
if [ "$1" = "ssh" ] && [ "$2" = "sftp" ] && [ "$3" = "get" ]; then
  target="$4"; file="$5"
  if [ "$target" = "studio-ssh" ] && [ "$file" = "docs/hello.txt" ]; then
    cat <<'PEOF'
peer host file contents
line two
PEOF
    exit 0
  fi
  echo "no such file $file" >&2
  exit 1
fi
echo "unexpected outpost invocation: $*" >&2
exit 1
`, len(peerBody), len(peerBody))

	outpostPath := filepath.Join(stubDir, "outpost")
	if err := os.WriteFile(outpostPath, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}

	layoutFile := filepath.Join(root, "layout.json")
	env := map[string]string{
		"PATH":              stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TROVE_LAYOUT_FILE": layoutFile,
		"TROVE_OWNER":       "alice@example.com",
	}

	if _, errs, code := runTrove(t, root, env, "mount", "add", "/studio", "dir:docs@studio-host"); code != 0 {
		t.Fatalf("mount add /studio: code=%d err=%q", code, errs)
	}
	if _, errs, code := runTrove(t, root, env, "mount", "add", "/sleepy", "dir:docs@sleepy-host"); code != 0 {
		t.Fatalf("mount add /sleepy: code=%d err=%q", code, errs)
	}

	// Online peer: ls + stat pass strict bashy-node-v1 validation.
	out, errs, code := runTrove(t, root, env, "ls", "/studio", "--json")
	if code != 0 {
		t.Fatalf("ls /studio: code=%d err=%q", code, errs)
	}
	rows := validateNodeRows(t, out)
	if len(rows) != 1 || rows[0].Path != "/studio/hello.txt" || rows[0].Attrs["mount"] != "/studio" {
		t.Fatalf("unexpected peer ls rows: %+v", rows)
	}

	// Compare trove cat against the peer-host sftp/lens output directly.
	catOut, errs, code := runTrove(t, root, env, "cat", "/studio/hello.txt")
	if code != 0 {
		t.Fatalf("trove cat /studio/hello.txt: code=%d err=%q", code, errs)
	}
	sftpCmd := exec.Command(outpostPath, "ssh", "sftp", "get", "studio-ssh", "docs/hello.txt")
	lensOut, err := sftpCmd.Output()
	if err != nil {
		t.Fatalf("direct outpost sftp get: %v", err)
	}
	if catOut != string(lensOut) || catOut != peerBody {
		t.Fatalf("trove cat %q != lens/sftp output %q", catOut, string(lensOut))
	}

	// Also verify bashy lens -t listing sees the same file on the stub peer.
	lensListCmd := exec.Command(bashyBin(t), "lens", "-t", "listing", "--depth", "0", "dir:docs/hello.txt@studio-host")
	lensListCmd.Env = append(os.Environ(), "PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if lo, err := lensListCmd.CombinedOutput(); err != nil || !strings.Contains(string(lo), "hello.txt") {
		t.Fatalf("bashy lens peer listing failed: %v\n%s", err, string(lo))
	}

	// Peer-host mount is read-only for put -> exit 3.
	src := filepath.Join(root, "x.txt")
	_ = os.WriteFile(src, []byte("x"), 0o644)
	if _, errs, code := runTrove(t, root, env, "put", src, "/studio/x.txt"); code != 3 {
		t.Fatalf("put to peer-host mount: code=%d err=%q, want exit 3", code, errs)
	}

	// Offline peer -> exit 3 naming the host.
	for _, args := range [][]string{
		{"ls", "/sleepy", "--json"},
		{"stat", "/sleepy/hello.txt", "--json"},
		{"cat", "/sleepy/hello.txt"},
	} {
		_, errs, code := runTrove(t, root, env, args...)
		if code != 3 || !strings.Contains(errs, "sleepy-host") || !strings.Contains(strings.ToLower(errs), "offline") {
			t.Fatalf("%v on offline host: code=%d err=%q, want exit 3 naming sleepy-host offline", args, code, errs)
		}
	}
}

func TestGitMountReadOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "readme.md"), []byte("# Git Trove\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	runGit("init", "-q")
	runGit("add", "docs/readme.md")
	runGit("commit", "-q", "-m", "initial")

	layoutFile := filepath.Join(root, "layout.json")
	env := map[string]string{
		"TROVE_LAYOUT_FILE": layoutFile,
		"TROVE_OWNER":       "alice@example.com",
	}

	if _, errs, code := runTrove(t, repo, env, "mount", "add", "/code", "git:HEAD:"); code != 0 {
		t.Fatalf("mount add git: code=%d err=%q", code, errs)
	}

	out, errs, code := runTrove(t, repo, env, "find", "/code", "--json")
	if code != 0 {
		t.Fatalf("find /code: code=%d err=%q", code, errs)
	}
	rows := validateNodeRows(t, out)
	var sawReadme bool
	for _, r := range rows {
		if r.Path == "/code/docs/readme.md" && r.Kind == "file" {
			sawReadme = true
		}
	}
	if !sawReadme {
		t.Fatalf("find /code missed /code/docs/readme.md: %+v", rows)
	}

	out, errs, code = runTrove(t, repo, env, "cat", "/code/docs/readme.md")
	if code != 0 || out != "# Git Trove\n" {
		t.Fatalf("cat git file: code=%d out=%q err=%q", code, out, errs)
	}

	// git: mount is read-only -> exit 3 on put.
	src := filepath.Join(root, "f.txt")
	_ = os.WriteFile(src, []byte("nope"), 0o644)
	if _, errs, code := runTrove(t, repo, env, "put", src, "/code/f.txt"); code != 3 {
		t.Fatalf("put to git mount: code=%d err=%q, want exit 3", code, errs)
	}
}

func TestHostedCallLeavesNoRunnerState(t *testing.T) {
	fv := newFakeVault(t, "hosted-ephemeral-token")
	targetWork := t.TempDir()
	if err := os.WriteFile(filepath.Join(targetWork, "seed.txt"), []byte("seeded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	layoutJSON, _ := json.Marshal(map[string]any{
		"schema_version": "bashy-trove-v1",
		"/work":          map[string]any{"locator": "dir:" + targetWork},
	})
	fv.items["trove/layout"] = string(layoutJSON)

	// Simulate an H1 hosted call environment: isolated scratch tree for
	// HOME / BASHY_HOME / XDG_* / TMPDIR plus per-call identity env vars.
	runnerScratch := t.TempDir()
	scratchHome := filepath.Join(runnerScratch, "home")
	scratchBashy := filepath.Join(runnerScratch, "bhome")
	scratchCfg := filepath.Join(runnerScratch, "cfg")
	scratchCache := filepath.Join(runnerScratch, "cache")
	scratchTmp := filepath.Join(runnerScratch, "tmp")
	for _, d := range []string{scratchHome, scratchBashy, scratchCfg, scratchCache, scratchTmp} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	src := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(src, []byte("from hosted call\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-warm the runner image's immutable adapter cache (as the H4 pilot
	// container image does) for the adapters trove composes (secret, jq,
	// lens, sha256sum), so per-call execution never compiles adapters or
	// downloads Go modules into the per-call scratch tree.
	adapterCache := filepath.Join(buildDir, "adapters")
	warmEnv := append(os.Environ(),
		"BASHY_ADAPTER_CACHE="+adapterCache,
		"BASHY_HINTS=off",
		"BASHY_TELEMETRY_QUIET=1",
	)
	for _, warmArgs := range [][]string{
		{"secret", "gen"},
		{"jq", "-n", "1"},
		{"lens", "-l"},
		{"sha256sum", "/dev/null"},
	} {
		warm := exec.Command(bashyBin(t), warmArgs...)
		warm.Env = warmEnv
		if outb, err := warm.CombinedOutput(); err != nil {
			t.Fatalf("pre-warm %v adapter: %v\n%s", warmArgs, err, string(outb))
		}
	}

	hostedEnv := map[string]string{
		"HOME":                 scratchHome,
		"BASHY_HOME":           scratchBashy,
		"XDG_CONFIG_HOME":      scratchCfg,
		"XDG_CACHE_HOME":       scratchCache,
		"TMPDIR":               scratchTmp,
		"BASHY_ADAPTER_CACHE":  adapterCache,
		"BASHY_CLOUDBOX_URL":   fv.srv.URL,
		"BASHY_SECRETS_TOKEN":  "hosted-ephemeral-token",
		"BASHY_PRINCIPAL":      "hosted-user@example.com",
		"BASHY_HOSTED_TASK_ID": "task-hosted-1",
	}

	out, errs, code := runTrove(t, scratchTmp, hostedEnv, "ls", "/work", "--json")
	if code != 0 {
		t.Fatalf("hosted trove ls: code=%d err=%q", code, errs)
	}
	lsRows := validateNodeRows(t, out)
	if len(lsRows) != 1 || lsRows[0].Path != "/work/seed.txt" || lsRows[0].Attrs["owner"] != "hosted-user@example.com" {
		t.Fatalf("unexpected hosted ls rows: %+v", lsRows)
	}

	out, errs, code = runTrove(t, scratchTmp, hostedEnv, "put", src, "/work/hosted.md", "--json")
	if code != 0 {
		t.Fatalf("hosted trove put: code=%d err=%q", code, errs)
	}
	putRows := validateNodeRows(t, out)
	if len(putRows) != 1 || putRows[0].Path != "/work/hosted.md" {
		t.Fatalf("unexpected hosted put row: %+v", putRows)
	}

	// Neither trove nor secret exec should have written any layout, token, or
	// trove temp files in the runner's config/cache/tmp directories.
	for _, checkDir := range []string{scratchCfg, scratchCache, scratchTmp} {
		entries, err := os.ReadDir(checkDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Fatalf("hosted call left runner state in %s: %v", checkDir, names)
		}
	}

	// Verify the put landed in the target mount.
	got, err := os.ReadFile(filepath.Join(targetWork, "hosted.md"))
	if err != nil || string(got) != "from hosted call\n" {
		t.Fatalf("hosted put output missing: %q err=%v", string(got), err)
	}
}
