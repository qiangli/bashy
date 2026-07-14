// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_obs

package agentos

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/otelquery"
)

// dispatchObs (default lean build) reports that the observability stack is not
// compiled in. `bashy otel` embeds ~193 MB of collector/victoria/jaeger/perses/
// k8s/aws — a host-only concern — so the default worker binary omits it. Build a
// host binary with `-tags bashy_obs`, or run otel on a host node.
func dispatchObs(arg string) {
	if arg == "otel" {
		cmd := otelquery.NewCommand()
		cmd.AddCommand(&cobra.Command{
			Use:   "serve",
			Short: "Start the embedded OTEL stack (requires -tags bashy_obs)",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("the observability stack is not in this build (rebuild with -tags bashy_obs, or run it on a host node)")
			},
		})
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			if !otelquery.ErrorAlreadyPrinted(err) {
				fmt.Fprintln(os.Stderr, "bashy otel:", err)
			}
			os.Exit(1)
		}
		os.Exit(0)
	}
}
