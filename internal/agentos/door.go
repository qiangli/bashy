// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/qiangli/yoke/pkg/broker"
	"github.com/qiangli/yoke/pkg/fleet"
)

// The host's model door (Sprint 302, yoke pkg/broker): ONE process per host,
// port 24556, in front of bashy's own Ollama engine and the cligw agent
// pools. `bashy ollama serve` and `bashy llm serve` both start it; the raw
// engine is this binary again, run by the door as a child with
// BASHY_OLLAMA_ENGINE=1 on a private loopback port nobody else is told about.

func init() {
	broker.EngineArgv = func() []string { return []string{bashySelfPath(), "ollama", "serve"} }
	broker.SelfArgv = func() []string { return []string{bashySelfPath(), "llm", "serve"} }
	// A fresh fleet.New() per call, not a cached Catalog, so it honors
	// BASHY_HOME (and the other fleet env overrides) as of the call — the
	// same lazy-load pattern dispatchService uses for `bashy service
	// enable/disable`, which this seam composes with.
	broker.SetServiceEnabled = func(ctx context.Context, unit string, enabled bool) error {
		return fleet.New().SetServiceEnabled(ctx, unit, enabled)
	}
	// `llm up --service` starts the monitor rather than the door itself, so
	// supervisord — not this call — is what restarts the door on a crash or
	// at boot. The argv matches dispatchSupervisord's own invocation: `bashy
	// supervisord` with no further arguments.
	broker.EnsureSupervisordRunning = func(ctx context.Context) error {
		return fleet.EnsureSupervisordRunning(ctx, []string{bashySelfPath(), "supervisord"})
	}
	// `llm down --service` prefers this over signalling the door pid
	// directly (Sprint 455 F9): the same control path `bashy service stop
	// llm` uses, so the stop goes through supervisord's own bookkeeping
	// instead of racing it. errNoMonitor surfaces as-is when nothing
	// answers the socket.
	broker.StopServiceNow = func(ctx context.Context, unit string) error {
		_, err := ctlCall(supervisordSocket(), ctlRequest{Op: "stop", Unit: unit}, 5*time.Second)
		return err
	}
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
