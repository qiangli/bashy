//go:build bashy_engines

package agentos

import (
	"os"
	"testing"
)

func TestEmbeddedOllamaProbe(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	probe := probeOllamaEngine()
	if probe.Build != "embedded" || probe.Binary != self || probe.Provisionable {
		t.Fatalf("embedded engine probe = %+v", probe)
	}
}
