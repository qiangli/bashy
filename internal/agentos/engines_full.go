// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build bashy_engines && !windows

package agentos

import (
	"os"

	"github.com/qiangli/coreutils/external/ollama"
	podmanengine "github.com/qiangli/coreutils/external/podman/engine"
)

// dispatchEngine (host build, -tags bashy_engines on a unix host) wires the
// container/LLM engine subcommands `bashy podman` / `bashy ollama`. They embed
// cgo + platform-specific backends (podman's btrfs/devmapper storage drivers,
// ollama's Apple MLX) and a heavy dep tree, so they are OPT-IN: excluded from
// the default lean worker (which cross-compiles to every platform with
// CGO_ENABLED=0) and from every Windows build.
func dispatchEngine(arg string) {
	arg = engineAlias(arg) // `bashy docker` -> podman engine
	switch arg {
	case "podman":
		// Managed, ISOLATED in-process podman (embeds a podman fork):
		// pass-through to the embedded binary with CONTAINER_HOST pointed at
		// bashy's own `bashy` machine socket, so images/containers never collide
		// with a host engine. $BASHY_PODMAN_SYSTEM=1 defers to host podman.
		cmd := podmanengine.NewPodmanCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "ollama":
		// Managed, ISOLATED ollama: own bashy-owned port (never 11434), models
		// under ~/.agents/bashy/ollama — never the host's ~/.ollama. Reached by
		// the `ollama` service name. Cloud (:cloud / signin) is gated: bashy ollama
		// is self-hosted, so ollama.com sign-in is opt-in only.
		if blocked, msg := ollamaCloudGate(os.Args[2:]); blocked {
			os.Stderr.WriteString(msg)
			os.Exit(2)
		}
		cmd := ollama.NewManagedOllamaCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
}
