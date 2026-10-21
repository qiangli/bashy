// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// Sprint: #490; Story: #2607; Story-ID: 7bc375b8ad69

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/fleet"
)

// newFleetCmd mounts the coordination-credential tree under the `fleet`
// verb: master-token save/mint (fleet.NewMasterTokenCmd). The minted or
// saved value is written to the 0600 master-token file and never printed.
func newFleetCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "fleet",
		Short: "Coordination credentials for satellite migrations (master-token save/mint)",
	}
	root.AddCommand(fleet.NewMasterTokenCmd())
	return root
}

// dispatchToken runs the scoped-token tree (fleet.NewTokenCmd: create list
// revoke) as the top-level `bashy token` verb.
func dispatchToken(args []string) int {
	cmd := fleet.NewTokenCmd()
	setCobraArgs(cmd, args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy token:", err)
		return fleet.ExitCode(err)
	}
	return 0
}

// dispatchFleet runs the `bashy fleet` verb (currently master-token only).
func dispatchFleet(args []string) int {
	cmd := newFleetCmd()
	setCobraArgs(cmd, args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy fleet:", err)
		return fleet.ExitCode(err)
	}
	return 0
}
