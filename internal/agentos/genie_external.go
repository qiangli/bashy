package agentos

// Sprint: #301; Story: #957; Story-ID: e5f503a3831a
//
// genie on an external model: `bashy genie -m NAME` where NAME is an API
// model in the model registry (`bashy model add|set`) runs genie against that
// provider's OpenAI-compatible endpoint instead of a local model — for hosts
// too small for any local model (a cheap cloud host) or when a stronger model
// is wanted. The registry entry names the endpoint (base_url), the
// provider-side id (model), the context (context_length) and the credential
// (api_key_ref: a vault name, or already in the environment). The key reaches
// the recipe through the environment only, never a command line.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

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
	key := ""
	for _, env := range append([]string{m.APIKeyRef}, secrets.CredentialEnvNames(m.APIKeyRef)...) {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			key = v
			break
		}
	}
	if key == "" {
		// The vault (cloudbox): the value is read into memory, not printed.
		var out bytes.Buffer
		cmd := exec.Command(bashySelfPath(), "secret", "get", m.APIKeyRef)
		cmd.Stdout, cmd.Stderr = &out, os.Stderr
		if err := cmd.Run(); err != nil {
			return genieExternal{}, true, fmt.Errorf("model %s: credential %s is neither in the environment nor readable from the vault (bashy secret get %s): %v", m.Name, m.APIKeyRef, m.APIKeyRef, err)
		}
		key = strings.TrimSpace(out.String())
	}
	if key == "" {
		return genieExternal{}, true, fmt.Errorf("model %s: credential %s is empty", m.Name, m.APIKeyRef)
	}
	ctx := m.ContextLength
	if ctx <= 0 {
		ctx = 32768
	}
	id := m.TargetFor("genie")
	if id == "" {
		id = m.Name
	}
	return genieExternal{Name: m.Name, ModelID: id, BaseURL: strings.TrimRight(m.BaseURL, "/"), Key: key, Context: ctx}, true, nil
}

// apply exports the external model to genie's recipe.
func (g genieExternal) apply() {
	os.Setenv("GENIE_EXTERNAL_MODEL", g.Name)
	os.Setenv("GENIE_MODEL_ID", g.ModelID)
	os.Setenv("GENIE_EXTERNAL_CONTEXT", strconv.FormatInt(g.Context, 10))
	os.Setenv("OPENAI_BASE_URL", g.BaseURL)
	os.Setenv("OPENAI_API_KEY", g.Key)
}
