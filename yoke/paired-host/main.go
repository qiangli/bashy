package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	cmd := fleet.NewPairedHostCmd()
	cmd.Use = "paired-host"
	return cmd
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "bashy paired-host:", fleet.ExitCode))
}
