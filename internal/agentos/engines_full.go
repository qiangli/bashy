// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build bashy_engines && !windows && (cgo || containers_image_openpgp)

package agentos

import (
	"os"

	podmanengine "github.com/qiangli/yoke/external/podman/engine"
)

// dispatchEngine wires the opt-in container and LLM engines on Unix.
// Pure-Go Podman builds need containers_image_openpgp to avoid native GPGME;
// without that tag the external Podman dispatcher remains available.
func dispatchEngine(arg string) {
	if code := guardEngine(arg, os.Args[2:]); code != 0 {
		dispatchExit(code)
	}
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
			dispatchExit(1)
		}
		dispatchExit(0)
	case "ollama":
		dispatchOllamaEngine()
	case "dks":
		// Provision/manage the dedicated ROOTFUL podman machine that DKS
		// (k3s) runs in — a uniform VM so outposts in the wild don't each
		// hit a different podman-machine quirk (rootless → cgroup denial →
		// pods silently never start). Reuses the engine's EnsureMachine.
		cmd := podmanengine.NewDKSCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			dispatchExit(1)
		}
		dispatchExit(0)
	}
}

func init() {
	cobraOptionalVerbs["podman"] = podmanengine.NewPodmanCmd
	cobraOptionalVerbs["docker"] = podmanengine.NewPodmanCmd
	cobraOptionalVerbs["dks"] = podmanengine.NewDKSCmd
}
