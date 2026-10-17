//go:build bashy_engines

package agentos

// The managed server tokenizes prompts in-process even though inference runs
// in the signed runner. Register its pure-Go model/tokenizer implementations;
// importing the server alone does not import the runner's model registry.
import _ "github.com/ollama/ollama/model/models"
