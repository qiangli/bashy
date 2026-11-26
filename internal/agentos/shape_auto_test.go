package agentos

import (
	"bytes"
	"strings"
	"testing"

	shapestate "github.com/qiangli/yoke/pkg/shape/state"
)

func runShapeAuto(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := bashyShapeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"auto"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestShapeAutoSwitchOverrideAndLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	if out, err := runShapeAuto(t, "status"); err != nil || !strings.Contains(out, "unattended shaping: on") {
		t.Fatalf("default status = %q, %v", out, err)
	}
	if out, err := runShapeAuto(t, "off"); err != nil || !strings.Contains(out, "unattended shaping: off") || shapestate.AutoEnabled(home) {
		t.Fatalf("off = %q, %v", out, err)
	}
	// An override without a reason is refused and changes nothing.
	if _, err := runShapeAuto(t, "on", "--override"); err == nil || !strings.Contains(err.Error(), "--override requires --reason") {
		t.Fatalf("override without --reason: %v", err)
	}
	if shapestate.AutoEnabled(home) {
		t.Fatal("refused override switched the host on")
	}
	if _, ok := shapestate.AutoOverridePending(home); ok {
		t.Fatal("refused override was armed")
	}
	out, err := runShapeAuto(t, "on", "--override", "--reason", "verifier outage, one long run")
	if err != nil || !strings.Contains(out, "unattended shaping: on") || !strings.Contains(out, "verifier outage, one long run") {
		t.Fatalf("on with override = %q, %v", out, err)
	}
	if why, ok := shapestate.AutoOverridePending(home); !ok || why != "verifier outage, one long run" {
		t.Fatalf("override pending = %q, %v", why, ok)
	}
	out, err = runShapeAuto(t, "log")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], "switch\toff") || !strings.Contains(lines[1], "switch\ton") ||
		!strings.Contains(lines[2], "override\tarmed") || !strings.Contains(lines[2], "verifier outage, one long run") {
		t.Fatalf("log = %q", out)
	}
}
