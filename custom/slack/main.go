package main

import (
	"context"
	"os"
	"strings"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

const reviewedVersion = "v4.10.0"

var slackDigests = map[string]bool{
	"slack_cli_4.10.0_macOS_arm64.tar.gz": true,
	"slack_cli_4.10.0_macOS_amd64.tar.gz": true,
	"slack_cli_4.10.0_linux_arm64.tar.gz": true,
	"slack_cli_4.10.0_linux_amd64.tar.gz": true,
	"slack_cli_4.10.0_windows_64-bit.zip": true,
}

func slackAssetMatch(asset, goos, goarch string) bool {
	if !slackDigests[asset] {
		return false
	}
	if goos == "windows" && goarch != "amd64" {
		return false
	}
	osTok := map[string]string{"darwin": "macos", "linux": "linux", "windows": "windows"}[goos]
	archTok := map[string]string{"amd64": "amd64", "arm64": "arm64"}[goarch]
	if goos == "windows" {
		archTok = "64-bit"
	}
	if osTok == "" || archTok == "" {
		return false
	}
	a := strings.ToLower(asset)
	return strings.Contains(a, osTok) && strings.Contains(a, "_"+archTok)
}

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:       "slack",
		Tier:       6,
		License:    "Apache-2.0",
		Synopsis:   "Slack CLI (managed external, Apache-2.0; app tooling — messaging via Web API adapter)",
		EnvVersion: "SLACK_VERSION",
		PreferHost: true,
		Long: `slack (slackapi/slack-cli, Apache-2.0; reviewed v4.10.0) is a short-lived client with
explicit target and credential selection (no ambient account). Webhook
forwarding/serving, tails, sockets and callback-listener logins are not admitted.
All args pass through. Remote service (separate): Slack platform (proprietary hosted service under the workspace owner's agreement).`,
		ExecEnv: func() []string { return os.Environ() },
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			if version == "" {
				version = reviewedVersion
			}
			return binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
				Name:       "slack",
				Repo:       "slackapi/slack-cli",
				Version:    version,
				Member:     "slack",
				AssetMatch: slackAssetMatch,
			})
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
