package agentos

import (
	"fmt"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

func selfSeedCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "seed", Short: "Transfer a verified managed-tool cache for offline installation"}
	cmd.AddCommand(&cobra.Command{Use: "export <archive.tar>", Short: "Export this platform's managed tool cache", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := binmgr.ExportSeed(cmd.Context(), args[0]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), args[0])
		return nil
	}})
	return cmd
}
