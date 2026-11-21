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
		Name:       "snow",
		Tier:       6,
		License:    "Apache-2.0",
		Synopsis:   "Snowflake CLI snow (host-only external, Apache-2.0; remote warehouse only)",
		PreferHost: true,
		Long: `snow is a short-lived client to an explicitly configured, externally managed
endpoint (Snowflake cloud data warehouse (SaaS)). bashy never installs, starts or provisions a
server, and uses the host's own client (no managed download is admitted). All
args pass through; the typed bounded/rollback path is dataclients.Execute.
Client licence: Apache-2.0. Remote engine licence (separate): proprietary hosted service under the account holder's own Snowflake agreement; not an open-source engine.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("snow not found on PATH and no sha256-pinned managed release is admitted — %s", "install Snowflake CLI (pipx install snowflake-cli) and put snow on PATH")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
