// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func mustFleetExamplesDir(t *testing.T) string {
	t.Helper()
	root, ok := findBashySourceRoot(mustGetwd())
	if !ok {
		t.Fatal("could not find bashy source root")
	}
	fleetDir := filepath.Join(root, "examples", "fleet")
	if fi, err := os.Stat(fleetDir); err != nil || !fi.IsDir() {
		t.Fatalf("examples/fleet dir not found at %s: %v", fleetDir, err)
	}
	return fleetDir
}

func executeFleetCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, string, error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func setupIsolatedFleetStore(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("BASHY_FLEET_DIR", tmp)
	t.Setenv("BASHY_FLEET_SEEDS", "off")
	return tmp
}

// TestFleetExamplesHeadersAndSecretsGuard asserts:
// - Every tools/*.yaml header comment has `# status:` and `# add:` lines.
// - No real hostnames or tokens in any fleet yaml file (regex guard: no /Users/, no .local hostnames, no sk-..., no long hex secrets).
func TestFleetExamplesHeadersAndSecretsGuard(t *testing.T) {
	fleetDir := mustFleetExamplesDir(t)

	toolFiles, err := filepath.Glob(filepath.Join(fleetDir, "tools", "*.yaml"))
	if err != nil || len(toolFiles) == 0 {
		t.Fatalf("no tool files found under %s/tools: %v", fleetDir, err)
	}
	if len(toolFiles) != 15 {
		t.Errorf("expected 15 tool files, found %d", len(toolFiles))
	}

	modelFiles, err := filepath.Glob(filepath.Join(fleetDir, "models", "*.yaml"))
	if err != nil || len(modelFiles) == 0 {
		t.Fatalf("no model files found under %s/models: %v", fleetDir, err)
	}
	if len(modelFiles) != 3 {
		t.Errorf("expected 3 model files, found %d", len(modelFiles))
	}

	agentFiles, err := filepath.Glob(filepath.Join(fleetDir, "agents", "*.yaml"))
	if err != nil || len(agentFiles) == 0 {
		t.Fatalf("no agent files found under %s/agents: %v", fleetDir, err)
	}
	if len(agentFiles) != 1 {
		t.Errorf("expected 1 agent file, found %d", len(agentFiles))
	}

	// 1. Assert every tools/*.yaml header comment has `# status:` and `# add:` lines.
	for _, tf := range toolFiles {
		base := filepath.Base(tf)
		data, err := os.ReadFile(tf)
		if err != nil {
			t.Fatalf("%s: read file: %v", base, err)
		}
		content := string(data)
		if !strings.Contains(content, "# status:") {
			t.Errorf("%s: missing `# status:` line in header comment", base)
		}
		if !strings.Contains(content, "# add:") {
			t.Errorf("%s: missing `# add:` line in header comment", base)
		}
	}

	// 2. Secret & hostname guard across all fleet yaml files:
	// Simple regex guard: no /Users/, no .local hostnames, no sk-..., no long hex secrets.
	allFiles := append(append(toolFiles, modelFiles...), agentFiles...)
	reUsers := regexp.MustCompile(`/Users/`)
	reLocal := regexp.MustCompile(`\b[a-zA-Z0-9_\-\.]+\.local\b`)
	reSKToken := regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{10,}`)
	reHexSecret := regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)

	for _, f := range allFiles {
		rel, _ := filepath.Rel(fleetDir, f)
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: read file: %v", rel, err)
		}
		content := string(data)

		if match := reUsers.FindString(content); match != "" {
			t.Errorf("%s: contains real path pattern %q", rel, match)
		}
		if match := reLocal.FindString(content); match != "" {
			t.Errorf("%s: contains .local hostname %q", rel, match)
		}
		if match := reSKToken.FindString(content); match != "" {
			t.Errorf("%s: contains API key token pattern %q", rel, match)
		}
		if match := reHexSecret.FindString(content); match != "" {
			t.Errorf("%s: contains long hex secret pattern %q", rel, match)
		}
	}
}

// fleetNounDriver defines operations for a fleet noun (tool, model, agent)
type fleetNounDriver struct {
	noun       string
	subDir     string
	newCmd     func() *cobra.Command
	parseNames func(base string, data []byte) ([]string, error)
	preAdd     func(t *testing.T, fleetDir string)
}

func runFleetNounLifecycleTest(t *testing.T, driver fleetNounDriver) {
	t.Helper()
	fleetDir := mustFleetExamplesDir(t)
	files, err := filepath.Glob(filepath.Join(fleetDir, driver.subDir, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no %s files found under %s/%s: %v", driver.noun, fleetDir, driver.subDir, err)
	}

	for _, file := range files {
		fileName := filepath.Base(file)
		t.Run(fileName, func(t *testing.T) {
			setupIsolatedFleetStore(t)

			if driver.preAdd != nil {
				driver.preAdd(t, fleetDir)
			}

			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			names, err := driver.parseNames(strings.TrimSuffix(fileName, ".yaml"), data)
			if err != nil {
				t.Fatalf("parse %s in %s: %v", driver.noun, fileName, err)
			}
			if len(names) == 0 {
				t.Fatalf("no %s entries found in %s", driver.noun, fileName)
			}

			// 1. add --force (with --force where a name collides with a built-in)
			addOut, stderr, err := executeFleetCmd(t, driver.newCmd(), "add", "--force", file)
			if err != nil {
				t.Fatalf("%s add --force %s failed: %v (stderr: %s)", driver.noun, file, err, stderr)
			}
			for _, name := range names {
				if !strings.Contains(addOut, name) {
					t.Errorf("%s add output %q did not mention %q", driver.noun, addOut, name)
				}
			}

			for _, name := range names {
				// 2. show --json (round-trips: re-adding the shown record yields the same show output)
				json1, stderr, err := executeFleetCmd(t, driver.newCmd(), "show", "--json", name)
				if err != nil {
					t.Fatalf("%s show --json %s failed: %v (stderr: %s)", driver.noun, name, err, stderr)
				}
				var obj map[string]any
				if err := json.Unmarshal([]byte(json1), &obj); err != nil {
					t.Fatalf("%s show --json output is invalid JSON: %v\n%s", driver.noun, err, json1)
				}
				if obj["name"] != name {
					t.Errorf("%s show --json name = %v, want %q", driver.noun, obj["name"], name)
				}

				// Re-adding the shown record yields the same show output
				tempJSON := filepath.Join(t.TempDir(), fmt.Sprintf("roundtrip-%s.json", name))
				if err := os.WriteFile(tempJSON, []byte(json1), 0o600); err != nil {
					t.Fatalf("write temp json: %v", err)
				}
				_, stderr, err = executeFleetCmd(t, driver.newCmd(), "add", "--force", tempJSON)
				if err != nil {
					t.Fatalf("re-add %s from json failed: %v (stderr: %s)", driver.noun, err, stderr)
				}
				json2, stderr, err := executeFleetCmd(t, driver.newCmd(), "show", "--json", name)
				if err != nil {
					t.Fatalf("%s show --json after re-add failed: %v (stderr: %s)", driver.noun, err, stderr)
				}
				if json1 != json2 {
					t.Errorf("%s show --json did not round-trip identically:\nfirst:\n%s\nsecond:\n%s", driver.noun, json1, json2)
				}

				// 3. list --custom contains it
				listOut, stderr, err := executeFleetCmd(t, driver.newCmd(), "list", "--custom", "--json")
				if err != nil {
					t.Fatalf("%s list --custom --json failed: %v (stderr: %s)", driver.noun, err, stderr)
				}
				var listRows []map[string]any
				if err := json.Unmarshal([]byte(listOut), &listRows); err != nil {
					t.Fatalf("%s list JSON unmarshal failed: %v\n%s", driver.noun, err, listOut)
				}
				found := false
				for _, row := range listRows {
					if row["name"] == name {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s list --custom does not contain %q; list output: %s", driver.noun, name, listOut)
				}

				// 4. verify reports a missing binary as not installed (never an error that aborts)
				verifyJSON, stderr, err := executeFleetCmd(t, driver.newCmd(), "verify", "--json", name)
				if err != nil {
					t.Fatalf("%s verify --json %s failed: %v (stderr: %s)", driver.noun, name, err, stderr)
				}
				var checks []fleet.Check
				if err := json.Unmarshal([]byte(verifyJSON), &checks); err != nil {
					t.Fatalf("%s verify --json output is invalid JSON: %v\n%s", driver.noun, err, verifyJSON)
				}
				if len(checks) == 0 {
					t.Fatalf("%s verify --json returned no checks for %s", driver.noun, name)
				}
				chk := checks[0]
				if chk.Name != name {
					t.Errorf("%s verify check name = %q, want %q", driver.noun, chk.Name, name)
				}

				// If the binary is missing on this host, verify reports it as not installed
				if driver.noun == "tool" && !chk.OK {
					if !strings.Contains(chk.Reason, "not installed") {
						t.Errorf("tool verify %s reason = %q; want 'not installed'", name, chk.Reason)
					}
				}

				// Also run text verify — confirms it produces diagnostic output without panicking/aborting
				verifyText, _, _ := executeFleetCmd(t, driver.newCmd(), "verify", name)
				if verifyText == "" {
					t.Errorf("%s verify text output unexpectedly empty for %s", driver.noun, name)
				}

				// 5. rm removes it
				_, stderr, err = executeFleetCmd(t, driver.newCmd(), "rm", name)
				if err != nil {
					t.Fatalf("%s rm %s failed: %v (stderr: %s)", driver.noun, name, err, stderr)
				}
				postListOut, _, _ := executeFleetCmd(t, driver.newCmd(), "list", "--custom", "--json")
				var postListRows []map[string]any
				_ = json.Unmarshal([]byte(postListOut), &postListRows)
				for _, row := range postListRows {
					if row["name"] == name {
						t.Errorf("%s %s still present in list --custom after rm", driver.noun, name)
					}
				}
			}
		})
	}
}

// TestFleetExamplesToolsOfflineRegistryLifecycle tests every file under examples/fleet/tools.
func TestFleetExamplesToolsOfflineRegistryLifecycle(t *testing.T) {
	runFleetNounLifecycleTest(t, fleetNounDriver{
		noun:   "tool",
		subDir: "tools",
		newCmd: func() *cobra.Command { return fleet.NewToolsCmd() },
		parseNames: func(base string, data []byte) ([]string, error) {
			parsed, err := fleet.ParseTool(base, data, nil)
			if err != nil {
				return nil, err
			}
			return []string{parsed.Name}, nil
		},
	})
}

// TestFleetExamplesModelsOfflineRegistryLifecycle tests every file under examples/fleet/models.
func TestFleetExamplesModelsOfflineRegistryLifecycle(t *testing.T) {
	runFleetNounLifecycleTest(t, fleetNounDriver{
		noun:   "model",
		subDir: "models",
		newCmd: func() *cobra.Command { return fleet.NewModelsCmd() },
		parseNames: func(base string, data []byte) ([]string, error) {
			parsed, err := fleet.ParseModel(base, data, nil)
			if err != nil {
				return nil, err
			}
			return []string{parsed.Name}, nil
		},
	})
}

// TestFleetExamplesAgentsOfflineRegistryLifecycle tests every file under examples/fleet/agents.
func TestFleetExamplesAgentsOfflineRegistryLifecycle(t *testing.T) {
	runFleetNounLifecycleTest(t, fleetNounDriver{
		noun:   "agent",
		subDir: "agents",
		newCmd: func() *cobra.Command { return fleet.NewAgentsCmd() },
		parseNames: func(base string, data []byte) ([]string, error) {
			parsed, err := fleet.ParseAgentFile(base, data, nil)
			if err != nil {
				return nil, err
			}
			var names []string
			for _, a := range parsed.Agents {
				names = append(names, a.Name)
			}
			return names, nil
		},
		preAdd: func(t *testing.T, fleetDir string) {
			// Pre-populate dependencies (tool qwen-code and model ollama-qwen3.5-9b) for binding resolution
			toolFile := filepath.Join(fleetDir, "tools", "qwen-code.yaml")
			if _, stderr, err := executeFleetCmd(t, fleet.NewToolsCmd(), "add", "--force", toolFile); err != nil {
				t.Fatalf("pre-add tool qwen-code: %v (stderr: %s)", err, stderr)
			}
			modelFile := filepath.Join(fleetDir, "models", "ollama-qwen3.5-9b.yaml")
			if _, stderr, err := executeFleetCmd(t, fleet.NewModelsCmd(), "add", "--force", modelFile); err != nil {
				t.Fatalf("pre-add model ollama-qwen3.5-9b: %v (stderr: %s)", err, stderr)
			}
		},
	})
}

// TestFleetExamplesNegativeCases covers negative cases:
// 1. An agent bound to an unknown model is refused or reported.
// 2. A model with an api_key_ref has no inline key.
func TestFleetExamplesNegativeCases(t *testing.T) {
	fleetDir := mustFleetExamplesDir(t)

	// Negative case 1: Agent bound to unknown model
	t.Run("AgentWithUnknownModelReported", func(t *testing.T) {
		setupIsolatedFleetStore(t)

		// Add a valid tool first
		qwenTool := filepath.Join(fleetDir, "tools", "qwen-code.yaml")
		if _, stderr, err := executeFleetCmd(t, fleet.NewToolsCmd(), "add", "--force", qwenTool); err != nil {
			t.Fatalf("add tool: %v (stderr: %s)", err, stderr)
		}

		// Mint an agent bound to an unknown model
		agentName := "agent-unknown-model"
		unknownModel := "nonexistent-model-xyz-999"
		_, _, _ = executeFleetCmd(t, fleet.NewAgentsCmd(), "add", "--force", agentName, "--tool", "qwen-code", "--model", unknownModel)

		cat := fleet.New()
		// Binding resolution should fail
		_, _, _, err := cat.Binding(agentName)
		if err == nil {
			t.Errorf("cat.Binding(%s) succeeded; expected error for unknown model %s", agentName, unknownModel)
		} else if !strings.Contains(err.Error(), unknownModel) && !strings.Contains(err.Error(), "not registered") {
			t.Errorf("cat.Binding(%s) error = %v; want mention of %s or not registered", agentName, err, unknownModel)
		}

		// VerifyAgent should report not ok
		chk := cat.VerifyAgent(agentName, fleet.Probes(nil))
		if chk.OK {
			t.Errorf("cat.VerifyAgent(%s) reported OK; expected failure for unknown model", agentName)
		}
		if !strings.Contains(chk.Reason, unknownModel) && !strings.Contains(chk.Reason, "not registered") {
			t.Errorf("cat.VerifyAgent(%s) reason = %q; want mention of missing model", agentName, chk.Reason)
		}

		// agent show reports resolves: no
		showOut, _, err := executeFleetCmd(t, fleet.NewAgentsCmd(), "show", agentName)
		if err != nil {
			t.Fatalf("agent show %s: %v", agentName, err)
		}
		if !strings.Contains(showOut, "resolves: no") {
			t.Errorf("agent show output %q does not contain 'resolves: no'", showOut)
		}
	})

	// Negative case 2: Model with api_key_ref has no inline key
	t.Run("ModelWithAPIKeyRefHasNoInlineKey", func(t *testing.T) {
		modelFiles, err := filepath.Glob(filepath.Join(fleetDir, "models", "*.yaml"))
		if err != nil || len(modelFiles) == 0 {
			t.Fatalf("no model files: %v", err)
		}

		forbiddenKeyFields := []string{"api_key", "key", "token", "secret", "api_token", "auth_token", "inline_key", "password"}

		for _, mf := range modelFiles {
			base := filepath.Base(mf)
			data, err := os.ReadFile(mf)
			if err != nil {
				t.Fatalf("read %s: %v", base, err)
			}
			var m map[string]any
			if err := yaml.Unmarshal(data, &m); err != nil {
				t.Fatalf("unmarshal %s: %v", base, err)
			}

			keyRef, _ := m["api_key_ref"].(string)
			if keyRef != "" {
				// Assert no forbidden inline key fields exist
				for _, forbidden := range forbiddenKeyFields {
					if val, ok := m[forbidden]; ok {
						t.Errorf("%s declares api_key_ref %q but also has inline key field %q: %v", base, keyRef, forbidden, val)
					}
				}

				// Assert key_ref is a reference name (e.g. zai, door, ollama) and not an inline secret token
				if strings.HasPrefix(keyRef, "sk-") || len(keyRef) > 30 {
					t.Errorf("%s api_key_ref %q appears to be an inline secret rather than a reference name", base, keyRef)
				}
			}
		}
	})
}
