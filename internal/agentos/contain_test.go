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
	ctx := withContainNet(context.Background(), "")
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
	if nativeContainSupported() != nil {
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

func TestModelScopeDoesNotDischargeNet(t *testing.T) {
	ctx := withContainModel(context.Background(), "")
	if containNetFrom(ctx) || !containModelFrom(ctx) {
		t.Fatalf("model scope must not count as a net-off scope")
	}
	got := containedEffects(ctx, "python3", []string{atlas.EffExec, atlas.EffNet})
	if !slices.Contains(got, atlas.EffNet) {
		t.Fatalf("model scope dropped net: %v", got)
	}
	deny := withContainNet(context.Background(), "")
	if nested := withContainModel(deny, ""); !containNetFrom(nested) {
		t.Fatalf("a model scope inside deny must stay deny")
	}
}

func TestContainHandlerWrapsModelScope(t *testing.T) {
	var got []string
	chain := containHandler()(func(ctx context.Context, args []string) error { got = args; return nil })
	if err := chain(withContainModel(context.Background(), ""), []string{"echo", "hi"}); err != nil {
		t.Fatal(err)
	}
	if len(got) < 5 || got[1] != "contain" || got[2] != "--net" || got[3] != "model" {
		t.Fatalf("wrapped argv = %v, want contain --net model", got)
	}
	if err := chain(withContainNet(context.Background(), ""), []string{"echo", "hi"}); err != nil {
		t.Fatal(err)
	}
	if got[3] != "deny" {
		t.Fatalf("deny scope wrapped as %v", got)
	}
}

func TestDispatchContainModelFailsClosedWithoutNativeEgress(t *testing.T) {
	if nativeModelEgress {
		t.Skip("native model egress available")
	}
	if code := dispatchContain([]string{"--net", "model", "--", "true"}); code != containUnsupportedStatus {
		t.Fatalf("model without native egress: exit %d, want %d", code, containUnsupportedStatus)
	}
}
