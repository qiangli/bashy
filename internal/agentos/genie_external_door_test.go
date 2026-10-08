package agentos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/broker/door"
)

// A door-* model is served by this host's model door; nothing else starts
// it, and a turn against a door that is not listening stalls silently.
func TestGenieExternalStartsTheModelDoorOnlyForDoorModels(t *testing.T) {
	calls := 0
	failWith := error(nil)
	orig := genieEnsureDoor
	genieEnsureDoor = func(context.Context) error { calls++; return failWith }
	t.Cleanup(func() { genieEnsureDoor = orig })

	viaDoor := genieExternal{Name: "door-codex-gpt-5.5", BaseURL: fmt.Sprintf("http://127.0.0.1:%d/sticky/genie-gpt-5.5/v1", door.Port())}
	if err := viaDoor.prepare(context.Background()); err != nil || calls != 1 {
		t.Fatalf("door model: err=%v calls=%d; want the door started once", err, calls)
	}
	for _, base := range []string{"https://api.openai.com/v1", "http://127.0.0.1:11500/v1", "http://10.0.0.5:" + fmt.Sprint(door.Port()) + "/v1"} {
		if err := (genieExternal{Name: "x", BaseURL: base}).prepare(context.Background()); err != nil {
			t.Fatalf("%s: %v", base, err)
		}
	}
	if calls != 1 {
		t.Fatalf("non-door models started the door: calls=%d", calls)
	}
	failWith = errors.New("no self command")
	err := viaDoor.prepare(context.Background())
	if err == nil || !strings.Contains(err.Error(), "model door did not start") {
		t.Fatalf("door start failure not reported: %v", err)
	}
}
