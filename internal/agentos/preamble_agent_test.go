package agentos

import (
	"strings"
	"testing"
)

// Agent mode shims the provisioners, python3/pip3 included (a model types
// them, and a host with only bashy has neither); a human's shell does not.
func TestPreambleForAgentModeShims(t *testing.T) {
	agent := PreambleFor(true)
	for _, want := range []string{"python() {", "python3() {", "pip3() {"} {
		if !strings.Contains(agent, want) {
			t.Errorf("agent preamble lacks %q", want)
		}
	}
	if !strings.Contains(agent, "python3() { command") || !strings.Contains(agent, "' python \"$@\"; }") {
		t.Errorf("python3 must shim to `bashy python`:\n%s", agent)
	}
	human := PreambleFor(false)
	for _, name := range []string{"python() {", "python3() {", "go() {"} {
		if strings.Contains(human, name) {
			t.Errorf("a human's preamble shims %q", name)
		}
	}
}
