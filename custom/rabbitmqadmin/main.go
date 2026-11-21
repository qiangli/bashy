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
		Name:       "rabbitmqadmin",
		Tier:       6,
		License:    "MIT",
		Synopsis:   "RabbitMQ management CLI rabbitmqadmin v2 (host-only external, MIT; remote server only)",
		PreferHost: true,
		Long: `rabbitmqadmin is a short-lived client to an explicitly configured, externally managed
broker (RabbitMQ server with the management plugin (or a compatible endpoint)). bashy never installs, starts or provisions a
broker, and uses the host's own client (no managed download is admitted);
subscriptions are always bounded — persistent subscribers are long-lived
services and are not admitted here. All args pass through; the typed
bounded/publish-explicit path is msgclients.Execute.
Host binaries: rabbitmqadmin. Client licence: MIT. Broker licence (separate): MPL-2.0 for the RabbitMQ server — recorded separately, not admitted by this client.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("rabbitmqadmin not found on PATH and no sha256-pinned managed release is admitted — %s", "install rabbitmqadmin v2 and put rabbitmqadmin on PATH")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
