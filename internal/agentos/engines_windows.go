// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build bashy_engines && windows && remote && containers_image_openpgp

package agentos

import (
	"os"

	podmanengine "github.com/qiangli/yoke/external/podman/engine"
)

// dispatchEngine wires the Windows-capable Podman machine frontend when bashy
// is built with the Podman remote tags, alongside the embedded Ollama server.
func dispatchEngine(arg string) {
	if code := guardEngine(arg, os.Args[2:]); code != 0 {
		dispatchExit(code)
	}
	arg = engineAlias(arg) // `bashy docker` -> podman engine
	if code := ensureEngineReady(arg, os.Args[2:]); code != 0 {
		dispatchExit(code)
	}
	switch arg {
	case "podman":
		cmd := podmanengine.NewPodmanCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			dispatchExit(1)
		}
		dispatchExit(0)
	case "ollama":
		dispatchOllamaEngine()
	case "dks":
		if handled, code := handleDKSScope(os.Args[2:]); handled {
			dispatchExit(code)
		}
		// Dedicated rootful DKS (k3s) machine — Windows uses the WSL/Hyper-V
		// provider under the hood via the same podman machine API.
		cmd := podmanengine.NewDKSCmd()
		cmd.Long += "\n\nScope namespaces: 'bashy dks use @scope:<path>' selects one; 'bashy dks list [--json]' lists reachable ones."
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
