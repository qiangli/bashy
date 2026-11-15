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

// Re-read the host's private app key on each request: outpost may provision it
// after this process starts, or rotate it without restarting the MCP server.
// No token is accepted on the command line or returned in diagnostics.
func mcpOutpostAuthenticate(r *http.Request) bool {
	if !coopauth.IsLoopbackAddr(r.RemoteAddr) {
		return false
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		dir = filepath.Join(home, ".config")
	}
	path := filepath.Join(dir, "matrix", "agent.json")
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || (runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0) {
		return false
	}
	var config struct {
		ServiceSSO map[string]string `json:"service_sso"`
	}
	if json.NewDecoder(f).Decode(&config) != nil {
		return false
	}
	if !coopauth.VerifyOutpost(r, []byte(config.ServiceSSO["bashy-mcp"])) {
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
