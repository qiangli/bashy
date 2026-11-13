// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package servicedoor

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// registerBrowserRods adds song-suno, image-chatgpt, and animate-kling to the fixture ring.
func (f *fixture) registerBrowserRods() {
	f.t.Helper()
	rods := []struct {
		name, script string
		sets         []string
	}{
		{
			name:   "song-suno",
			script: "song-suno.bsh",
			sets: []string{
				"provides.0=song", "route=browser", "requires=has=bashy",
				"effects.0=read", "effects.1=write", "effects.2=exec", "effects.3=net", "effects.4=spend",
			},
		},
		{
			name:   "image-chatgpt",
			script: "image-chatgpt.bsh",
			sets: []string{
				"provides.0=image.generate", "provides.1=image.edit", "route=browser", "requires=has=bashy",
				"effects.0=read", "effects.1=write", "effects.2=exec", "effects.3=net", "effects.4=spend",
			},
		},
		{
			name:   "animate-kling",
			script: "animate-kling.bsh",
			sets: []string{
				"provides.0=animate", "route=browser", "requires=has=bashy",
				"effects.0=read", "effects.1=write", "effects.2=exec", "effects.3=net", "effects.4=spend",
			},
		},
	}

	for _, r := range rods {
		scriptPath := filepath.Join(moduleRoot(f.t), "examples", "service-door", r.script)
		args := []string{"commands", "add", r.name, "--set", "exec.0=" + f.bin, "--set", "exec.1=--bashsharp", "--set", "exec.2=" + scriptPath}
		for _, s := range r.sets {
			args = append(args, "--set", s)
		}
		cmd := exec.Command(f.bin, args...)
		cmd.Env = f.env
		if out, err := cmd.CombinedOutput(); err != nil {
			f.t.Fatalf("commands add %s: %v\n%s", r.name, err, out)
		}
	}
}

// Browser rods declare spend effect; without --yes, the door reports them unavailable.
func TestBrowserRodsSpendNeedsYes(t *testing.T) {
	f := newFixture(t)
	f.registerBrowserRods()

	cases := []struct {
		service, role, expectedProvider string
	}{
		{"song", "song", "song-suno"},
		{"image", "image.generate", "image-chatgpt"},
		{"animate", "animate", "animate-kling"},
	}

	for _, tc := range cases {
		t.Run(tc.service+"_without_yes", func(t *testing.T) {
			env, _, code := f.runDoor(nil, tc.role, "--route", "browser")
			if code != 126 || env.Outcome != "unavailable" {
				t.Fatalf("%s without --yes: code %d outcome %q", tc.service, code, env.Outcome)
			}
			// Other routes' providers may be listed too (codex:image is a vendor
			// image provider); check the browser rod's own entry.
			var hint string
			for _, u := range env.Unavailable {
				if u["provider"] == tc.expectedProvider {
					hint = u["install"]
				}
			}
			if !strings.Contains(hint, "--yes") {
				t.Errorf("%s unavailable hint missing --yes: %+v", tc.service, env.Unavailable)
			}
		})

		t.Run(tc.service+"_with_yes_dry_run", func(t *testing.T) {
			env, _, code := f.runDoor(nil, tc.role, "--route", "browser", "--yes", "--dry-run")
			if code != 0 || env.Outcome != "dry-run" {
				t.Fatalf("%s dry-run: code %d outcome %q", tc.service, code, env.Outcome)
			}
			if env.Provider != tc.expectedProvider || env.Route != "browser" || env.CostClass != "browser" {
				t.Errorf("%s envelope mismatch: %+v", tc.service, env)
			}
		})
	}
}

// image-chatgpt also provides image.edit role via --role image.edit.
func TestBrowserImageEditRole(t *testing.T) {
	f := newFixture(t)
	f.registerBrowserRods()

	env, _, code := f.runDoor(nil, "image", "--route", "browser", "--role", "image.edit", "--yes", "--dry-run")
	if code != 0 || env.Outcome != "dry-run" {
		t.Fatalf("image.edit dry-run: code %d outcome %q", code, env.Outcome)
	}
	if env.Role != "image.edit" || env.Provider != "image-chatgpt" || env.Route != "browser" {
		t.Errorf("image.edit envelope mismatch: %+v", env)
	}
}

// Doctor response contract: each browser rod returns bashy-svc-doctor-v1.
func TestBrowserRodsDoctorContract(t *testing.T) {
	f := newFixture(t)
	scripts := []string{"song-suno.bsh", "image-chatgpt.bsh", "animate-kling.bsh"}

	for _, s := range scripts {
		t.Run(s, func(t *testing.T) {
			scriptPath := filepath.Join(moduleRoot(t), "examples", "service-door", s)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, f.bin, "--bashsharp", scriptPath, "--doctor", "--json")
			cmd.Env = f.env
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s --doctor --json: %v\n%s", s, err, out)
			}

			var doc struct {
				Schema    string           `json:"schema_version"`
				Available bool             `json:"available"`
				Missing   []map[string]any `json:"missing"`
			}
			if jerr := json.Unmarshal(out, &doc); jerr != nil {
				t.Fatalf("%s stdout not valid JSON doctor envelope: %v\n%s", s, jerr, out)
			}
			if doc.Schema != "bashy-svc-doctor-v1" {
				t.Errorf("%s schema_version = %q, want bashy-svc-doctor-v1", s, doc.Schema)
			}
		})
	}
}

