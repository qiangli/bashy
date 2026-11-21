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
		Name:       "twilio",
		Tier:       6,
		License:    "MIT",
		Synopsis:   "Twilio CLI (host-only external, MIT; sandbox account only)",
		PreferHost: true,
		Long: `twilio (twilio/twilio-cli, MIT; reviewed 7.2.0) is a short-lived client with
explicit target and credential selection (no ambient account). Webhook
forwarding/serving, tails, sockets and callback-listener logins are not admitted.
All args pass through. Remote service (separate): Twilio API (proprietary hosted service under the account holder's agreement).`,
		ExecEnv: func() []string { return os.Environ() },
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return binmgr.Tool{}, fmt.Errorf("twilio not found on PATH and no managed archive release is admitted — install twilio-cli 7.2.0 (npm i -g twilio-cli@7.2.0 or the vendor installer) and put twilio on PATH; the release publishes installers only, no archive")
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
