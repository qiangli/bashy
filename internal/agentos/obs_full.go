// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build bashy_obs

package agentos

import (
	"os"

	"github.com/qiangli/coreutils/external/otel/otelcli"
)

// dispatchObs (full build, -tags bashy_obs) wires the all-in-one observability
// stack as `bashy otel`. It pulls in the OpenTelemetry Collector +
// VictoriaMetrics/Logs + Jaeger + Perses + k8s/aws SDKs (~193 MB), so it is
// compiled in only for a host build, never the default lean worker.
func dispatchObs(arg string) {
	if arg == "otel" {
		cmd := otelcli.NewCommand()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
}
