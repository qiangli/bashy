package main

import (
	"context"
	"fmt"
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:       "m365",
		Tier:       6,
		License:    "MIT",
		Synopsis:   "Microsoft 365 CLI (host-only external, MIT; explicit test tenant, least-scope Graph permissions)",
		PreferHost: true,
		Long: `m365 (pnp/cli-microsoft365, MIT; reviewed v11.11.0) is a short-lived client with
explicit tenant/domain and credential selection (no ambient account). Bulk
fan-out, raw API requests and interactive logins are not admitted. All args
pass through. Remote service (separate): Microsoft 365 / Entra ID / Microsoft Graph (proprietary hosted service under the tenant owner's agreement).`,
		ExecEnv: func() []string { return os.Environ() },
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("m365 not found on PATH and no managed archive release is admitted — %s", "install @pnp/cli-microsoft365@11.11.0 (npm i -g @pnp/cli-microsoft365@11.11.0, Node runtime required) and put m365 on PATH; the project publishes an npm package only, no binary archive")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
