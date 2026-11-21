package main

import (
	"context"
	"os"
	"strings"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

const reviewedVersion = "v1.54.0"

var stripeDigests = map[string]bool{
	"stripe_1.54.0_mac-os_arm64.tar.gz":  true,
	"stripe_1.54.0_mac-os_x86_64.tar.gz": true,
	"stripe_1.54.0_linux_arm64.tar.gz":   true,
	"stripe_1.54.0_linux_x86_64.tar.gz":  true,
	"stripe_1.54.0_windows_x86_64.zip":   true,
}

func stripeAssetMatch(asset, goos, goarch string) bool {
	if !stripeDigests[asset] {
		return false
	}
	if goos == "windows" && goarch != "amd64" {
		return false
	}
	osTok := map[string]string{"darwin": "mac-os", "linux": "linux", "windows": "windows"}[goos]
	archTok := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[goarch]
	if osTok == "" || archTok == "" {
		return false
	}
	a := strings.ToLower(asset)
	return strings.Contains(a, osTok) && strings.Contains(a, "_"+archTok)
}

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:       "stripe",
		Tier:       6,
		License:    "Apache-2.0",
		Synopsis:   "Stripe CLI (managed external, Apache-2.0; host-first, test-mode only)",
		EnvVersion: "STRIPE_VERSION",
		PreferHost: true,
		Long: `stripe (stripe/stripe-cli, Apache-2.0; reviewed v1.54.0) is a short-lived client with
explicit target and credential selection (no ambient account). Webhook
forwarding/serving, tails, sockets and callback-listener logins are not admitted.
All args pass through. Remote service (separate): Stripe API (proprietary hosted service under the account holder's agreement).`,
		ExecEnv: func() []string {
			return append(append([]string{}, os.Environ()...), "STRIPE_CLI_TELEMETRY_OPTOUT=1")
		},
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			if version == "" {
				version = reviewedVersion
			}
			return binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
				Name:       "stripe",
				Repo:       "stripe/stripe-cli",
				Version:    version,
				Member:     "stripe",
				AssetMatch: stripeAssetMatch,
			})
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
