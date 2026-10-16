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
