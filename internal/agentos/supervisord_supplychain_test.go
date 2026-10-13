package agentos

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

// Service supply-chain policy (Sprint 433): every start path refuses a unit
// whose binary is not a verified bashy-signed build — the monitor at
// activation and at an explicit start/restart, the CLI before it asks the
// monitor, and `service run`. No network: the refusal precedes any install.

// The default gate is the host catalog's CheckServiceBuild: a service pin
// without a Sigstore identity is refused with the policy message; a unit
// naming no tool is unaffected.
func TestCheckServiceBuildRefusesUnsignedToolPin(t *testing.T) {
	isolateFleet(t)
	tool := saveUnsignedServiceTool(t)
	err := checkServiceBuild(fleet.Service{Name: "unsigned-service", Tool: tool})
	if !errors.Is(err, fleet.ErrUnsignedServiceBuild) || !strings.Contains(err.Error(), "bashy only runs bashy-signed server builds") {
		t.Fatalf("err = %v", err)
	}
	if err := checkServiceBuild(fleet.Service{Name: "web"}); err != nil {
		t.Fatalf("toolless unit: %v", err)
	}
	if err := checkServiceBuild(fleet.Service{Name: "x", Tool: "no-such-tool"}); !errors.Is(err, fleet.ErrUnsignedServiceBuild) {
		t.Fatalf("unknown tool: %v", err)
	}
}

// An enabled unit the gate refuses is never handed to its provider: it
// latches failed with the policy message, and an explicit start or restart is
// refused at the control socket with the same message.
func TestMonitorRefusesUnsignedBuildBeforeActivation(t *testing.T) {
	bad := testUnit("matrix-chat", true)
	bad.Tool = "tuwunel"
	good := testUnit("web", true)
	set := &unitSet{units: map[string]fleet.Service{bad.Name: bad, good.Name: good}}
	prov := newFakeProvider(fleet.ServiceProviderProcess)
	gate := func(u fleet.Service) error {
		if u.Tool != "" {
			return errors.New("fleet: service \"" + u.Name + "\": " + fleet.ErrUnsignedServiceBuild.Error())
		}
		return nil
	}
	m, _, _ := testMonitorGated(t, set, prov, gate)
	st := waitState(t, m, "matrix-chat", service.Failed)
	if !strings.Contains(st.Reason, "bashy only runs bashy-signed server builds") {
		t.Fatalf("reason = %q", st.Reason)
	}
	waitState(t, m, "web", service.Active)
	for _, op := range []string{"start", "restart"} {
		r := ctl(t, m, ctlRequest{Op: op, Unit: "matrix-chat"})
		if r.OK || !strings.Contains(r.Error, "bashy only runs bashy-signed server builds") {
			t.Fatalf("%s: %+v", op, r)
		}
	}
	time.Sleep(30 * time.Millisecond)
	if n := prov.count(prov.acts, "matrix-chat"); n != 0 {
		t.Fatalf("unsigned unit activated %d times", n)
	}
}

// `bashy service enable|start|restart|run` refuse before contacting the monitor, so
// the refusal is the policy message, not "no monitor".
func TestServiceCLIRefusesUnsignedBuild(t *testing.T) {
	isolateFleet(t)
	t.Setenv("BASHY_SUPERVISORD_SOCKET", t.TempDir()+"/none.sock")
	tool := saveUnsignedServiceTool(t)
	if err := fleet.New().SaveService(fleet.Service{Name: "unsigned-service", Kind: fleet.KindService,
		Exec: []string{tool}, Tool: tool}); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"enable", "start", "restart", "run"} {
		cmd := newServiceCmd()
		cmd.SetArgs([]string{verb, "unsigned-service"})
		cmd.SetOut(new(strings.Builder))
		cmd.SetErr(new(strings.Builder))
		err := cmd.Execute()
		if !errors.Is(err, fleet.ErrUnsignedServiceBuild) {
			t.Fatalf("%s: err = %v", verb, err)
		}
	}
}

func saveUnsignedServiceTool(t *testing.T) string {
	t.Helper()
	const name = "unsigned-service-fixture"
	tool := fleet.Tool{Name: name, Kind: fleet.ToolKindCLI, CLI: fleet.ToolCLI{Managed: &fleet.ToolManaged{
		Version: "v0.0.0-test",
		Service: true,
	}}}
	if err := fleet.New().SaveTool(tool); err != nil {
		t.Fatal(err)
	}
	return name
}

func isolateFleet(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_FLEET_DIR", home+"/fleet")
	for _, e := range os.Environ() {
		if k, _, _ := strings.Cut(e, "="); strings.HasPrefix(k, "BASHY_") && strings.HasSuffix(k, "_PATH") {
			t.Setenv(k, "")
		}
		if k, _, _ := strings.Cut(e, "="); strings.HasPrefix(k, "BASHY_") && strings.HasSuffix(k, "S_DIR") {
			t.Setenv(k, "")
		}
	}
}

func testMonitorGated(t *testing.T, set *unitSet, prov service.Provider, gate func(fleet.Service) error) (*monitor, *eventLog, chan os.Signal) {
	t.Helper()
	saved := checkServiceBuildHook
	checkServiceBuildHook = gate
	t.Cleanup(func() { checkServiceBuildHook = saved })
	return testMonitor(t, set, prov)
}
