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
		Name:       "psql",
		Tier:       6,
		License:    "PostgreSQL",
		Synopsis:   "PostgreSQL client psql (host-only external, PostgreSQL licence; remote endpoint only)",
		PreferHost: true,
		Long: `psql is a short-lived client to an explicitly configured, externally managed
endpoint (PostgreSQL server (or wire-compatible)). bashy never installs, starts or provisions a
server, and uses the host's own client (no managed download is admitted). All
args pass through; the typed bounded/rollback path is dataclients.Execute.
Client licence: PostgreSQL. Remote engine licence (separate): PostgreSQL (permissive); wire-compatible forks and managed services carry their own terms.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("psql not found on PATH and no sha256-pinned managed release is admitted — %s", "install the PostgreSQL client package (postgresql-client / libpq) and put psql on PATH")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
