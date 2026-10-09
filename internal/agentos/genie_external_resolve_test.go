package agentos

// Sprint: #379; Story: #37; Story-ID: 5b537ed16256

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet/fleettest"
)

// The catalog names a credential by a STANDARD ref (`api_key_ref: zai`); the
// host binds it under its own vault name (`ZAI_API_KEY=@dragon-zai`). genie
// asked the vault for the bare ref and found nothing on every such host. It
// has to walk the binding. And a record with no context_length must say so,
// once, instead of silently running a 1M-context model on 20480 tokens.
func TestGenieExternalResolvesKeyThroughHostBindingAndWarnsOnMissingContext(t *testing.T) {
	fleettest.Ring(t)
	models := filepath.Join(t.TempDir(), "models")
	if err := os.MkdirAll(models, 0o755); err != nil {
		t.Fatal(err)
	}
	record := "name: zai-test\nprovider: openai-compat\nkind: api\nbase_url: https://example.invalid/v1/\napi_key_ref: zai\nmodel: glm-test\n"
	if err := os.WriteFile(filepath.Join(models, "zai-test.yaml"), []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_MODELS_PATH", models)

	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("BASHY_SECRETS_TOKEN", "fixture-token")
	for _, name := range []string{"zai", "ZAI", "ZAI_API_KEY", "ZAI_TOKEN", "ZAI_KEY"} {
		t.Setenv(name, "")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The vault knows the HOST name only; there is no secret called "zai".
		_, _ = w.Write([]byte(`{"secrets":[{"name":"dragon-zai","value":"vault-fixture"}]}`))
	}))
	defer server.Close()
	t.Setenv("BASHY_CLOUDBOX_URL", server.URL)
	if err := os.MkdirAll(filepath.Join(cfg, "bashy"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "bashy", "secrets.map"), []byte("ZAI_API_KEY=@dragon-zai\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var warned []string
	orig := genieWarnf
	genieWarnf = func(format string, args ...any) { warned = append(warned, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { genieWarnf = orig })
	genieContextWarned.Delete("zai-test")

	g, ok, err := resolveGenieExternal("zai-test")
	if err != nil || !ok {
		t.Fatalf("resolve: ok=%v err=%v", ok, err)
	}
	if g.Key != "vault-fixture" {
		t.Fatalf("key = %q; want the value bound as ZAI_API_KEY=@dragon-zai", g.Key)
	}
	if g.Context != genieContextFallback {
		t.Fatalf("context = %d; want the %d fallback for a record with none", g.Context, genieContextFallback)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "zai-test") || !strings.Contains(warned[0], "context_length") {
		t.Fatalf("fallback warning = %q; want one warning naming the model and context_length", warned)
	}
	if _, _, err := resolveGenieExternal("zai-test"); err != nil || len(warned) != 1 {
		t.Fatalf("second resolve: err=%v warnings=%d; want once per model", err, len(warned))
	}
}
