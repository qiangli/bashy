package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/fleet"
)

func TestProvidersCLIAndMCPDiscovery(t *testing.T) {
	ringDir(t)
	for _, name := range []string{"plan", "image"} {
		writeRecord(t, fleet.Command{Name: name, Script: "true", Effects: []string{"read"}})
	}
	request := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(request, []byte(`{"prompt":"draw a circle"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if code := runProviders([]string{"select", "plan", "--provider", "claude:plan", "--json", request}, &out, &errout); code != 0 {
		t.Fatalf("select: %d %s", code, errout.String())
	}
	var selected struct {
		Name  string `json:"name"`
		Route string `json:"route"`
	}
	if err := json.Unmarshal(out.Bytes(), &selected); err != nil || selected.Name != "claude:plan" || selected.Route != "vendor" {
		t.Fatalf("selected = %+v, %v", selected, err)
	}
	names := map[string]bool{}
	for _, c := range mcpRegisteredCommands() {
		names[c.Name] = true
	}
	for _, name := range []string{"plan", "image"} {
		if !names[name] {
			t.Fatalf("MCP missing %s", name)
		}
	}
	definitions, err := mcpToolDefinitions(yokemcp.Options{Registered: mcpRegisteredCommands})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"plan", "image"} {
		found := false
		for _, d := range definitions {
			if d.Name == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("MCP definitions missing %s: %d tools", name, len(definitions))
		}
	}
}
