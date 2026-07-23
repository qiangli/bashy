// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build bashy_engines && windows && remote && containers_image_openpgp

package agentos

import (
	"fmt"
	"os"

	podmanengine "github.com/qiangli/coreutils/external/podman/engine"
)

// dispatchEngine wires the Windows-capable Podman machine frontend when bashy
// is built with -tags bashy_engines. Ollama remains a unix host feature.
func dispatchEngine(arg string) {
	arg = engineAlias(arg) // `bashy docker` -> podman engine
	switch arg {
	case "podman":
		cmd := podmanengine.NewPodmanCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "ollama":
		fmt.Fprintln(os.Stderr, "bashy ollama: not supported in the Windows engine build")
		os.Exit(1)
	case "dks":
		// Dedicated rootful DKS (k3s) machine — Windows uses the WSL/Hyper-V
		// provider under the hood via the same podman machine API.
		cmd := podmanengine.NewDKSCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
}
