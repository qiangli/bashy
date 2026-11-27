package agentos

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/shape"
	shapestate "github.com/qiangli/yoke/pkg/shape/state"
)

// shape ledger on the host reads the decision log as the operator: the
// problem key and every principal are shown, text by default, --json on ask.
func TestShapeLedgerLocalReadsTheDecisionLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	oldLookup, oldOptions := shapeAppLookup, shapeServiceOptions
	shapeServiceOptions = func(string) shape.Options { return shape.ProductionOptions(home) }
	shapeAppLookup = func(string) (shape.App, error) {
		return shape.App{Name: "site", Owner: "owner@example.test", Shaping: &fleet.AppShaping{Kind: "web"}}, nil
	}
	t.Cleanup(func() { shapeAppLookup, shapeServiceOptions = oldLookup, oldOptions })
	for _, d := range []shapestate.AutoDecision{
		{App: "site", Event: "trigger", Result: "wish", Problem: "todo:T-42", Principal: "cloud:c@example.test", Line: "owner@example.test", Detail: "0123456789abcdef add a footer"},
		{App: "site", Event: "activation", Result: "activated", Problem: "todo:T-42", Principal: "cloud:c@example.test", Line: "owner@example.test", Agent: "stub", Why: "footer was missing", Rev: "rev1", Detail: "0123456789abcdef rev1"},
		{App: "site", Event: "undo", Result: "activated", Line: "owner@example.test", By: "person", Rev: "rev1", Detail: "not yet"},
	} {
		if err := shapestate.LogAuto(home, d); err != nil {
			t.Fatal(err)
		}
	}
	result, err := shapeCLIExecute(context.Background(), shapeCLIRequest{Verb: "ledger", App: "site", User: "anyone"})
	if err != nil {
		t.Fatal(err)
	}
	l, ok := result.(shape.Ledger)
	if !ok || len(l.Problems) != 1 || l.Problems[0].Key != "todo:T-42" || !l.Problems[0].Open ||
		l.Problems[0].Attempts[0].Standing != shape.StandingUndone || l.Problems[0].Attempts[0].Principal != "cloud:c@example.test" {
		t.Fatalf("ledger = %+v", result)
	}
	var text, js bytes.Buffer
	if err := writeShapeResult(&text, result, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"open  todo:T-42  add a footer", "undone", "footer was missing", "person not yet"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("text lacks %q:\n%s", want, text.String())
		}
	}
	if err := writeShapeResult(&js, result, true); err != nil || !strings.Contains(js.String(), `"standing": "undone"`) {
		t.Errorf("json: %v\n%s", err, js.String())
	}
	// At the base revision the later undo does not count.
	at, err := shapeCLIExecute(context.Background(), shapeCLIRequest{Verb: "ledger", App: "site", User: "anyone", ID: "rev1", MaxBytes: -1})
	if err != nil || at.(shape.Ledger).Problems[0].Attempts[0].Standing != shape.StandingActive {
		t.Errorf("at rev1: %v %+v", err, at)
	}
}

func TestShapeLedgerRemoteAsksTheConsoleRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/shape/site/ledger" || r.URL.Query().Get("base") != "rev1" || r.URL.Query().Get("max") != "512" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`{"app":"site","problems":[{"key":"p-0011223344","open":true,"attempts":[{"standing":"pending"}]}]}`))
	}))
	defer server.Close()
	cookie := filepath.Join(t.TempDir(), "cookie")
	if err := os.WriteFile(cookie, []byte("synthetic-session"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := executeShapeRemote(context.Background(), shapeCLIRequest{Verb: "ledger", App: "site", ID: "rev1", MaxBytes: 512, Remote: server.URL, CookieFile: cookie})
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := result.(shape.Ledger); !ok || len(l.Problems) != 1 || l.Problems[0].Key != "p-0011223344" {
		t.Errorf("ledger = %+v", result)
	}
}
