// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/bash/interp"
	"github.com/qiangli/yoke/pkg/artifact"
	"github.com/qiangli/yoke/pkg/resources"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

func isolatePressureEnv(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	artDir := filepath.Join(base, "artifacts")
	t.Setenv("BASHY_HOME", filepath.Join(base, "home"))
	t.Setenv("BASHY_ARTIFACTS_DIR", artDir)
	t.Setenv("BASHY_FLEET_DIR", filepath.Join(base, "fleet"))
	t.Setenv("BASHY_TOOLS_DIR", filepath.Join(base, "tools"))
	t.Setenv("BASHY_COMMANDS_DIR", filepath.Join(base, "commands"))
	t.Setenv("BASHY_TOOLS_PATH", "")
	t.Setenv("BASHY_HOST_ADMISSION_POLICY", "")
	return artDir
}

func TestResourcePressureWeaveAdmissionGate(t *testing.T) {
	artDir := isolatePressureEnv(t)
	now := time.Date(2026, 11, 26, 16, 0, 0, 0, time.UTC)

	// 1. Advisory-only (Activated == false) even with ObservedLevel == critical:
	// preserves 100% of existing unactivated behavior (no refusal).
	if err := artifact.SavePressureState(artDir, artifact.PressureStateV1{
		SchemaVersion: artifact.PressureStateSchemaV1,
		UpdatedAt:     now,
		Activated:     false,
		Level:         artifact.PressureCritical,
		ObservedLevel: artifact.PressureCritical,
		GrowthBlocked: false,
		Reasons:       []string{"disk 98% used"},
	}); err != nil {
		t.Fatalf("SavePressureState advisory: %v", err)
	}
	if err := checkArtifactPressureAdmission(""); err != nil {
		t.Fatalf("advisory pressure state unexpectedly blocked admission: %v", err)
	}
	advHooks := buildWeaveResourceHooks()
	if advHooks.RequireHostCheck || advHooks.CheckHost != nil {
		t.Fatalf("expected advisory mode to leave RequireHostCheck=false and CheckHost=nil, got RequireHostCheck=%v", advHooks.RequireHostCheck)
	}
	advCmd := &cobra.Command{Use: "weave"}
	configureWeaveResourceAdmission(advCmd)

	// 2. Activated + GrowthBlocked == true + UnsatisfiedFloor == true:
	// blocks new weave admissions with a clear error naming pressure state and floor details.
	if err := artifact.SavePressureState(artDir, artifact.PressureStateV1{
		SchemaVersion:       artifact.PressureStateSchemaV1,
		UpdatedAt:           now,
		Activated:           true,
		Level:               artifact.PressureCritical,
		ObservedLevel:       artifact.PressureCritical,
		GrowthBlocked:       true,
		ThrottleConcurrency: 1,
		UnsatisfiedFloor:    true,
		FloorDetail: &artifact.UnsatisfiedFloorDetail{
			Path:               artDir,
			ProtectedBytes:     12 << 30,
			LeasedBytes:        2 << 30,
			FreeBytes:          4 << 30,
			RequiredFloorBytes: 10 << 30,
		},
		Reasons: []string{"disk critical: only protected baseline data remains"},
	}); err != nil {
		t.Fatalf("SavePressureState critical: %v", err)
	}

	err := checkArtifactPressureAdmission("")
	if err == nil {
		t.Fatal("expected checkArtifactPressureAdmission to refuse when Activated && GrowthBlocked")
	}
	for _, want := range []string{"state=critical", "unsatisfied_floor=true", "protected_bytes=", "only protected baseline data remains"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("admission error %q missing %q", err.Error(), want)
		}
	}

	// Verify checkHostAdmission also refuses even when host metrics look healthy in obs.
	obs := &resources.HostObservation{
		System: &resources.System{
			CPU:    resources.CPU{Source: "ticks", UsagePercent: 10},
			Memory: resources.Memory{AvailableBytes: 32 * gib},
			Disks:  []resources.Disk{{Mount: artDir, FreeBytes: 100 * gib}},
		},
		Sections: map[string]resources.ObservationStatus{
			"cpu":    {Kind: "actual", At: now, ExpiresAt: now.Add(time.Minute)},
			"memory": {Kind: "actual", At: now, ExpiresAt: now.Add(time.Minute)},
			"disks":  {Kind: "actual", At: now, ExpiresAt: now.Add(time.Minute)},
		},
	}
	if err := checkHostAdmission(hostAdmissionPolicy{Version: 1}, obs, weave.WeaveResourceDemand{Queue: artDir}, now); err == nil {
		t.Fatal("expected checkHostAdmission to refuse under active GrowthBlocked state")
	}

	// Verify configureWeaveResourceAdmission sets RequireHostCheck=true and CheckHost refuses admission.
	critHooks := buildWeaveResourceHooks()
	if !critHooks.RequireHostCheck || critHooks.CheckHost == nil {
		t.Fatalf("expected RequireHostCheck=true and non-nil CheckHost under GrowthBlocked")
	}
	if checkErr := critHooks.CheckHost(context.Background(), weave.WeaveResourceDemand{Run: "1", Queue: artDir}); checkErr == nil || !strings.Contains(checkErr.Error(), "unsatisfied_floor=true") {
		t.Fatalf("expected CheckHost refusal naming unsatisfied_floor, got %v", checkErr)
	}

	weaveCmd := weave.NewWeaveCmd()
	configureWeaveResourceAdmission(weaveCmd)
	var outBuf bytes.Buffer
	weaveCmd.SetOut(&outBuf)
	weaveCmd.SetErr(&outBuf)
	weaveCmd.SetArgs([]string{"list"})
	// Read-only weave list must still succeed cleanly even when new admissions are blocked!
	t.Setenv("BASHY_WEAVE_DIR", filepath.Join(t.TempDir(), "weave"))
	if err := weaveCmd.Execute(); err != nil {
		t.Fatalf("read-only weave list failed under pressure: %v (%s)", err, outBuf.String())
	}
}

