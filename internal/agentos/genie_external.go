package agentos

// Sprint: #301; Story: #957; Story-ID: e5f503a3831a
//
// genie on an external model: `bashy genie -m NAME` where NAME is an API
// model in the model registry (`bashy model add|set`) runs genie against that
// provider's OpenAI-compatible endpoint instead of a local model — for hosts
// too small for any local model (a cheap cloud host) or when a stronger model
// is wanted. The registry entry names the endpoint (base_url), the
// provider-side id (model), the context (context_length) and the credential
// (api_key_ref: a STANDARD ref the host binds to its own vault name in
// secrets.map, or already in the environment). The key reaches the recipe
// through the environment only, never a command line.
//
// Sprint: #379; Story: #37; Story-ID: 5b537ed16256 — the credential walks the
// host's secrets.map binding (`zai` -> ZAI_API_KEY=@host-zai -> the vault),
// and a record with no context_length warns instead of silently starving.

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/qiangli/yoke/pkg/broker"
	"github.com/qiangli/yoke/pkg/broker/door"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/secrets"
)

// genieExternal is the recipe environment for an external model.
type genieExternal struct {
	Name, ModelID, BaseURL, Key string
	Context                     int64
}

// resolveGenieExternal looks name up in the model registry. ok is false for a
// name that is not a registered API model (a local Ollama tag, say), which
// keeps genie's own model server.
func resolveGenieExternal(name string) (genieExternal, bool, error) {
	m, found := fleet.New().Model(name)
	if !found || m.Kind != fleet.ModelKindAPI {
		return genieExternal{}, false, nil
	}
	if strings.TrimSpace(m.BaseURL) == "" {
		return genieExternal{}, true, fmt.Errorf("model %s has no base_url; set its OpenAI-compatible endpoint: bashy model set %s --set base_url=URL", m.Name, m.Name)
	}
	if m.APIKeyRef == "" {
		return genieExternal{}, true, fmt.Errorf("model %s has no api_key_ref; name its credential: bashy model set %s --set api_key_ref=NAME", m.Name, m.Name)
	}
	key, err := genieExternalKey(m)
	if err != nil {
		return genieExternal{}, true, err
	}
	ctx := m.ContextLength
	if ctx <= 0 {
		ctx = genieContextFallback
		genieWarnContextFallback(m.Name)
	}
	id := m.TargetFor("genie")
	if id == "" {
		id = m.Name
	}
	return genieExternal{Name: m.Name, ModelID: id, BaseURL: strings.TrimRight(m.BaseURL, "/"), Key: key, Context: ctx}, true, nil
}

// genieContextFallback is the context genie assumes for an API model whose
// record declares none. It is small on purpose — a guess that is too LARGE
// makes the provider reject the request — but small means STARVED: genie turns
// it into a 20480-token budget after its output and compaction reserves, and a
// 1M-context model (glm-5.3) lost its tool output within a few turns while
// nothing said why. So the fallback is never silent: it warns once per model.
const genieContextFallback = 32768

var (
	// genieWarnf is where the fallback warning goes; a seam so tests can read it.
	genieWarnf = func(format string, args ...any) { fmt.Fprintf(os.Stderr, format, args...) }
	// genieContextWarned holds the models already warned about, once per process.
	genieContextWarned sync.Map
)

func genieWarnContextFallback(model string) {
	if _, dup := genieContextWarned.LoadOrStore(model, struct{}{}); dup {
		return
	}
	genieWarnf("genie: model %s declares no context_length; assuming %d tokens, which starves a large model — declare the vendor's context window: bashy model set %s --set context_length=N\n", model, genieContextFallback, model)
}

// genieExternalKey resolves the model's credential on THIS host, in order:
//
//  1. the environment — the ref itself, then its conventional names
//     (ZAI_API_KEY, ZAI_TOKEN, …);
//  2. the host's secrets.map binding for one of those names, rendered through
//     the vault or its offline cache (secrets.ResolveAgentKey). This is the
//     step that was missing: the catalog ref is STANDARD (`zai`, the same in
//     every copy of the catalog) while the vault name is PER HOST
//     (`host-zai`), and only the binding joins the two — so asking the vault
//     for the bare ref found nothing on every host set up from the template;
//  3. the vault by the bare ref, for a host that named its secret after the
//     ref directly.
//
// The value is read into memory, never printed.
func genieExternalKey(m fleet.Model) (string, error) {
	if v := strings.TrimSpace(os.Getenv(m.APIKeyRef)); v != "" {
		return v, nil
	}
	if kv, ok := secrets.ResolveAgentKey(os.Environ(), m.APIKeyRef); ok {
		if _, v, found := strings.Cut(kv, "="); found && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), nil
		}
	}
	var out bytes.Buffer
	cmd := exec.Command(bashySelfPath(), "secret", "get", m.APIKeyRef)
	cmd.Stdout, cmd.Stderr = &out, os.Stderr
	if err := cmd.Run(); err != nil {
		names := secrets.CredentialEnvNames(m.APIKeyRef)
		return "", fmt.Errorf("model %s: credential %s is not in the environment, not bound in secrets.map, and not a vault secret by that name (bashy secret get %s): %v — bind it on this host: %s=@<vault secret name> in ~/.config/bashy/secrets.map", m.Name, m.APIKeyRef, m.APIKeyRef, err, names[0])
	}
	key := strings.TrimSpace(out.String())
	if key == "" {
		return "", fmt.Errorf("model %s: credential %s is empty", m.Name, m.APIKeyRef)
	}
	return key, nil
}

// apply exports the external model to genie's recipe.
func (g genieExternal) apply() {
	os.Setenv("GENIE_EXTERNAL_MODEL", g.Name)
	os.Setenv("GENIE_MODEL_ID", g.ModelID)
	os.Setenv("GENIE_EXTERNAL_CONTEXT", strconv.FormatInt(g.Context, 10))
	os.Setenv("OPENAI_BASE_URL", g.BaseURL)
	os.Setenv("OPENAI_API_KEY", g.Key)
}

// genieEnsureDoor starts the host's model door; a seam so tests never start a
// real one.
var genieEnsureDoor = broker.EnsureUp

// viaDoor reports whether the model is served by this host's model door
// (the door-* registry entries). Nothing else starts the door for such a
// model: genie's local-model recipe runs `llm up`, the external path does
// not, and a request to a door that is not listening stalls the turn at
// llm.requested with no output.
func (g genieExternal) viaDoor() bool {
	u, err := url.Parse(g.BaseURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return (host == "127.0.0.1" || host == "localhost") && u.Port() == strconv.Itoa(door.Port())
}

// prepare makes the model reachable before genie's turn starts.
func (g genieExternal) prepare(ctx context.Context) error {
	if !g.viaDoor() {
		return nil
	}
	if err := genieEnsureDoor(ctx); err != nil {
		return fmt.Errorf("the model door did not start for %s: %w", g.Name, err)
	}
	return nil
}
