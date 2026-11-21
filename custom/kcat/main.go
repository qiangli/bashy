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
		Name:       "kcat",
		Tier:       6,
		License:    "BSD-2-Clause",
		Synopsis:   "Kafka client kcat (host-only external, BSD-2-Clause; remote cluster only)",
		PreferHost: true,
		Long: `kcat is a short-lived client to an explicitly configured, externally managed
broker (Apache Kafka cluster (or a wire-compatible endpoint)). bashy never installs, starts or provisions a
broker, and uses the host's own client (no managed download is admitted);
subscriptions are always bounded — persistent subscribers are long-lived
services and are not admitted here. All args pass through; the typed
bounded/publish-explicit path is msgclients.Execute.
Host binaries: kcat. Client licence: BSD-2-Clause. Broker licence (separate): Apache-2.0 for Apache Kafka — recorded separately, not admitted by this client.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("kcat not found on PATH and no sha256-pinned managed release is admitted — %s", "install kcat (bundling librdkafka) and put kcat on PATH")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
