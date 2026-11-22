// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// Sprint: #490; Story: #2607; Story-ID: 7bc375b8ad69
// Sprint: #490; Story: #2644; Story-ID: 0a55048e5c6a (satellite/reproduce/paired-host rods)

import (
	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/share"
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

// newShareCmd mounts yoke's host-app share verbs (`who`, and the grant rule
// verbs `add`, `ls`, `rm`) under the existing `share` command beside `qr`.
func newShareCmd() *cobra.Command {
	cmd := share.NewCmd()
	for _, sub := range fleet.NewShareCmd().Commands() {
		cmd.AddCommand(sub)
	}
	return cmd
}