// Direct invocation without --yes is refused with exit 2.
func TestBrowserRodsDirectInvocationRequiresYes(t *testing.T) {
	f := newFixture(t)
	scripts := []string{"song-suno.bsh", "image-chatgpt.bsh", "animate-kling.bsh"}

	for _, s := range scripts {
		t.Run(s, func(t *testing.T) {
			scriptPath := filepath.Join(moduleRoot(t), "examples", "service-door", s)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, f.bin, "--bashsharp", scriptPath)
			cmd.Env = f.env
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("%s without --yes should exit non-zero; output: %s", s, out)
			}
			ee, ok := err.(*exec.ExitError)
			if !ok || ee.ExitCode() != 2 {
				t.Errorf("%s exit code = %v, want 2", s, err)
			}
			if !strings.Contains(string(out), "requires --yes") {
				t.Errorf("%s output missing 'requires --yes': %s", s, out)
			}
		})
	}
}

// Direct invocation with --dry-run returns bashy-svc-rod-v1.
func TestBrowserRodsDirectInvocationDryRun(t *testing.T) {
	f := newFixture(t)
	scripts := []string{"song-suno.bsh", "image-chatgpt.bsh", "animate-kling.bsh"}

	for _, s := range scripts {
		t.Run(s, func(t *testing.T) {
			scriptPath := filepath.Join(moduleRoot(t), "examples", "service-door", s)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, f.bin, "--bashsharp", scriptPath, "--dry-run")
			cmd.Env = f.env
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s --dry-run: %v\n%s", s, err, out)
			}

			var rod struct {
				Schema  string              `json:"schema_version"`
				Outputs []map[string]string `json:"outputs"`
				Text    string              `json:"text"`
			}
			if jerr := json.Unmarshal(out, &rod); jerr != nil {
				t.Fatalf("%s not valid rod result: %v\n%s", s, jerr, out)
			}
			if rod.Schema != "bashy-svc-rod-v1" {
				t.Errorf("%s schema_version = %q, want bashy-svc-rod-v1", s, rod.Schema)
			}
			if len(rod.Outputs) == 0 {
				t.Errorf("%s outputs empty in dry run", s)
			}
		})
	}
}

// Test with recorded page fixtures: loads fixture, checks elements, writes outputs.
func TestBrowserRodsRecordedFixtures(t *testing.T) {
	f := newFixture(t)
	f.registerBrowserRods()

	fixtureDir := filepath.Join(moduleRoot(t), "examples", "service-door", "fixtures")

	cases := []struct {
		name        string
		service     string
		role        string
		fixtureFile string
		expectedExt string
		mediaType   string
	}{
		{"song_create", "song", "song", "suno.html", ".mp3", "audio/mpeg"},
		{"song_completed", "song", "song", "suno-completed.html", ".mp3", "audio/mpeg"},
		{"image_create", "image", "image.generate", "chatgpt.html", ".png", "image/png"},
		{"image_completed", "image", "image.generate", "chatgpt-completed.html", ".png", "image/png"},
		{"animate_create", "animate", "animate", "kling.html", ".mp4", "video/mp4"},
		{"animate_completed", "animate", "animate", "kling-completed.html", ".mp4", "video/mp4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixturePath := filepath.Join(fixtureDir, tc.fixtureFile)
			outDir := filepath.Join(f.root, "out-"+tc.name)
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				t.Fatal(err)
			}

			env, stderr, code := f.runDoor(
				[]string{"BASHY_SVC_FIXTURE=" + fixturePath},
				tc.role, "--route", "browser", "--yes", "--out", outDir,
			)
			if code != 0 || env.Outcome != "success" {
				t.Fatalf("%s fixture run failed: code %d outcome %q\nstderr: %s", tc.name, code, env.Outcome, stderr)
			}
			if env.Route != "browser" || env.CostClass != "browser" {
				t.Errorf("%s envelope route/cost_class: %+v", tc.name, env)
			}
			if len(env.RawOutputs) == 0 {
				t.Fatalf("%s returned no outputs", tc.name)
			}

			outPath := env.RawOutputs[0]["path"]
			if !strings.HasSuffix(outPath, tc.expectedExt) {
				t.Errorf("output %q does not have expected extension %q", outPath, tc.expectedExt)
			}
			if env.RawOutputs[0]["media_type"] != tc.mediaType {
				t.Errorf("output media_type = %q, want %q", env.RawOutputs[0]["media_type"], tc.mediaType)
			}
			if fi, err := os.Stat(outPath); err != nil || fi.Size() == 0 {
				t.Errorf("output file %s not written or empty (err=%v)", outPath, err)
			}
			if _, err := exec.LookPath("ffprobe"); err == nil {
				cmd := exec.Command("ffprobe", "-v", "error", outPath)
				if probeOut, err := cmd.CombinedOutput(); err != nil {
					t.Errorf("ffprobe validation failed for %s: %v\n%s", outPath, err, probeOut)
				}
			}
		})
	}
}
