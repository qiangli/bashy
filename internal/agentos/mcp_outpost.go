package agentos

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/yoke/pkg/coopauth"
)

// mcpOutpostApp is the part of one outpost app registration the MCP fronts
// read: its signing key, its login posture and its loopback upstream.
type mcpOutpostApp struct {
	Name         string `json:"name"`
	SSOSecret    string `json:"sso_secret"`
	RequireLogin bool   `json:"require_login"`
	Scheme       string `json:"scheme"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	BasePath     string `json:"base_path"`
}

type mcpOutpostConfig struct {
	OAuth2Email string            `json:"oauth2_email"`
	ServiceSSO  map[string]string `json:"service_sso"`
	Apps        []mcpOutpostApp   `json:"apps"`
}

// Re-read the host's private app key on each request: outpost may provision it
// after this process starts, or rotate it without restarting the MCP server.
// No token is accepted on the command line or returned in diagnostics.
func readMCPOutpostConfig() (mcpOutpostConfig, bool) {
	var config mcpOutpostConfig
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return config, false
		}
		dir = filepath.Join(home, ".config")
	}
	path := filepath.Join(dir, "matrix", "agent.json")
	f, err := os.Open(path)
	if err != nil {
		return config, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || (runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0) {
		return config, false
	}
	if json.NewDecoder(f).Decode(&config) != nil {
		return config, false
	}
	return config, true
}

// signingKeys are the keys outpost signs an app's requests with: the service
// unit (service_sso) or an app published with `outpost apps add
// --trust-cloud-identity` (apps[].sso_secret). Both are host-private entries
// in the same 0600 file; nothing else is ever accepted.
func (c mcpOutpostConfig) signingKeys(app string) []string {
	var keys []string
	if key := strings.TrimSpace(c.ServiceSSO[app]); key != "" {
		keys = append(keys, key)
	}
	for _, entry := range c.Apps {
		if key := strings.TrimSpace(entry.SSOSecret); entry.Name == app && key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func (c mcpOutpostConfig) app(name string) (mcpOutpostApp, bool) {
	for _, entry := range c.Apps {
		if entry.Name == name {
			return entry, true
		}
	}
	return mcpOutpostApp{}, false
}

func mcpOutpostAuthenticate(r *http.Request) bool {
	if !coopauth.IsLoopbackAddr(r.RemoteAddr) {
		return false
	}
	config, ok := readMCPOutpostConfig()
	if !ok {
		return false
	}
	verified := false
	for _, key := range config.signingKeys("bashy-mcp") {
		if coopauth.VerifyOutpost(r, []byte(key)) {
			verified = true
			break
		}
	}
	if !verified {
		return false
	}
	// Trust only the signed identity, never the advisory cloud role. Reject
	// conflicting aliases rather than letting downstream code pick another user.
	caller := r.Header.Get(coopauth.HdrRemoteUser)
	if strings.TrimSpace(caller) == "" {
		return false
	}
	for _, key := range []string{coopauth.HdrRemoteEmail, "X-Periscope-User"} {
		if v := r.Header.Get(key); v != "" && v != caller {
			return false
		}
	}
	prefix := strings.TrimRight(r.Header.Get(coopauth.HdrForwardedPrefix), "/")
	return strings.HasSuffix(prefix, "/app/bashy-mcp")
}
