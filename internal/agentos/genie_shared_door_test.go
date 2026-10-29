package agentos

import (
	"context"
	"fmt"
	"github.com/qiangli/yoke/pkg/broker"
	"github.com/qiangli/yoke/pkg/broker/door"
	"github.com/qiangli/yoke/pkg/fleet"
	"testing"
)

func TestGenieSharedDoorSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BASHY_HOME", "")
	t.Setenv("BASHY_FLEET_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("DRAGON_DOOR_API_KEY", "stale")
	// An old manually registered alias must not strand its copied credential.
	if err := fleet.New().SaveModel(fleet.Model{Name: "dragon-door-codex-gpt6-sol", Kind: fleet.ModelKindAPI, Provider: "openai-compat", BaseURL: "https://stale.test/v1", APIKeyRef: "dragon-door"}); err != nil {
		t.Fatal(err)
	}
	grant := broker.DoorGrant{Schema: broker.GrantSchemaVersion, Door: "dragon", Peer: "elephant", CloudboxURL: "https://example.test/app/llm"}
	for _, key := range []string{"bpeer_first", "bpeer_rotated"} {
		grant.Credential = key
		if err := broker.SaveDoorGrant(grant); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"codex-gpt6-sol@dragon", "dragon-door-codex-gpt6-sol"} {
			g, ok, err := resolveGenieExternal(name)
			if err != nil || !ok {
				t.Fatalf("%s: ok=%v err=%v", name, ok, err)
			}
			if g.ModelID != "codex-gpt6-sol" || g.Key != key || g.BaseURL != grant.CloudboxURL+"/k/"+key+"/v1" || g.Context <= 0 {
				t.Fatal("wrong endpoint, model, context or stale key")
			}
		}
	}
	if _, ok, err := resolveGenieExternal("m@missing"); !ok || err == nil {
		t.Fatalf("missing door fell through: ok=%v err=%v", ok, err)
	}
}

func TestGenieSharedDoorNeverStartsLocalServer(t *testing.T) {
	old := genieEnsureDoor
	t.Cleanup(func() { genieEnsureDoor = old })
	genieEnsureDoor = func(context.Context) error { t.Fatal("started local server for shared door"); return nil }
	g := genieExternal{SharedDoor: true, BaseURL: fmt.Sprintf("http://127.0.0.1:%d/v1", door.Port())}
	if err := g.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
}
