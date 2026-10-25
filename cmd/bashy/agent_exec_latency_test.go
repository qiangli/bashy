//go:build perf

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// TestAgentExternalCommandLatency catches work accidentally added to every
// external dispatch by a harness marker. Run with -tags=perf on an otherwise
// idle host; it is deliberately separate from the deterministic unit suite.
func TestAgentExternalCommandLatency(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses /usr/bin/true")
	}
	bin := filepath.Join(t.TempDir(), "bashy")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build bashy: %v\n%s", err, out)
	}
	const n = 8
	script := "i=0; while [ $i -lt " + strconv.Itoa(n) + " ]; do /usr/bin/true; i=$((i+1)); done"
	fixtureRepo := filepath.Join(t.TempDir(), "fixture")
	if err := os.MkdirAll(filepath.Join(fixtureRepo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A fresh store hides the regression: the old write path replayed the
	// entire graph log for every command. Seed a representative large log in
	// isolation so the test remains useful on a clean CI host.
	skillsDir := filepath.Join(t.TempDir(), "skills")
	if err := os.MkdirAll(skillsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(skillsDir, "edges.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	line := []byte(`{"schema":"bashy-spacegraph-v1","src":"repo:unrelated","rel":"contains","dst":"path:.","n":1,"ok":1,"first":"2026-10-24T00:00:00Z","last":"2026-10-24T00:00:00Z","valid_from":"2026-10-24T00:00:00Z"}` + "\n")
	block := bytes.Repeat(line, 1024)
	for size := 0; size < 96<<20; size += len(block) {
		if _, err := log.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	baseEnv := []string{"HOME=" + os.Getenv("HOME"), "PATH=/usr/bin:/bin", "BASHY_SKILLS_DIR=" + skillsDir, "BASHY_EXECHIST=" + filepath.Join(t.TempDir(), "history")}
	run := func(agent bool) time.Duration {
		t.Helper()
		cmd := exec.Command(bin, "-c", script)
		cmd.Dir = fixtureRepo
		cmd.Env = append([]string(nil), baseEnv...)
		if agent {
			cmd.Env = append(cmd.Env, "CLAUDECODE=1")
		}
		start := time.Now()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("agent=%t: %v\n%s", agent, err, out)
		}
		return time.Since(start)
	}
	// Warm page cache, then pair samples in both orders to limit host drift.
	run(false)
	run(true)
	var deltas []time.Duration
	for _, agentFirst := range []bool{false, true, false} {
		first, second := run(agentFirst), run(!agentFirst)
		if agentFirst {
			deltas = append(deltas, first-second)
		} else {
			deltas = append(deltas, second-first)
		}
	}
	// The middle sample rejects a single noisy host interval.
	if deltas[0] > deltas[1] {
		deltas[0], deltas[1] = deltas[1], deltas[0]
	}
	if deltas[1] > deltas[2] {
		deltas[1], deltas[2] = deltas[2], deltas[1]
	}
	if deltas[0] > deltas[1] {
		deltas[0], deltas[1] = deltas[1], deltas[0]
	}
	perCommand := deltas[1] / n
	t.Logf("CLAUDECODE overhead: %v/command (paired deltas: %v)", perCommand, deltas)
	if perCommand > 10*time.Millisecond {
		t.Fatalf("agent marker added %v per external command; budget is 10ms", perCommand)
	}
}
