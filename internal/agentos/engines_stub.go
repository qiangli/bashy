// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines || (windows && (!remote || !containers_image_openpgp))

package agentos

import (
	"fmt"
	"os"
)

// dispatchEngine (default lean build) reports that the container/LLM engines are
// not compiled in. The default worker is a self-contained shell + git + dag +
// `bashy go` + userland that cross-compiles everywhere with CGO_ENABLED=0.
func dispatchEngine(arg string) {
	switch arg {
	case "podman", "ollama":
		extra := ""
		if arg == "podman" {
			extra = "; Windows Podman support also requires -tags 'remote containers_image_openpgp'"
		}
		fmt.Fprintf(os.Stderr,
			"bashy %s: the container/LLM engines are not in this build "+
				"(rebuild with -tags bashy_engines%s, or run them on a host node)\n", arg, extra)
		os.Exit(1)
	}
}
