package agentos

import (
	"encoding/json"
	"testing"
)

// The one-shot calls llm and agent are typed tools in every profile that
// lists them, and each appears once (the typed tool replaces the argv verb).
func TestMCPTypedCallsAreListedTyped(t *testing.T) {
	for _, profile := range []string{"default", "all", "agent,llm"} {
		opts, err := mcpOptions("", profile)
		if err != nil {
			t.Fatalf("%s: %v", profile, err)
		}
		definitions, err := mcpToolDefinitions(opts)
		if err != nil {
			t.Fatalf("%s: %v", profile, err)
		}
		seen := map[string]int{}
		for _, def := range definitions {
			if def.Name != "llm" && def.Name != "agent" {
				continue
			}
			seen[def.Name]++
			raw, _ := json.Marshal(def.InputSchema)
			var schema struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			}
			_ = json.Unmarshal(raw, &schema)
			if _, argv := schema.Properties["args"]; argv || len(schema.Required) != 2 {
				t.Errorf("%s: %s is not typed: %s", profile, def.Name, raw)
			}
		}
		if seen["llm"] != 1 || seen["agent"] != 1 {
			t.Errorf("%s: typed calls listed %v", profile, seen)
		}
	}
}
