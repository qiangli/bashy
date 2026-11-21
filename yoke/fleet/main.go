package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "fleet",
		Short: "Coordination credentials for satellite migrations (master-token save/mint)",
	}
	root.AddCommand(fleet.NewMasterTokenCmd())
	return root
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "bashy fleet:", fleet.ExitCode))
}
