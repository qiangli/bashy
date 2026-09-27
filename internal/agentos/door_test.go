// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// Sprint: #302; Story: #964; Story-ID: c2f2a971dc77

import (
	"os"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/broker"
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
