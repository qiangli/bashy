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
		Name:       "gam",
		Tier:       6,
		License:    "Apache-2.0",
		Synopsis:   "GAM Workspace admin (host-only external, Apache-2.0; admin gaps gws defers only, reuses the gws credential selection)",
		PreferHost: true,
		Long: `gam (GAM-team/GAM, Apache-2.0; reviewed v7.48.30) is a short-lived client with
explicit tenant/domain and credential selection (no ambient account). Bulk
fan-out, raw API requests and interactive logins are not admitted. All args
pass through. Remote service (separate): Google Workspace Admin SDK (proprietary hosted service under the domain owner's agreement).`,
		ExecEnv: func() []string { return os.Environ() },
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("gam not found on PATH and no managed archive release is admitted — %s", "install GAM v7.48.30 from the GAM-team/GAM release (verify the SHA-256 in EVIDENCE.md) and put gam on PATH; releases are .tar.xz/.zip/.exe, which binmgr does not extract")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
