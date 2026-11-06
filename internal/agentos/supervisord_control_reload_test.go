package agentos

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

type reloadRecordingProvider struct {
	*fakeProvider
	declMu sync.Mutex
	images map[string]string
}

func (p *reloadRecordingProvider) Activate(ctx context.Context, u fleet.Service, rt service.Runtime) error {
	p.declMu.Lock()
	p.images[u.Name] = u.Settings["image"]
	p.declMu.Unlock()
	return p.fakeProvider.Activate(ctx, u, rt)
}
func (p *reloadRecordingProvider) image(name string) string {
	p.declMu.Lock()
	defer p.declMu.Unlock()
	return p.images[name]
}
func controlReloadFixture(t *testing.T, setup ...func(*monitor)) (*monitor, *fleet.Catalog, *reloadRecordingProvider, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("BASHY_FLEET_DIR", root)
	t.Setenv("BASHY_SERVICES_DIR", "")
	t.Setenv("BASHY_SERVICES_PATH", "")
	sock := shortSocket(t)
	t.Setenv("BASHY_SUPERVISORD_SOCKET", sock)
	cat := fleet.New(fleet.WithRoot(root), fleet.WithoutCloudOverlay(), fleet.WithBaselineFS(fstest.MapFS{}))
	prov := &reloadRecordingProvider{fakeProvider: newFakeProvider(fleet.ServiceProviderProcess), images: map[string]string{}}
	prov.confErr = service.ErrRestartRequired
	m := newMonitor()
	m.load = cat.Services
	m.providers = service.Providers{fleet.ServiceProviderProcess: prov}
	m.pollEvery = 0
	m.checkPorts = nil
	m.log = io.Discard
	m.events = func(service.Event) {}
	m.notify = func(service.Event) {}
	for _, apply := range setup {
		apply(m)
	}
	stop, e := m.serveControl(sock)
	if e != nil {
		t.Fatal(e)
	}
	sigs := make(chan os.Signal, 1)
	done := make(chan int, 1)
	go func() { done <- m.run(sigs) }()
	t.Cleanup(func() {
		sigs <- os.Interrupt
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("monitor shutdown timed out")
		}
		stop()
	})
	if _, e = ctlCall(sock, ctlRequest{Op: "status"}, time.Second); e != nil {
		t.Fatal(e)
	} // Initial load completed; polling remains disabled.
	return m, cat, prov, root
}
func invokeServiceControl(op, name string) (string, string, error) {
	cmd := newServiceCtlCmd(op)
	var out, errout bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errout)
	cmd.SetArgs([]string{name})
	e := cmd.Execute()
	return out.String(), errout.String(), e
}
func TestServiceControlReloadsCreatedAndChangedUnitsBeforePoll(t *testing.T) {
	m, cat, prov, _ := controlReloadFixture(t)
	unit := testUnit("fresh-preview-control", true)
	unit.Settings = map[string]string{"image": "first"}
	if e := cat.SaveService(unit); e != nil {
		t.Fatal(e)
	} // Direct library write, no CLI post-write reload.
	if _, _, e := invokeServiceControl("restart", unit.Name); e != nil {
		t.Fatal(e)
	}
	waitRunning(t, m, prov.fakeProvider, unit.Name, 1)
	if prov.image(unit.Name) != "first" {
		t.Fatal("new declaration was not activated")
	}
	bystander := testUnit("stable-bystander-control", true)
	if e := cat.SaveService(bystander); e != nil {
		t.Fatal(e)
	}
	if _, _, e := invokeServiceControl("start", bystander.Name); e != nil {
		t.Fatal(e)
	}
	waitRunning(t, m, prov.fakeProvider, bystander.Name, 1)
	stableStarts := prov.count(prov.acts, bystander.Name)
	unit.Settings = map[string]string{"image": "second"}
	if e := cat.SaveService(unit); e != nil {
		t.Fatal(e)
	}
	before := prov.count(prov.acts, unit.Name)
	if _, _, e := invokeServiceControl("restart", unit.Name); e != nil {
		t.Fatal(e)
	}
	waitRunning(t, m, prov.fakeProvider, unit.Name, before+1)
	if prov.image(unit.Name) != "second" {
		t.Fatalf("restarted cached old image: %q", prov.image(unit.Name))
	}
	if prov.count(prov.acts, bystander.Name) != stableStarts {
		t.Fatal("reload restarted unchanged bystander")
	}
	// Explicit start also loads a declaration that has never reached this monitor.
	off := testUnit("fresh-disabled-control", false)
	if e := cat.SaveService(off); e != nil {
		t.Fatal(e)
	}
	if _, _, e := invokeServiceControl("start", off.Name); e != nil {
		t.Fatal(e)
	}
	waitRunning(t, m, prov.fakeProvider, off.Name, 1)
	if _, _, e := invokeServiceControl("stop", unit.Name); e != nil {
		t.Fatal(e)
	}
	waitState(t, m, unit.Name, service.Inactive)
}
func TestServiceControlReloadErrorBlocksStartButPreservesEmergencyStop(t *testing.T) {
	m, cat, prov, root := controlReloadFixture(t)
	unit := testUnit("known-stop-control", true)
	if e := cat.SaveService(unit); e != nil {
		t.Fatal(e)
	}
	if _, _, e := invokeServiceControl("restart", unit.Name); e != nil {
		t.Fatal(e)
	}
	waitRunning(t, m, prov.fakeProvider, unit.Name, 1)
	if e := os.WriteFile(filepath.Join(root, "services", "broken.yaml"), []byte("name: [invalid\n"), 0600); e != nil {
		t.Fatal(e)
	}
	// The real monitor reports partial reload faults as OK=true + Error.
	response, e := ctlCall(supervisordSocket(), ctlRequest{Op: "reload"}, time.Second)
	if e != nil || !response.OK || response.Error == "" {
		t.Fatalf("actual reload fault not exercised: %+v %v", response, e)
	}
	before := prov.count(prov.acts, unit.Name)
	for _, op := range []string{"start", "restart"} {
		if _, _, e := invokeServiceControl(op, unit.Name); e == nil {
			t.Fatalf("%s ignored registry reload error", op)
		}
	}
	if prov.count(prov.acts, unit.Name) != before {
		t.Fatal("failed reload changed active process")
	}
	out, warnings, e := invokeServiceControl("stop", unit.Name)
	if e != nil || !strings.Contains(out, "stop requested") || !strings.Contains(warnings, "reload failed") {
		t.Fatalf("emergency stop blocked or silent: %q %q %v", out, warnings, e)
	}
	waitState(t, m, unit.Name, service.Inactive)
	if _, _, e = invokeServiceControl("stop", "missing-stop-control"); e == nil || !strings.Contains(e.Error(), "no service") || !strings.Contains(e.Error(), "broken:") {
		t.Fatalf("stop failures not joined with reload fault: %v", e)
	}
}
func TestServiceControlFreshDeclarationStillUsesBuildGate(t *testing.T) {
	_, cat, prov, _ := controlReloadFixture(t, func(m *monitor) {
		m.checkBuild = func(u fleet.Service) error {
			if u.Settings["image"] == "blocked" {
				return errors.New("fresh build gate refused")
			}
			return checkServiceBuild(u)
		}
	})
	unit := testUnit("fresh-gated-control", false)
	unit.Settings = map[string]string{"image": "blocked"}
	if e := cat.SaveService(unit); e != nil {
		t.Fatal(e)
	}
	if _, _, e := invokeServiceControl("start", unit.Name); e == nil || !strings.Contains(e.Error(), "fresh build gate refused") {
		t.Fatalf("fresh declaration bypassed build gate: %v", e)
	}
	if prov.count(prov.acts, unit.Name) != 0 {
		t.Fatal("refused build activated")
	}
}
