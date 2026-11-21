package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/share"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	cmd := share.NewCmd()
	for _, sub := range fleet.NewShareCmd().Commands() {
		cmd.AddCommand(sub)
	}
	return cmd
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "bashy share:", fleet.ExitCode))
}
