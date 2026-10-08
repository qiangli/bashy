package agentos

import (
	"slices"
	"testing"
)

func TestOutGraduatedInAtlasAndDefaultCatalog(t *testing.T) {
	_, _, verbs := commandsCatalog()
	if !slices.Contains(verbs, "out") {
		t.Error("out is hidden from the default catalog")
	}
	for _, r := range liveAtlas(true) {
		if r.Name == "out" {
			if r.Hidden || r.Status == statusExperimental {
				t.Fatalf("out has not graduated: %+v", r)
			}
			return
		}
	}
	t.Fatal("out absent from atlas")
}
