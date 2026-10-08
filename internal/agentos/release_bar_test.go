package agentos

import (
	"slices"
	"testing"
)

func TestReleaseInstallAgentIsListedAndClassified(t *testing.T) {
	_, _, verbs := commandsCatalog()
	if !slices.Contains(verbs, "install-agent") {
		t.Error("implemented release verb install-agent is absent from commands --json and dispatch e2e")
	}
	r := verbAtlasRecord("install-agent", false)
	if r.Group == "" || r.Tier == "" || r.Stage == "" || len(r.Effects) == 0 {
		t.Errorf("install-agent has no complete atlas entry: %+v", r)
	}
}

func TestReleaseYcodeIsListedAndClassified(t *testing.T) {
	_, _, verbs := commandsCatalog()
	if !slices.Contains(verbs, "ycode") {
		t.Error("implemented release verb ycode is absent from commands --json and dispatch e2e")
	}
	r := verbAtlasRecord("ycode", false)
	if r.Group == "" || r.Tier == "" || len(r.Effects) == 0 {
		t.Errorf("ycode has no complete atlas entry: %+v", r)
	}
}
