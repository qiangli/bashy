package agentos

import (
	"context"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
)

func TestResolveUnitRuntimeDeclaredSecretEnv(t *testing.T) {
	const name = "S418_SERVICE_GATE_TOKEN"
	t.Setenv(name, "bound-on-host")
	u := fleet.Service{Secrets: []fleet.ServiceSecret{{Env: name, Ref: "s418/service-gate"}}}
	rt, err := resolveUnitRuntime(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if got := rt.Env[name]; got != "bound-on-host" {
		t.Fatalf("resolved secret = %q", got)
	}
}

func TestResolveUnitRuntimeUnboundSecretRefused(t *testing.T) {
	const name = "S418_SERVICE_UNBOUND_TOKEN"
	t.Setenv(name, "")
	u := fleet.Service{Secrets: []fleet.ServiceSecret{{Env: name, Ref: "s418/unbound-secret"}}}
	_, err := resolveUnitRuntime(context.Background(), u)
	if err == nil || !strings.Contains(err.Error(), "not bound on this host") {
		t.Fatalf("unbound secret error = %v", err)
	}
}
