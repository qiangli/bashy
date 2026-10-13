package agentos

import (
	"fmt"
	"os"
	"path/filepath"

	runnerembed "github.com/qiangli/yoke/external/ollama/runner_embed"
)

var ensureEmbeddedOllamaRunner = runnerembed.EnsureRunner
var runEmbeddedOllamaRunner = execEnginePassthrough

// dispatchEmbeddedOllamaRunner implements the Ollama v0.21.2 re-exec
// protocol: Ollama starts os.Executable() with "runner" followed by the
// runner's original arguments. EnsureRunner only materializes the embedded,
// signed build artifact; there is deliberately no PATH or system-binary
// fallback here.
func dispatchEmbeddedOllamaRunner(args []string) int {
	cache := engineCacheDir()
	if cache == "" {
		fmt.Fprintln(os.Stderr, "bashy ollama runner: cannot determine the managed cache directory")
		return 1
	}
	path, err := ensureEmbeddedOllamaRunner(filepath.Join(cache, "ollama-runner"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy ollama runner: embedded runner unavailable: %v\n", err)
		return 1
	}
	return runEmbeddedOllamaRunner(path, args)
}
