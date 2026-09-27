// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/qiangli/yoke/pkg/broker"
)

// The host's model door (Sprint 302, yoke pkg/broker): ONE process per host,
// port 24556, in front of bashy's own Ollama engine and the cligw agent
// pools. `bashy ollama serve` and `bashy llm serve` both start it; the raw
// engine is this binary again, run by the door as a child with
// BASHY_OLLAMA_ENGINE=1 on a private loopback port nobody else is told about.

func init() {
	broker.EngineArgv = func() []string { return []string{bashySelfPath(), "ollama", "serve"} }
	broker.SelfArgv = func() []string { return []string{bashySelfPath(), "llm", "serve"} }
}

// isDoorServe reports whether `bashy ollama ARGS` asks for the door rather
// than the raw engine: `serve`, from anyone but the door itself.
func isDoorServe(args []string) bool {
	return len(args) > 0 && args[0] == "serve" && os.Getenv(broker.EngineModeEnv) == ""
}

// runOllamaDoor serves the door for `bashy ollama serve`. Extra arguments are
// refused: the engine's own serve flags belong to the engine, which the door
// configures.
func runOllamaDoor(args []string) int {
	if len(args) > 0 && args[0] != "--help" && args[0] != "-h" {
		fmt.Fprintf(os.Stderr, "bashy ollama serve: unexpected arguments %v — the door takes its settings from `bashy llm serve --help`\n", args)
		return 2
	}
	if len(args) > 0 {
		fmt.Println("bashy ollama serve starts the host's model door (same as `bashy llm serve`): see `bashy llm serve --help`.")
		return 0
	}
	err := broker.RunDoor(context.Background(), broker.DoorOptions{Out: os.Stdout})
	if err != nil && !errors.Is(err, broker.ErrAlreadyServing) {
		fmt.Fprintln(os.Stderr, "bashy ollama serve:", err)
		return 1
	}
	return 0
}

// ShellStartup mints this shell's model-door session (Sprint 255 §3 scoping,
// Sprint 302 Q5): a child of the exported view it inherited, or a new root.
// Only the exported view goes into the environment, so every child process —
// in any language — sees the exported part only; gate executors start empty
// because the attest allowlist drops the variable.
func ShellStartup() {
	shellModelSession = broker.ShellSessionFromEnv()
}

// shellModelSession is this shell's own handle (its full view); the
// environment carries only ExportedView(shellModelSession).
var shellModelSession string
