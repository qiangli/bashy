// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the website and shaping-host Bash# commands (Sprint 615
// S17). The scripts run under the installed bashy (`bashy --bashsharp`;
// skipped when bashy is not on PATH) against fakes only: the outpost
// override (WEBSITE_OUTPOST_BIN / SHAPING_OUTPOST_BIN) records argv and
// answers `apps secret`, `status --json`, `status --local-presence` and
// `satellite list --json` from the environment, the go override
// (WEBSITE_GO_BIN) writes a stub server binary, and WEBSITE_STATE points at
// a temp dir. A real outpost, the real toolchain and the network are never
// touched.
package website

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// bashyBin is the installed bashy; these tests skip without it.
func bashyBin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fakes are POSIX shell fixtures")
	}
	bin, err := exec.LookPath("bashy")
	if err != nil {
		t.Skip("bashy is not on PATH")
	}
	return bin
}

// scriptsDir is the directory holding website.bsh (and the test file);
// shaping-host.bsh lives two levels up in examples/shaping-host.
func scriptsDir(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	return filepath.Join(dir, "website.bsh"),
		filepath.Join(dir, "..", "..", "shaping-host", "shaping-host.bsh")
}

// fakeOutpost records argv and answers the probes from the environment:
// $FAKE_PAIRED (status --json reports the paired state), $FAKE_ATTACHED
// (satellite list --json reports an attached satellite), $FAKE_RUNNING
// (status --local-presence succeeds) and $FAKE_SECRET (apps secret).
const fakeOutpost = `#!/bin/sh
echo "$*" >> "$FAKE_LOG"
case "$1 $2" in
  "apps secret") [ -n "$FAKE_SECRET" ] && printf '%s\n' "$FAKE_SECRET"; exit 0 ;;
  "status --json")
    if [ "$FAKE_PAIRED" = 1 ]; then echo '{"state":"paired","config":{"cloudbox":{"host":"shape-host","configured":true}}}'; else echo '{"state":"dormant"}'; fi ;;
  "status --local-presence")
    if [ "$FAKE_RUNNING" = 1 ]; then echo '{"running":true}'; else echo '{"running":false}'; exit 1; fi ;;
  "satellite list")
    if [ "$FAKE_ATTACHED" = 1 ]; then echo '{"satellites":[{"id":"sat_1","name":"home","attached":true}]}'; else echo '{"satellites":[]}'; fi ;;
esac
exit 0
`

// fakeGo stands in for the toolchain: it records argv and writes the
// requested build output as a bounded live stub (`go -C DIR build ... -o OUT`).
const fakeGo = `#!/bin/sh
echo "$*" >> "$FAKE_GO_LOG"
dir="" out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -C) dir="$2"; shift 2 ;;
    -o) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
[ -n "$dir" ] && [ -n "$out" ] || { echo "fake go: unexpected args: $*" >&2; exit 1; }
case "$out" in /*) ;; *) out="$dir/$out" ;; esac
mkdir -p "$(dirname "$out")"
printf '#!/bin/sh\nfor i in 1 2 3 4 5 6; do sleep 5; done\n' > "$out"
chmod +x "$out"
`

// fakeBashy records argv only (register runs it in dry-run mode at most).
const fakeBashy = `#!/bin/sh
echo "$*" >> "$FAKE_BASHY_LOG"
exit 0
`

