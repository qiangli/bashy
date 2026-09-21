package agentos

import (
	"context"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/policy/advice"
)

func TestFenceEffectGate(t *testing.T) {
	ctx := context.Background()
	if err := fenceEffectGate(ctx, "iac.apply", []string{"net", "write", "spend"}); err != nil {
		t.Fatalf("no cap must allow: %v", err)
	}
	cap, err := advice.ParseCap("read,net,write")
	if err != nil {
		t.Fatal(err)
	}
	guarded := advice.WithCap(ctx, cap)
	if err := fenceEffectGate(guarded, "iac.plan", []string{"net", "read"}); err != nil {
		t.Fatalf("plan within the cap: %v", err)
	}
	err = fenceEffectGate(guarded, "iac.apply", []string{"net", "write", "spend"})
	if err == nil || !strings.Contains(err.Error(), "iac.apply: declared effects net,write,spend exceed the guard (spend not allowed by") {
		t.Fatalf("apply over the cap: %v", err)
	}
}

func TestFenceRunnerRegistered(t *testing.T) {
	for name, want := range map[string]bool{
		"podman": true, "dag": true, "tofu": true, "doctl": true,
		"ls-nonexistent-runner": false, "": false,
	} {
		if got := fenceRunnerRegistered(name); got != want {
			t.Errorf("fenceRunnerRegistered(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFenceToolsAnswerWithSelf(t *testing.T) {
	for _, tool := range fenceTools {
		row, ok := islandToolchains[tool]
		if !ok {
			t.Fatalf("no toolchain row for %s", tool)
		}
		argv, why, err := row(context.Background())
		if err != nil || len(argv) != 2 || argv[1] != tool || !strings.Contains(why, "bashy "+tool) {
			t.Errorf("%s: argv=%v why=%q err=%v", tool, argv, why, err)
		}
	}
}
