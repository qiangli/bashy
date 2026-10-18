package agentos

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const prefsMCPFixture = `{"budget_bytes":2048,"view":"json","defaults":{"*":{"api_token":"sk-LEAK","limit":3}}}`

func TestInstallAgentMCPNoPrefsByteIdentical(t *testing.T) {
	t.Setenv("BASHY_PREFS", "")
	if codexMCPEntry() != codexMCPServerEntry {
		t.Fatalf("codex entry changed: %q", codexMCPEntry())
	}
	if got := claudeMCPWriter().entry(false); got != "claude mcp add bashy -- bashy mcp serve\n" {
		t.Fatalf("claude: %q", got)
	}
	if strings.Contains(opencodeMCPEntry(), "environment") {
		t.Fatalf("opencode: %s", opencodeMCPEntry())
	}
}

func TestInstallAgentMCPProjectsPrefs(t *testing.T) {
	t.Setenv("BASHY_PREFS", prefsMCPFixture)
	for name, got := range map[string]string{
		"claude":   claudeMCPWriter().entry(false),
		"codex":    codexMCPEntry(),
		"opencode": opencodeMCPEntry(),
	} {
		if !strings.Contains(got, "BASHY_PREFS") || !strings.Contains(got, "budget_bytes") {
			t.Errorf("%s: prefs not projected: %s", name, got)
		}
		if strings.Contains(got, "sk-LEAK") || strings.Contains(got, "api_token") {
			t.Errorf("%s: secret leaked: %s", name, got)
		}
	}
	if got := claudeMCPWriter().entry(false); !strings.HasPrefix(got, "claude mcp add bashy -e BASHY_PREFS=") || !strings.HasSuffix(got, " -- bashy mcp serve\n") {
		t.Errorf("claude argv order: %s", got)
	}
}

func TestInstallAgentCodexMCPPrefsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_PREFS", prefsMCPFixture)
	w := codexMCPWriter()
	if _, err := w.install(false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(codexMCPConfigPath())
	if !strings.Contains(string(b), "BASHY_PREFS") || strings.Contains(string(b), "sk-LEAK") {
		t.Fatalf("config: %s", b)
	}
	if _, err := w.uninstall(false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(codexMCPConfigPath())
	if strings.Contains(string(b), "BASHY_PREFS") || strings.Contains(string(b), "mcp_servers") {
		t.Fatalf("not removed: %q", b)
	}
}

func TestInstallAgentOpencodeMCPPrefsWritten(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BASHY_PREFS", prefsMCPFixture)
	if _, err := opencodeMCPWriter().install(false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(opencodeConfigPath(false))
	var m map[string]map[string]map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	env, _ := m["mcp"]["bashy"]["environment"].(map[string]any)
	if v, _ := env["BASHY_PREFS"].(string); !strings.Contains(v, "budget_bytes") || strings.Contains(string(b), "sk-LEAK") {
		t.Fatalf("config: %s", b)
	}
}

func TestInstallAgentMCPMalformedPrefsFailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_PREFS", `{"budget`)
	if rc := dispatchInstallAgentMCP("codex", false, false, false, false, false); rc == 0 {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(codexMCPConfigPath()); err == nil {
		t.Fatal("config written despite malformed prefs")
	}
}