type fixture struct {
	t                           *testing.T
	bin                         string
	root                        string
	log, bashyLog, goLog, state string
	site                        string
	env                         []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	fakes := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakes, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"outpost": fakeOutpost, "go": fakeGo, "bashy": fakeBashy,
	} {
		p := filepath.Join(fakes, name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{t: t, bin: bashyBin(t), root: root,
		log:      filepath.Join(root, "outpost-calls.log"),
		bashyLog: filepath.Join(root, "bashy-calls.log"),
		goLog:    filepath.Join(root, "go-calls.log"),
		state:    filepath.Join(root, "state"),
		site:     filepath.Join(root, "site"),
	}
	f.env = []string{
		"HOME=" + filepath.Join(root, "home"),
		"BASHY_HINTS=off",
		"FAKE_LOG=" + f.log,
		"FAKE_BASHY_LOG=" + f.bashyLog,
		"FAKE_GO_LOG=" + f.goLog,
		"WEBSITE_OUTPOST_BIN=" + filepath.Join(fakes, "outpost"),
		"SHAPING_OUTPOST_BIN=" + filepath.Join(fakes, "outpost"),
		"WEBSITE_BASHY_BIN=" + filepath.Join(fakes, "bashy"),
		"WEBSITE_GO_BIN=" + filepath.Join(fakes, "go"),
		"WEBSITE_STATE=" + f.state,
	}
	return f
}

// run runs one script under the installed bashy. The inherited environment
// is stripped of every variable that could steer the scripts at real state
// (pair code, satellite, workspace platform markers) before the fixture's
// overrides and the per-call extras are applied.
func (f *fixture) run(script string, extra []string, args ...string) (string, int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bin, append([]string{"--bashsharp", script}, args...)...)
	cmd.Env = append(cleanEnviron(), append(append([]string{}, f.env...), extra...)...)
	cmd.Dir = f.root
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatalf("run %v: %v", args, err)
	}
	return string(out), code
}

func (f *fixture) website(extra []string, args ...string) (string, int) {
	websiteScript, _ := scriptsDir(f.t)
	return f.run(websiteScript, extra, args...)
}

func (f *fixture) host(extra []string, args ...string) (string, int) {
	_, hostScript := scriptsDir(f.t)
	return f.run(hostScript, extra, args...)
}

// calls / bashyCalls / goCalls are what the fakes received.
func (f *fixture) calls() string      { b, _ := os.ReadFile(f.log); return string(b) }
func (f *fixture) bashyCalls() string { b, _ := os.ReadFile(f.bashyLog); return string(b) }
func (f *fixture) goCalls() string    { b, _ := os.ReadFile(f.goLog); return string(b) }

const (
	paired   = "FAKE_PAIRED=1"
	attached = "FAKE_ATTACHED=1"
	running  = "FAKE_RUNNING=1"
	secret   = "FAKE_SECRET=sso-stub-7f3c9d"
)

func cleanEnviron() []string {
	drop := []string{
		"BASHY_PAIR_CODE", "SHAPING_SATELLITE", "SHAPING_OUTPOST_REF", "SHAPING_HOST_NAME",
		"CODESPACES", "GITPOD_WORKSPACE_ID", "GITHUB_REPOSITORY",
		"WEBSITE_OUTPOST_BIN", "WEBSITE_BASHY_BIN", "WEBSITE_GO_BIN", "WEBSITE_STATE",
		"SHAPING_OUTPOST_BIN",
	}
	var keep []string
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		skip := false
		for _, d := range drop {
			if key == d {
				skip = true
				break
			}
		}
		if !skip {
			keep = append(keep, kv)
		}
	}
	return keep
}

func wantAll(t *testing.T, what, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("%s missing %q:\n%s", what, w, out)
		}
	}
}

