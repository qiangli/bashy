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
		Name:       "mosquitto",
		Tier:       6,
		License:    "BSD-3-Clause",
		Synopsis:   "Mosquitto MQTT clients mosquitto_pub/mosquitto_sub (host-only external, BSD-3-Clause; remote broker only, no broker startup)",
		PreferHost: true,
		Long: `mosquitto is a short-lived client to an explicitly configured, externally managed
broker (Eclipse Mosquitto broker, or any MQTT v3.1/v3.1.1/v5 broker). bashy never installs, starts or provisions a
broker, and uses the host's own client (no managed download is admitted);
subscriptions are always bounded — persistent subscribers are long-lived
services and are not admitted here. All args pass through; the typed
bounded/publish-explicit path is msgclients.Execute.
Host binaries: mosquitto_pub and mosquitto_sub. Client licence: BSD-3-Clause. Broker licence (separate): EPL-2.0 OR EDL-1.0 for the Eclipse Mosquitto broker; other brokers and managed services carry their own terms — recorded separately, not admitted by this client.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("mosquitto not found on PATH and no sha256-pinned managed release is admitted — %s", "install the Mosquitto clients (mosquitto-clients) and put mosquitto_pub and mosquitto_sub on PATH")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
