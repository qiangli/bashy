// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the service door's rod contract (Sprint 627 M1,
// docs/service-rod-contract.md).
//
// The door runs under a freshly built bashy and selects through the real
// `bashy providers` selector. Every test points BASHY_COMMANDS_DIR and
// BASHY_HOME at temp dirs and registers small fixture rods there, so a real
// registry is never touched.
package servicedoor

import (
	"context"
	"encoding/json"
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
	buildDir, err = os.MkdirTemp("", "service-door-*")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

// moduleRoot is the bashy repository root (the directory holding cmd/bashy).
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
			t.Fatal("bashy repository root (cmd/bashy) not found above test file")
		}
		dir = parent
	}
}

func bashyBin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture rods are POSIX paths")
	}
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bin := filepath.Join(buildDir, "bashy")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
		if workspace := filepath.Join(filepath.Dir(cmd.Dir), "go.work"); fileExists(workspace) {
			cmd.Env = append(os.Environ(), "GOWORK="+workspace)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err, string(out)}
			return
		}
		buildBin = bin
	})
	if buildErr != nil {
		t.Fatalf("build ./cmd/bashy: %v", buildErr)
	}
	return buildBin
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

// rodScript answers --doctor with one missing dependency, records the
// handoff it received, and writes one artifact. ROD_RESULT=1 makes it print
// a bashy-svc-rod-v1 result instead of plain text.
const rodScript = `#!/usr/bin/env -S bashy --bashsharp
if [ "${1:-}" = --doctor ]; then
    echo '{"schema_version":"bashy-svc-doctor-v1","available":false,"missing":[{"need":"has=no-such-dep-m1","install":"brew install no-such-dep-m1"}]}'
    exit 0
fi
printf 'role=%s route=%s yes=%s request=%s\n' "$BASHY_SVC_ROLE" "$BASHY_SVC_ROUTE" "$BASHY_SVC_YES" "$BASHY_SVC_REQUEST" > "$HANDOFF_LOG"
printf 'RIFF' > "$BASHY_SVC_OUT/take.wav"
if [ "${ROD_RESULT:-0}" = 1 ]; then
    echo '{"schema_version":"bashy-svc-rod-v1","outputs":[{"path":"take.wav","note":"still image with a slow pan"}],"text":"rod result"}'
else
    echo "plain text"
fi
`

type fixture struct {
	t                    *testing.T
	bin, door, root, rod string
	env                  []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{
		t:    t,
		bin:  bashyBin(t),
		door: filepath.Join(moduleRoot(t), "examples", "service-door", "service-door.bsh"),
		root: root,
		rod:  filepath.Join(root, "rod.bsh"),
	}
	for _, d := range []string{"ring", "home"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(f.rod, []byte(rodScript), 0o755); err != nil {
		t.Fatal(err)
	}
	f.env = append(os.Environ(),
		"BASHY_COMMANDS_DIR="+filepath.Join(root, "ring"), "BASHY_HOME="+filepath.Join(root, "home"),
		"BASHY_HINTS=off", "BASHY_TELEMETRY_QUIET=1", "BASHY_SVC_BASHY_BIN="+f.bin,
		"HANDOFF_LOG="+filepath.Join(root, "handoff.log"), "TMPDIR="+root)
	return f
}

// addRod registers the fixture rod under name with the given record fields.
func (f *fixture) addRod(name string, sets ...string) {
	f.t.Helper()
	args := []string{"commands", "add", name, "--set", "exec.0=" + f.bin, "--set", "exec.1=--bashsharp", "--set", "exec.2=" + f.rod}
	for _, s := range sets {
		args = append(args, "--set", s)
	}
	cmd := exec.Command(f.bin, args...)
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("commands add %s: %v\n%s", name, err, out)
	}
}

type envelope struct {
	Schema      string              `json:"schema_version"`
	Role        string              `json:"role"`
	Provider    string              `json:"provider"`
	Route       string              `json:"route"`
	Outcome     string              `json:"outcome"`
	Text        string              `json:"text"`
	CostClass   string              `json:"cost_class"`
	RawOutputs  []map[string]string `json:"outputs"`
	Unavailable []map[string]string `json:"unavailable"`
}

func (f *fixture) runDoor(extra []string, args ...string) (envelope, string, int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bin, append([]string{"--bashsharp", f.door}, args...)...)
	cmd.Env = append(append([]string{}, f.env...), extra...)
	cmd.Dir = f.root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatalf("door %v: %v", args, err)
	}
	var env envelope
	if jerr := json.Unmarshal(out, &env); jerr != nil {
		f.t.Fatalf("door %v: stdout is not one envelope: %v\n%s\nstderr: %s", args, jerr, out, stderr.String())
	}
	if env.Schema != "bashy-svc-v1" {
		f.t.Fatalf("schema_version = %q", env.Schema)
	}
	return env, stderr.String(), code
}

func (f *fixture) handoff() string {
	b, _ := os.ReadFile(filepath.Join(f.root, "handoff.log"))
	return string(b)
}

