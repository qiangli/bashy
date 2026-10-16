//go:build !bashy_engines

package agentos

import (
	"fmt"
	"os"
)

func dispatchOllamaEngine() {
	fmt.Fprint(os.Stderr, engineNotFoundMessage("ollama"))
	dispatchExit(1)
}

func probeOllamaEngine() ollamaEngineProbe {
	return ollamaEngineProbe{Build: "lean"}
}
