// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// Sprint: #490; Story: #2607; Story-ID: 7bc375b8ad69
// Sprint: #490; Story: #2644; Story-ID: 0a55048e5c6a (satellite/reproduce/paired-host rods)

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

// newPairedHostCmd mounts the paired-hosts tree (fleet.NewPairedHostCmd)
// under the `paired-host` verb (S490 story 0a55048e5c6a). Yoke's tree is
// named `host`, but a bare `host` shim would shadow the platform DNS
// lookup (/usr/bin/host on unix), so the front door respells it; the only
// subcommand in this run is `ls`.
func newPairedHostCmd() *cobra.Command {
	cmd := fleet.NewPairedHostCmd()
	cmd.Use = "paired-host"
	return cmd
}

// dispatchSatellite runs the `bashy satellite` verb (currently invite only).
func dispatchSatellite(args []string) int {
	cmd := fleet.NewSatelliteCmd()
	setCobraArgs(cmd, args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy satellite:", err)
		return fleet.ExitCode(err)
	}
	return 0
}

// dispatchReproduce runs the `bashy reproduce` verb (plan-only in this run).
func dispatchReproduce(args []string) int {
	cmd := fleet.NewReproduceCmd()
	setCobraArgs(cmd, args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy reproduce:", err)
		return fleet.ExitCode(err)
	}
	return 0
}

// dispatchPairedHost runs the `bashy paired-host` verb (currently ls only).
func dispatchPairedHost(args []string) int {
	cmd := newPairedHostCmd()
	setCobraArgs(cmd, args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy paired-host:", err)
		return fleet.ExitCode(err)
	}
	return 0
}
