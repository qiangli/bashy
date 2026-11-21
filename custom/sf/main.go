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
		Name:       "sf",
		Tier:       6,
		License:    "Apache-2.0",
		Synopsis:   "Salesforce CLI sf (host-only external, Apache-2.0; explicit sandbox org only)",
		PreferHost: true,
		Long: `sf (salesforcecli/cli, Apache-2.0; reviewed 2.156.4) is a short-lived client with
explicit target and credential selection (no ambient account). Webhook
forwarding/serving, tails, sockets and callback-listener logins are not admitted.
All args pass through. Remote service (separate): Salesforce platform (proprietary hosted service under the org owner's agreement).`,
		ExecEnv: func() []string {
			return append(append([]string{}, os.Environ()...), "SF_DISABLE_TELEMETRY=true", "SF_AUTOUPDATE_DISABLE=true")
		},
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("sf not found on PATH and no managed archive release is admitted — install the Salesforce CLI release 2.156.4 (npm i -g @salesforce/cli@2.156.4 or the vendor tarball) and put sf on PATH")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