// enable writes every template file plus .devcontainer, bin/{website,
// shaping-host}.bsh and site.json, and keeps files that already exist.
func TestEnableWritesLayerAndKeepsExistingFiles(t *testing.T) {
	f := newFixture(t)
	out, code := f.website(nil, "enable", f.site, "--site", "https://example.com",
		"--title", "My Name", "--contact", "mailto:me@example.com")
	if code != 0 {
		t.Fatalf("enable: exit %d\n%s", code, out)
	}
	for _, name := range []string{
		"go.mod", "main.go", "identity.go", "audience.go", "proxy.go", "view.go",
		"main_test.go", "shaped.json", "contract.bsh", "site.json", ".gitignore",
	} {
		b, err := os.ReadFile(filepath.Join(f.site, name))
		if err != nil || len(b) == 0 {
			t.Errorf("enable did not write %s", name)
		}
	}
	for _, name := range []string{"public", "checks", "fields", ".devcontainer", ".git", "bin"} {
		if fi, err := os.Stat(filepath.Join(f.site, name)); err != nil || !fi.IsDir() {
			t.Errorf("enable did not write %s/", name)
		}
	}
	for _, name := range []string{"website.bsh", "shaping-host.bsh"} {
		fi, err := os.Stat(filepath.Join(f.site, "bin", name))
		if err != nil || fi.Mode()&0111 == 0 {
			t.Errorf("bin/%s not copied executable", name)
		}
	}
	siteJSON, _ := os.ReadFile(filepath.Join(f.site, "site.json"))
	wantAll(t, "site.json", string(siteJSON),
		`"site_url": "https://example.com"`, `"title": "My Name"`, `"contact": "mailto:me@example.com"`)

	// A second enable over a dir with its own main.go, site.json and fields/
	// keeps them untouched.
	kept := filepath.Join(f.root, "kept")
	if err := os.MkdirAll(filepath.Join(kept, "fields"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, body := range map[string]string{
		filepath.Join(kept, "main.go"):        "package main // keep-me\n",
		filepath.Join(kept, "site.json"):      "{\"KEEP\": true}\n",
		filepath.Join(kept, "fields", "a.md"): "keep-dir\n",
	} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, code = f.website(nil, "enable", kept, "--site", "https://example.com")
	if code != 0 {
		t.Fatalf("re-enable: exit %d\n%s", code, out)
	}
	wantAll(t, "re-enable", out, "keep: main.go (exists)", "keep: site.json (exists)", "keep: fields/ (exists)")
	for p, want := range map[string]string{
		filepath.Join(kept, "main.go"):        "keep-me",
		filepath.Join(kept, "site.json"):      "KEEP",
		filepath.Join(kept, "fields", "a.md"): "keep-dir",
	} {
		if b, _ := os.ReadFile(p); !strings.Contains(string(b), want) {
			t.Errorf("%s was overwritten", p)
		}
	}
}

// enable refuses values it cannot embed in site.json unescaped.
func TestEnableRefusesQuoteInTitle(t *testing.T) {
	f := newFixture(t)
	out, code := f.website(nil, "enable", f.site, "--site", "https://example.com",
		"--title", `bad "quote" title`)
	if code == 0 {
		t.Fatalf("quote title accepted:\n%s", out)
	}
	wantAll(t, "quote refusal", out, "quote or backslash")
}

// serve --dry-run prints the build, the outpost app registration and the
// server start, and runs none of them.
func TestServeDryRunPrintsBuildRegisterAndStart(t *testing.T) {
	f := newFixture(t)
	if out, code := f.website(nil, "enable", f.site, "--site", "https://example.com"); code != 0 {
		t.Fatalf("enable: exit %d\n%s", code, out)
	}
	out, code := f.website(nil, "serve", f.site, "--name", "site", "--dry-run")
	if code != 0 {
		t.Fatalf("serve --dry-run: exit %d\n%s", code, out)
	}
	wantAll(t, "serve --dry-run", out,
		"GOWORK=off CGO_ENABLED=0",
		"build -trimpath -o .build/site-shaping .",
		"apps add site --port 18930 --trust-cloud-identity",
		"-addr 127.0.0.1:18930",
		"SITE_SSO_SECRET from outpost apps secret site")
	if got := f.goCalls(); got != "" {
		t.Errorf("dry-run built: %q", got)
	}
	if got := f.calls(); got != "" {
		t.Errorf("dry-run called outpost: %q", got)
	}
}

// register --dry-run prints the bashy app add call for the shaping layer.
func TestRegisterDryRunPrintsBashyAppAdd(t *testing.T) {
	f := newFixture(t)
	if out, code := f.website(nil, "enable", f.site, "--site", "https://example.com"); code != 0 {
		t.Fatalf("enable: exit %d\n%s", code, out)
	}
	out, code := f.website(nil, "register", f.site, "--name", "site", "--dry-run")
	if code != 0 {
		t.Fatalf("register --dry-run: exit %d\n%s", code, out)
	}
	wantAll(t, "register --dry-run", out,
		"app add site-shaping --type shaped",
		"--source "+f.site,
		"--label Shaping layer for site")
	if got := f.bashyCalls(); got != "" {
		t.Errorf("dry-run called bashy: %q", got)
	}
}

// shaping-host check reports paired/attached from the fake outpost probes.
func TestShapingHostCheckReportsPairedAttached(t *testing.T) {
	f := newFixture(t)
	out, code := f.host(nil, "check")
	if code != 0 {
		t.Fatalf("check: exit %d\n%s", code, out)
	}
	wantAll(t, "bare check", out, `"outpost":true`, `"paired":false`, `"attached":false`)
	out, code = f.host([]string{paired, attached, running}, "check")
	if code != 0 {
		t.Fatalf("paired check: exit %d\n%s", code, out)
	}
	wantAll(t, "paired check", out, `"running":true`, `"paired":true`, `"attached":true`)
}

// up with nothing missing only probes and registers/attaches nothing.
func TestUpWithNothingMissingRegistersNothing(t *testing.T) {
	f := newFixture(t)
	out, code := f.host([]string{paired, attached, running}, "up")
	if code != 0 {
		t.Fatalf("up: exit %d\n%s", code, out)
	}
	wantAll(t, "up done", out, `"paired":true`, `"attached":true`)
	got := f.calls()
	wantAll(t, "up probes", got, "status --json", "satellite list --json")
	for _, bad := range []string{"register", "satellite attach", "install.sh"} {
		if strings.Contains(got, bad) {
			t.Errorf("up with nothing missing ran %q: %q", bad, got)
		}
	}
}

// up unpaired without BASHY_PAIR_CODE stops and names the secret.
func TestUpUnpairedWithoutPairCodeFails(t *testing.T) {
	f := newFixture(t)
	out, code := f.host([]string{running}, "up")
	if code == 0 {
		t.Fatalf("unpaired up succeeded:\n%s", out)
	}
	wantAll(t, "unpaired up", out, "BASHY_PAIR_CODE")
	if got := f.calls(); strings.Contains(got, "register") {
		t.Errorf("unpaired up registered anyway: %q", got)
	}
}

// serve for real builds through WEBSITE_GO_BIN (a stub server binary),
// registers the app and reads its SSO secret from the fake outpost; stop
// ends it. Nothing outside temp dirs is touched.
func TestServeBuildsViaFakeGoAndStops(t *testing.T) {
	f := newFixture(t)
	if out, code := f.website(nil, "enable", f.site, "--site", "https://example.com"); code != 0 {
		t.Fatalf("enable: exit %d\n%s", code, out)
	}
	stub := filepath.Join(f.site, ".build", "site-shaping")
	t.Cleanup(func() { exec.Command("pkill", "-f", stub).Run() })
	out, code := f.website([]string{secret}, "serve", f.site, "--name", "site")
	if code != 0 {
		t.Fatalf("serve: exit %d\n%s", code, out)
	}
	wantAll(t, "serve", out,
		"serving: site on 127.0.0.1:18930",
		"engine face: https://<cloudbox>/matrix/h/<this host>/app/site/")
	if fi, err := os.Stat(stub); err != nil || fi.Mode()&0111 == 0 {
		t.Errorf("fake go did not write the stub server binary: %v", err)
	}
	wantAll(t, "go argv", f.goCalls(), "build -trimpath -o .build/site-shaping .")
	wantAll(t, "outpost argv", f.calls(),
		"apps add site --port 18930 --trust-cloud-identity",
		"apps secret site")
	out, code = f.website(nil, "stop", "--name", "site")
	if code != 0 || !strings.Contains(out, "stopped: site") {
		t.Errorf("stop: exit %d\n%s", code, out)
	}
}