func TestResourcePressureHostFloorBlocksGrowthWhileAllowingRecovery(t *testing.T) {
	artDir := isolatePressureEnv(t)
	// Stub host hardware probes to look abundant so ONLY pressure-state.json governs refusal.
	stubHost(t, 16*gib, 32*gib, 200*gib, 500*gib)
	now := time.Date(2026, 11, 26, 16, 30, 0, 0, time.UTC)

	// 1. Advisory state (Activated == false) -> write/growth commands are NOT refused.
	if err := artifact.SavePressureState(artDir, artifact.PressureStateV1{
		SchemaVersion: artifact.PressureStateSchemaV1,
		UpdatedAt:     now,
		Activated:     false,
		Level:         artifact.PressureCritical,
		ObservedLevel: artifact.PressureCritical,
		GrowthBlocked: false,
	}); err != nil {
		t.Fatal(err)
	}
	ranAdv, stderrAdv, errAdv := runFloors(t, "cp a b; go build ./...")
	if errAdv != nil || len(ranAdv) != 2 {
		t.Fatalf("advisory pressure unexpectedly refused commands: ran=%v err=%v stderr=%q", ranAdv, errAdv, stderrAdv)
	}

	// 2. Activated + GrowthBlocked == true -> write/growth commands refused with exit 75,
	// while builtins (echo), read-only classified commands (date), and recovery commands
	// (rm, du, df, ls, ps, kill, bashy resource, bashy prune, git status) remain runnable!
	if err := artifact.SavePressureState(artDir, artifact.PressureStateV1{
		SchemaVersion:       artifact.PressureStateSchemaV1,
		UpdatedAt:           now,
		Activated:           true,
		Level:               artifact.PressureCritical,
		ObservedLevel:       artifact.PressureCritical,
		GrowthBlocked:       true,
		ThrottleConcurrency: 1,
		UnsatisfiedFloor:    true,
		FloorDetail: &artifact.UnsatisfiedFloorDetail{
			Path:               artDir,
			ProtectedBytes:     8 << 30,
			FreeBytes:          3 << 30,
			RequiredFloorBytes: 10 << 30,
		},
		Reasons: []string{"unsatisfied_floor: free_bytes=3221225472 required_floor_bytes=10737418240"},
	}); err != nil {
		t.Fatal(err)
	}

	script := strings.Join([]string{
		"cp a b",
		"echo after_cp:$?",
		"kill -0 $$",
		"rm -f scratch.tmp",
		"du -sh .",
		"df -h",
		"ls -la",
		"ps",
		"pkill -0 agent",
		"bashy resource pressure status",
		"bashy.real resource pressure status",
		"bashy.real prune",
		"git status",
		"date",
		"git commit -m growth",
	}, "; ")
	ran, stderr, runErr := runFloors(t, script)

	var got []string
	for _, argv := range ran {
		got = append(got, strings.Join(argv, " "))
	}
	wantAllowed := []string{
		"rm -f scratch.tmp",
		"du -sh .",
		"df -h",
		"ls -la",
		"ps",
		"pkill -0 agent",
		"bashy resource pressure status",
		"bashy.real resource pressure status",
		"bashy.real prune",
		"git status",
		"date",
	}
	if !slices.Equal(got, wantAllowed) {
		t.Fatalf("allowed commands = %q, want %q\nstderr:\n%s", got, wantAllowed, stderr)
	}

	var exitSt interp.ExitStatus
	if !errors.As(runErr, &exitSt) || exitSt != limitRefusedStatus {
		t.Fatalf("expected trailing git commit refusal exit %d, got %v", limitRefusedStatus, runErr)
	}
	if !strings.Contains(stderr, `"kind":"refused"`) || !strings.Contains(stderr, `"resource":"pressure"`) || !strings.Contains(stderr, `"unsatisfied_floor`) {
		t.Fatalf("unexpected refusal stderr:\n%s", stderr)
	}
}

func TestResourcePressureCLICommonOptionsJSONAndSealedInproc(t *testing.T) {
	artDir := isolatePressureEnv(t)
	rcmd := resources.NewCommand()
	var out, errBuf bytes.Buffer
	rcmd.SetOut(&out)
	rcmd.SetErr(&errBuf)
	outArgs, err := applyCommonOptions(rcmd, []string{"pressure", "status", "--artifact-root", artDir, "--json"})
	if err != nil {
		t.Fatalf("applyCommonOptions(pressure status --artifact-root ... --json): %v", err)
	}
	rcmd.SetArgs(outArgs)
	if err := rcmd.Execute(); err != nil {
		t.Fatalf("rcmd.Execute: %v (stderr: %s)", err, errBuf.String())
	}
	if !strings.Contains(out.String(), `"schema_version"`) {
		t.Fatalf("expected JSON output, got %s", out.String())
	}

	t.Setenv("BASHY_OFFLINE", "1")
	if !shouldRunOfflineOrInproc([]string{"pressure", "status"}) {
		t.Fatal("expected shouldRunOfflineOrInproc true when BASHY_OFFLINE=1")
	}
}
