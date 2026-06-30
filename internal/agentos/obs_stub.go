// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_obs

package agentos

import (
	"fmt"
	"os"
)

// dispatchObs (default lean build) reports that the observability stack is not
// compiled in. `bashy otel` embeds ~193 MB of collector/victoria/jaeger/perses/
// k8s/aws — a host-only concern — so the default worker binary omits it. Build a
// host binary with `-tags bashy_obs`, or run otel on a host node.
func dispatchObs(arg string) {
	if arg == "otel" {
		fmt.Fprintln(os.Stderr,
			"bashy otel: the observability stack is not in this build "+
				"(rebuild with -tags bashy_obs, or run it on a host node)")
		os.Exit(1)
	}
}
