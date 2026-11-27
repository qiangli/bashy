package agentos

import (
	"strings"
	"testing"

	geniesrc "github.com/qiangli/ycode/examples/genie"
)

// The genie bashy embeds forwards BASHY_DELEGATE_DEPTH to its shell, or the
// nested-delegation count resets at every genie hop (Sprint 732 delegate
// depth). Guards the pinned ycode/examples/genie module, which moves on its
// own replace line.
func TestEmbeddedGenieForwardsDelegateDepth(t *testing.T) {
	for _, name := range []string{"agent.yaml", "agent-yolo.yaml"} {
		data, err := geniesrc.Source.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "BASHY_DELEGATE_DEPTH") {
			t.Errorf("embedded genie %s does not forward BASHY_DELEGATE_DEPTH", name)
		}
	}
}
