// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// Sprint: #302; Story: #964; Story-ID: c2f2a971dc77

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/broker"
	"github.com/qiangli/yoke/pkg/fleet"
)

// `bashy ollama serve` is the door, except for the door's own engine child.
func TestIsDoorServe(t *testing.T) {
	t.Setenv(broker.EngineModeEnv, "")
	os.Unsetenv(broker.EngineModeEnv)
	if !isDoorServe([]string{"serve"}) {
		t.Fatal("ollama serve should start the door")
	}
	if isDoorServe([]string{"list"}) || isDoorServe(nil) {
		t.Fatal("client verbs are not the door")
	}
	t.Setenv(broker.EngineModeEnv, "1")
	if isDoorServe([]string{"serve"}) {
		t.Fatal("the door's engine child must run the raw engine")
	}
}

// The door re-runs this binary as its engine: the argv is built lazily and
// never executed here (a test binary must not re-exec itself).
func TestDoorEngineArgv(t *testing.T) {
	argv := broker.EngineArgv()
	if len(argv) != 3 || argv[1] != "ollama" || argv[2] != "serve" {
		t.Fatalf("engine argv %v", argv)
	}
	if self := broker.SelfArgv(); len(self) != 3 || !strings.HasSuffix(strings.Join(self[1:], " "), "llm serve") {
		t.Fatalf("self argv %v", self)
	}
}

// `llm up/down --service` has no effect unless init() wired
// broker.SetServiceEnabled to a real service layer. This proves the seam is
// live and actually reaches pkg/fleet's Catalog.SetServiceEnabled on the
// "llm" unit, against a temp fleet root — never the real ~/.bashy.
func TestSetServiceEnabledWiresToFleetCatalog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_FLEET_DIR", filepath.Join(home, "fleet"))

	if broker.SetServiceEnabled == nil {
		t.Fatal("broker.SetServiceEnabled is nil after init")
	}

	before, ok := fleet.New().Service(broker.ServiceUnit)
	if !ok {
		t.Fatalf("no %q service unit in a fresh fleet catalog", broker.ServiceUnit)
	}
	want := !before.Enabled

	if err := broker.SetServiceEnabled(context.Background(), broker.ServiceUnit, want); err != nil {
		t.Fatalf("SetServiceEnabled: %v", err)
	}

	after, ok := fleet.New().Service(broker.ServiceUnit)
	if !ok || after.Enabled != want {
		t.Fatalf("service %+v, want enabled=%v", after, want)
	}
}

// `llm up --service` has no effect unless init() wired
// broker.EnsureSupervisordRunning to pkg/fleet's own EnsureSupervisordRunning.
// This proves the seam is live and actually reaches it, against a temp fleet
// root — never the real ~/.bashy, and never a real exec of this binary: a
// fake listener already answers the control socket, so
// fleet.EnsureSupervisordRunning takes its documented no-op short-circuit
// ("idempotent... argv is never touched") rather than spawning anything.
func TestEnsureSupervisordRunningWiresToFleet(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	// A short dir, not t.TempDir(): a unix socket path must fit sun_path
	// (~104 bytes on macOS/BSD), and t.TempDir() nests the test's own name.
	sockDir, err := os.MkdirTemp("", "ysd")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(sockDir)
	sock := filepath.Join(sockDir, "supervisord.sock")
	t.Setenv("BASHY_SUPERVISORD_SOCKET", sock)

	if broker.EnsureSupervisordRunning == nil {
		t.Fatal("broker.EnsureSupervisordRunning is nil after init")
	}

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	if err := broker.EnsureSupervisordRunning(context.Background()); err != nil {
		t.Fatalf("EnsureSupervisordRunning: %v", err)
	}
}

// `llm down --service` has no effect unless init() wired
// broker.StopServiceNow to the same control path `bashy service stop llm`
// uses (Sprint 455 F9): a direct signal to the door pid races the monitor's
// own bookkeeping of a disable and can restart the unit once before the
// disable catches up. This proves the seam reaches supervisord's "stop" op
// on its control socket, against a temp socket path — never the real
// ~/.bashy — and never a real exec of this binary.
func TestStopServiceNowWiresToSupervisordStop(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	sockDir, err := os.MkdirTemp("", "ysn")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(sockDir)
	sock := filepath.Join(sockDir, "supervisord.sock")
	t.Setenv("BASHY_SUPERVISORD_SOCKET", sock)

	if broker.StopServiceNow == nil {
		t.Fatal("broker.StopServiceNow is nil after init")
	}

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	got := make(chan ctlRequest, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var req ctlRequest
		if err := json.NewDecoder(c).Decode(&req); err != nil {
			return
		}
		got <- req
		_ = json.NewEncoder(c).Encode(ctlResponse{OK: true})
	}()

	if err := broker.StopServiceNow(context.Background(), broker.ServiceUnit); err != nil {
		t.Fatalf("StopServiceNow: %v", err)
	}

	select {
	case req := <-got:
		if req.Op != "stop" || req.Unit != broker.ServiceUnit {
			t.Fatalf("request = %+v, want op=stop unit=%q", req, broker.ServiceUnit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisord never received the stop request")
	}
}

// A shell mints a child of the view it inherited and exports only its view.
func TestShellStartupExportsView(t *testing.T) {
	t.Setenv(broker.SessionEnv, "s-000000000000~")
	ShellStartup()
	if !strings.HasPrefix(shellModelSession, "s-000000000000~") || shellModelSession == "s-000000000000~" {
		t.Fatalf("shell session %q", shellModelSession)
	}
	if got := os.Getenv(broker.SessionEnv); got != shellModelSession+"~" {
		t.Fatalf("exported %q, want the view of %q", got, shellModelSession)
	}
}
