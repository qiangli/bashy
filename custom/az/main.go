package main

import (
	"context"
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/az"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:       "az",
		Tier:       6,
		License:    az.License,
		Synopsis:   "Azure CLI (managed external, MIT; host-first)",
		PreferHost: true,
		Long: `az (Microsoft Azure CLI, Azure/azure-cli, MIT) fronts resource groups,
VMs and ARM deployments with an explicit subscription
(AZURE_SUBSCRIPTION_ID) plus a credential source. bashy uses the host's
own install when present; if absent it points at the vendor installer
rather than downloading. All args pass through to az. There is no
ambient-subscription fallback and login never implies provisioning
permission. Deployment mutations preview with --what-if; other dry runs
refuse without execution.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return az.Resolve(ctx, version)
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