// The rod receives the request path, out dir, route and consent, and the
// door reports the artifact it added as an absolute, typed output.
func TestRodHandoffAndDiscoveredOutputs(t *testing.T) {
	f := newFixture(t)
	f.addRod("song-local", "provides.0=song", "route=oss", "effects.0=read", "effects.1=write")
	req := filepath.Join(f.root, "song.json")
	os.WriteFile(req, []byte(`{"schema_version":"bashy-svc-request-v1","prompt":"a calm ballad","duration_s":30}`), 0o644)
	out := filepath.Join(f.root, "out")
	env, stderr, code := f.runDoor(nil, "song", "--out", out, req)
	if code != 0 || env.Outcome != "success" {
		t.Fatalf("exit %d outcome %q\n%s", code, env.Outcome, stderr)
	}
	if env.Provider != "song-local" || env.Route != "oss" || env.CostClass != "oss" || env.Text != "plain text" {
		t.Errorf("envelope = %+v", env)
	}
	if len(env.RawOutputs) != 1 || env.RawOutputs[0]["path"] != filepath.Join(out, "take.wav") || env.RawOutputs[0]["media_type"] != "audio/wav" {
		t.Errorf("outputs = %v", env.RawOutputs)
	}
	if got, want := f.handoff(), "role=song route=oss yes=0 request="+req+"\n"; got != want {
		t.Errorf("handoff = %q, want %q", got, want)
	}
}

// A bashy-svc-rod-v1 result names the outputs (relative to the out dir) and
// the text; notes such as a still-image fallback label survive.
func TestRodResultOutputs(t *testing.T) {
	f := newFixture(t)
	f.addRod("animate-local", "provides.0=animate", "route=oss", "effects.0=read", "effects.1=write")
	env, stderr, code := f.runDoor([]string{"ROD_RESULT=1"}, "animate")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if env.Text != "rod result" || len(env.RawOutputs) != 1 {
		t.Fatalf("envelope = %+v", env)
	}
	o := env.RawOutputs[0]
	if !filepath.IsAbs(o["path"]) || filepath.Base(o["path"]) != "take.wav" || o["media_type"] != "audio/wav" || o["note"] != "still image with a slow pan" {
		t.Errorf("output = %v", o)
	}
}

// A rod with a missing dependency is unavailable, and the door prints the
// rod's own install line from its --doctor answer.
func TestMissingDependencyReportsInstallHint(t *testing.T) {
	f := newFixture(t)
	f.addRod("narrate-local", "provides.0=narrate", "route=oss", "requires=has=no-such-dep-m1", "effects.0=read")
	env, stderr, code := f.runDoor(nil, "narrate")
	if code != 126 || env.Outcome != "unavailable" {
		t.Fatalf("exit %d outcome %q", code, env.Outcome)
	}
	if len(env.Unavailable) != 1 || env.Unavailable[0]["provider"] != "narrate-local" || env.Unavailable[0]["install"] != "brew install no-such-dep-m1" || !strings.Contains(env.Unavailable[0]["reason"], "no-such-dep-m1") {
		t.Errorf("unavailable = %v", env.Unavailable)
	}
	if !strings.Contains(stderr, "install: brew install no-such-dep-m1") {
		t.Errorf("stderr lacks the install line:\n%s", stderr)
	}
}

// No registered rod: the door says how to add one instead of failing bare.
func TestNoProviderReportsRegistrationHint(t *testing.T) {
	f := newFixture(t)
	env, stderr, code := f.runDoor(nil, "song")
	if code != 126 || env.Outcome != "unavailable" || len(env.Unavailable) != 1 {
		t.Fatalf("exit %d envelope %+v", code, env)
	}
	if !strings.Contains(env.Unavailable[0]["install"], "provides.0=song") || !strings.Contains(stderr, "service-rod-contract.md") {
		t.Errorf("hint = %v\n%s", env.Unavailable, stderr)
	}
}

// A spend rod is refused without --yes (no rod runs), selected with it, and
// --dry-run reports the route without running it.
func TestSpendNeedsYes(t *testing.T) {
	f := newFixture(t)
	f.addRod("image-paid", "provides.0=image.generate", "route=api", "effects.0=net", "effects.1=spend")
	env, _, code := f.runDoor(nil, "image.generate")
	if code != 126 || env.Outcome != "unavailable" || len(env.Unavailable) != 1 || !strings.Contains(env.Unavailable[0]["install"], "--yes") {
		t.Fatalf("without --yes: exit %d %+v", code, env)
	}
	if f.handoff() != "" {
		t.Fatal("refused rod ran")
	}
	env, _, code = f.runDoor(nil, "image.generate", "--yes", "--dry-run")
	if code != 0 || env.Outcome != "dry-run" || env.Route != "api" || env.CostClass != "api" || f.handoff() != "" {
		t.Fatalf("dry-run: exit %d %+v", code, env)
	}
	env, _, code = f.runDoor(nil, "image.generate", "--yes")
	if code != 0 || env.Outcome != "success" || !strings.Contains(f.handoff(), "yes=1") {
		t.Fatalf("with --yes: exit %d %+v handoff %q", code, env, f.handoff())
	}
}

// --role selects a sibling role of the same service and refuses another.
func TestRoleOverride(t *testing.T) {
	f := newFixture(t)
	f.addRod("image-edit-local", "provides.0=image.edit", "route=oss", "effects.0=read", "effects.1=write")
	env, _, code := f.runDoor(nil, "image.generate", "--role", "image.edit", "--dry-run")
	if code != 0 || env.Role != "image.edit" || env.Provider != "image-edit-local" {
		t.Fatalf("exit %d %+v", code, env)
	}
	cmd := exec.Command(f.bin, "--bashsharp", f.door, "image.generate", "--role", "song")
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "not a image role") {
		t.Fatalf("cross-service --role accepted: %v\n%s", err, out)
	}
}
