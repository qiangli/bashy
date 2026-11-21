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
		Name:       "valkey-cli",
		Tier:       6,
		License:    "BSD-3-Clause",
		Synopsis:   "Valkey client valkey-cli (host-only external, BSD-3-Clause; remote endpoint only)",
		PreferHost: true,
		Long: `valkey-cli is a short-lived client to an explicitly configured, externally managed
endpoint (Valkey server, or a Redis-protocol-compatible server). bashy never installs, starts or provisions a
server, and uses the host's own client (no managed download is admitted). All
args pass through; the typed bounded/rollback path is dataclients.Execute.
Client licence: BSD-3-Clause. Remote engine licence (separate): BSD-3-Clause for Valkey; Redis 7.4+ is RSALv2/SSPLv1 (8.0+ also AGPLv3) — recorded separately, not admitted by this client.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("valkey-cli not found on PATH and no sha256-pinned managed release is admitted — %s", "install the Valkey client tools (valkey-tools / valkey) and put valkey-cli on PATH")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
