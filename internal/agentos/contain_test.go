package agentos

import (
	"context"
	"os/exec"
	"runtime"
	"slices"
	"testing"

	"github.com/qiangli/yoke/pkg/atlas"
)

func TestContainedEffectsDropNetOnlyForExternalChildren(t *testing.T) {
	ctx := withContainNet(context.Background())
	got := containedEffects(ctx, "python3", []string{atlas.EffExec, atlas.EffNet, atlas.EffWrite})
	for _, e := range got {
		if e == atlas.EffNet {
			t.Fatalf("contained external child kept net: %v", got)
		}
	}
	// an in-process native tool is not a child: containment does not bound it
	if !slices.Contains(containedEffects(ctx, "fetch", []string{atlas.EffNet}), atlas.EffNet) {
		t.Fatalf("native tool lost its net effect under contain")
	}
	// outside a @contain scope nothing changes
	if !slices.Contains(containedEffects(context.Background(), "python3", []string{atlas.EffNet}), atlas.EffNet) {
		t.Fatalf("net dropped outside a contain scope")
	}
}

func TestDispatchContainUsage(t *testing.T) {
	if code := dispatchContain(nil); code != 2 {
		t.Fatalf("no command: exit %d, want 2", code)
	}
	if code := dispatchContain([]string{"--net", "allow", "--", "true"}); code != 2 {
		t.Fatalf("unsupported mode: exit %d, want 2", code)
	}
}

func TestDispatchContainRunsCommand(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("network isolation backend is darwin/linux only")
	}
	if containSupported() != nil {
		t.Skip("no isolation backend on this host")
	}
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("no true(1)")
	}
	if code := dispatchContain([]string{"--net", "deny", "--", "true"}); code != 0 {
		t.Fatalf("contained true: exit %d", code)
	}
	if code := dispatchContain([]string{"--net", "deny", "--", "false"}); code != 1 {
		t.Fatalf("contained false: exit %d, want 1", code)
	}
}
