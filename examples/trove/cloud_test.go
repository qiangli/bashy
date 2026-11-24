// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package troveexample

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultDefaultsToDeadEndpoint(t *testing.T) {
	ambient := newFakeVault(t, "ambient-token")
	t.Setenv("BASHY_CLOUDBOX_URL", ambient.srv.URL)
	_, errs, code := runTrove(t, t.TempDir(), map[string]string{
		"BASHY_SECRETS_TOKEN": "ambient-token",
	}, "mount", "ls", "--json")
	if code != 1 || !strings.Contains(errs, "127.0.0.1:1") {
		t.Fatalf("expected dead loopback vault: code=%d err=%q", code, errs)
	}
}

func TestCloudMounts(t *testing.T) {
	dir, scratch := t.TempDir(), t.TempDir()
	fv := newFakeVault(t, "cloud-mount-test")
	fv.items["trove/layout"] = `{
		"schema_version":"bashy-trove-v1",
		"/drive":{"locator":"drive:My Drive/Project","account":"drive@example.test"},
		"/repo":{"locator":"gh:demo/repo","account":"work"}
	}`
	fv.items["google/drive@example.test/credentials"] = `{"type":"authorized_user","client_id":"fake-client","client_secret":"fake-secret","refresh_token":"fake-refresh"}`
	fv.items["github/work/token"] = "projected-github-token"

	env := map[string]string{
		"BASHY_CLOUDBOX_URL":             fv.srv.URL,
		"BASHY_SECRETS_TOKEN":            fv.token,
		"BASHY_HOME":                     filepath.Join(dir, "bashy-home"),
		"BASHY_COMMANDS_DIR":             filepath.Join(dir, "commands"),
		"BASHY_COMMANDS_PATH":            "",
		"HOME":                           dir,
		"XDG_CONFIG_HOME":                filepath.Join(dir, "config"),
		"XDG_CACHE_HOME":                 filepath.Join(dir, "cache"),
		"TMPDIR":                         scratch,
		"PATH":                           dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TROVE_GH_BIN":                   "gh",
		"GWS_BIN":                        "",
		"GWS_ACCOUNT":                    "ambient@example.test",
		"GWS_CREDENTIALS_FILE":           "/unused-ambient",
		"GWS_CONFIG_ROOT":                "/unused-ambient-root",
		"GWS_SERVICE_ACCOUNT_KEY_FILE":   "/unused-ambient-key",
		"GOOGLE_WORKSPACE_CLI_TOKEN":     "ambient-google-token",
		"GOOGLE_APPLICATION_CREDENTIALS": "/unused-ambient-app",
		"GH_TOKEN":                       "ambient-github-token",
		"GITHUB_TOKEN":                   "ambient-fallback-token",
	}
	// Keep Go's existing build/module caches when changing HOME in the fixture.
	goCaches, err := exec.Command("go", "env", "GOCACHE", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	env["GOCACHE"], env["GOMODCACHE"], _ = strings.Cut(strings.TrimSpace(string(goCaches)), "\n")
	env["BASHY_ADAPTER_CACHE"] = filepath.Join(buildDir, "adapters")
	// Match the Google wrapper fixture: use an existing interpreter offline.
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	venv := filepath.Join(dir, "venv")
	if err := os.MkdirAll(filepath.Join(venv, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(python, filepath.Join(venv, "bin", "python")); err != nil {
		t.Fatal(err)
	}
	env["VIRTUAL_ENV"] = venv
	for _, tool := range []string{"gws", "gh"} {
		if err := os.WriteFile(filepath.Join(dir, tool), []byte(fakeCloudCLI), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// Use the real registered Google wrapper with only its external gws stubbed.
	bin := bashyBin(t)
	register := exec.Command(bin, "commands", "add", "google", "--force",
		"--set", "exec.0="+bin, "--set", "exec.1=--bashsharp", "--set",
		"exec.2="+filepath.Join(moduleRoot(t), "examples", "google", "google.bsh"))
	register.Dir = dir
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if _, overridden := env[k]; !overridden {
			register.Env = append(register.Env, kv)
		}
	}
	for k, v := range env {
		register.Env = append(register.Env, k+"="+v)
	}
	register.Env = append(register.Env, "BASHY_TELEMETRY_QUIET=1")
	if out, err := register.CombinedOutput(); err != nil {
		t.Fatalf("register google: %v\n%s", err, out)
	}

	run := func(want int, args ...string) string {
		t.Helper()
		out, errs, code := runTrove(t, dir, env, args...)
		if code != want {
			t.Fatalf("trove %v: exit %d, want %d\nstdout: %s\nstderr: %s", args, code, want, out, errs)
		}
		return out
	}
	checkPaths := func(out string, want ...string) []providerNode {
		t.Helper()
		rows := validateNodeRows(t, out)
		paths := make([]string, len(rows))
		for i, row := range rows {
			paths[i] = row.Path
		}
		if strings.Join(paths, "|") != strings.Join(want, "|") {
			t.Fatalf("paths %q, want %q", paths, want)
		}
		return rows
	}

	driveRows := checkPaths(run(0, "ls", "/drive", "--json"), "/drive/O'Brien.txt", "/drive/sub")
	if driveRows[0].Attrs["account"] != "drive@example.test" {
		t.Fatalf("wrong Drive account in row: %+v", driveRows[0])
	}
	if got := run(0, "cat", "/drive/O'Brien.txt"); got != "drive\x00bytes\n\n" {
		t.Fatalf("Drive raw cat = %q", got)
	}
	checkPaths(run(0, "find", "/drive", "--depth", "2", "--name", "*.txt", "--json"),
		"/drive/O'Brien.txt", "/drive/sub/deep.txt")
	run(1, "stat", "/drive/missing")
	run(1, "cat", "/drive/sub")

	src := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(src, []byte("new Drive file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	upload := filepath.Join(dir, "uploaded")
	run(0, "put", "new.txt", "/drive/sub/", "--dry-run", "--json")
	if _, err := os.Stat(upload); !os.IsNotExist(err) {
		t.Fatalf("dry-run reached gws create: %v", err)
	}
	rows := checkPaths(run(0, "put", "new.txt", "/drive/sub/", "--json"), "/drive/sub/new.txt")
	if rows[0].Attrs["provenance"].(map[string]any)["via"] != "trove put" {
		t.Fatalf("missing upload provenance: %+v", rows[0])
	}
	if got := run(0, "cat", "/drive/sub/new.txt"); got != "new Drive file\n" {
		t.Fatalf("Drive upload read-back = %q", got)
	}
	run(1, "put", "new.txt", "/drive/sub/new.txt")

	checkPaths(run(0, "ls", "/repo", "--json"), "/repo/.issues", "/repo/README.md", "/repo/docs")
	if got := run(0, "cat", "/repo/README.md"); got != "# Example\n\n" {
		t.Fatalf("GitHub raw cat = %q", got)
	}
	checkPaths(run(0, "ls", "/repo/.issues", "--json"), "/repo/.issues/7", "/repo/.issues/9")
	row := checkPaths(run(0, "cat", "/repo/.issues/7", "--json"), "/repo/.issues/7")[0]
	if row.Attrs["content"] != "Report\n\nExample body\n" {
		t.Fatalf("issue cat = %+v", row)
	}
	checkPaths(run(0, "find", "/repo", "--depth=2", "--name=*.md", "--json"),
		"/repo/README.md", "/repo/docs/intro.md")
	run(1, "stat", "/repo/absent")
	run(3, "put", src, "/repo/new.txt")
	run(3, "add", "https://example.test/page", "/repo/new.md")

	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) > 0 {
		t.Fatalf("cloud calls left scratch state: %v, %v", entries, err)
	}
}

const fakeCloudCLI = `#!/usr/bin/env python3
import json
import os
import sys
from pathlib import Path

tool = Path(sys.argv[0]).name
args = sys.argv[1:]
root = Path(sys.argv[0]).resolve().parent
folder_type = "application/vnd.google-apps.folder"
sha = "1234567890123456789012345678901234567890"

def emit(value):
    print(json.dumps(value))

def item(file_id, name, mime="text/plain"):
    return {"id": file_id, "name": name, "mimeType": mime}

if tool == "gws":
    assert os.environ["GWS_ACCOUNT"] == "drive@example.test"
    credentials = Path(os.environ["GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE"])
    assert credentials.parent.name == "drive@example.test"
    assert credentials.stat().st_mode & 0o777 == 0o600
    assert json.loads(credentials.read_bytes())["refresh_token"] == "fake-refresh"
    assert not os.environ.get("GOOGLE_WORKSPACE_CLI_TOKEN")
    assert not os.environ.get("GOOGLE_APPLICATION_CREDENTIALS")
    assert args[:2] == ["drive", "files"]
    upload = root / "uploaded"
    if args[2] == "list":
        params = json.loads(args[args.index("--params") + 1])
        query = params["q"]
        listing = {
            "'root' in parents and trashed = false": [item("project", "Project", folder_type)],
            "'project' in parents and trashed = false": [item("note", "O'Brien.txt"), item("sub", "sub", folder_type)],
            "'sub' in parents and trashed = false": [item("deep", "deep.txt")],
        }
        if upload.exists():
            listing["'sub' in parents and trashed = false"].append(item("upload", "new.txt"))
        base, _, named = query.partition(" and name = ")
        files = listing.get(base, [])
        if named:
            files = [entry for entry in files if "'" + entry["name"].replace("'", "\\'") + "'" == named]
        if not named and base.startswith("'project'"):
            if params.get("pageToken") == "p2":
                emit({"files": files[1:]})
            else:
                emit({"files": files[:1], "nextPageToken": "p2"})
        else:
            emit({"files": files})
    elif args[2] == "get":
        params = json.loads(args[args.index("--params") + 1])
        assert params["alt"] == "media" and "fields" not in params
        data = upload.read_bytes() if params["fileId"] == "upload" else b"drive\x00bytes\n\n"
        Path(args[args.index("--output") + 1]).write_bytes(data)
        emit({"ok": True})
    elif args[2] == "create":
        meta = json.loads(args[args.index("--json") + 1])
        assert meta["parents"] == ["sub"] and meta["name"] == "new.txt"
        upload.write_bytes(Path(args[args.index("--upload") + 1]).read_bytes())
        emit(item("upload", "new.txt"))
    else:
        raise AssertionError(args)
elif tool == "gh":
    assert os.environ["GH_TOKEN"] == "projected-github-token"
    assert not os.environ.get("GITHUB_TOKEN")
    assert args[:5] == ["api", "--hostname", "github.com", "--method", "GET"]
    endpoint = args[-1]
    if endpoint == "repos/demo/repo":
        emit({"default_branch": "main", "has_issues": True})
    elif endpoint == "repos/demo/repo/git/trees/main?recursive=1":
        emit({"truncated": False, "tree": [
            {"path": "README.md", "type": "blob", "sha": sha, "mode": "100644", "size": 11},
            {"path": "docs", "type": "tree", "sha": sha, "mode": "040000"},
            {"path": "docs/intro.md", "type": "blob", "sha": sha, "size": 11},
        ]})
    elif endpoint.endswith("/git/blobs/" + sha):
        assert args[5:7] == ["-H", "Accept: application/vnd.github.raw+json"]
        sys.stdout.buffer.write(b"# Example\n\n")
    elif endpoint.endswith("/issues?state=all&per_page=100"):
        assert args[5:7] == ["--paginate", "--slurp"]
        emit([[{"number":7, "title":"Report"}, {"number":8, "pull_request":{}}], [{"number":9}]])
    elif endpoint.endswith("/issues/7"):
        emit({"title":"Report", "body":"Example body\n"})
    else:
        raise AssertionError(args)
else:
    raise AssertionError(tool)
`
