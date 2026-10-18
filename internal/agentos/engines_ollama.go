//go:build bashy_engines

package agentos

import (
	"os"

	"github.com/qiangli/yoke/external/ollama"
)

// dispatchOllamaEngine uses the managed server and signed runner on every OS.
func dispatchOllamaEngine() {
	if blocked, msg := ollamaCloudGate(os.Args[2:]); blocked {
		os.Stderr.WriteString(msg)
		dispatchExit(2)
	}
	defaultOllamaNewEngine()
	cmd := ollama.NewManagedOllamaCmd()
	cmd.SetArgs(os.Args[2:])
	if err := cmd.Execute(); err != nil {
		dispatchExit(1)
	}
	dispatchExit(0)
}

func probeOllamaEngine() ollamaEngineProbe {
	self, _ := os.Executable()
	return ollamaEngineProbe{Build: "embedded", Binary: self}
}

// ollamaNeedsNewEngine is set by the !cgo build: without cgo the engine's
// compatibility tokenizer is absent ("llama model loading unavailable: built
// without CGo"), so only Ollama's pure-Go engine can load a model.
var ollamaNeedsNewEngine bool

// defaultOllamaNewEngine selects the pure-Go engine on a cgo-off build unless
// the operator chose explicitly (OLLAMA_NEW_ENGINE is read per request).
func defaultOllamaNewEngine() {
	if ollamaNeedsNewEngine {
		if _, set := os.LookupEnv("OLLAMA_NEW_ENGINE"); !set {
			os.Setenv("OLLAMA_NEW_ENGINE", "true")
		}
	}
}

func init() { cobraOptionalVerbs["ollama"] = ollama.NewManagedOllamaCmd }
