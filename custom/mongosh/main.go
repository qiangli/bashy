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
		Name:       "mongosh",
		Tier:       6,
		License:    "Apache-2.0",
		Synopsis:   "MongoDB Shell mongosh (host-only external, Apache-2.0; remote endpoint only)",
		PreferHost: true,
		Long: `mongosh is a short-lived client to an explicitly configured, externally managed
endpoint (MongoDB server (or wire-compatible)). bashy never installs, starts or provisions a
server, and uses the host's own client (no managed download is admitted). All
args pass through; the typed bounded/rollback path is dataclients.Execute.
Client licence: Apache-2.0. Remote engine licence (separate): SSPL-1.0 for MongoDB Community Server (not OSI-permissive); Atlas/managed and compatible services carry their own terms.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("mongosh not found on PATH and no sha256-pinned managed release is admitted — %s", "install mongosh from https://www.mongodb.com/try/download/shell and put it on PATH")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
