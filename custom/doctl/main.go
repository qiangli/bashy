package main

import (
	"context"
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

const defaultVersion = "v1.179.0"

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:           "doctl",
		Tier:           6,
		License:        "Apache-2.0",
		Synopsis:       "DigitalOcean CLI (managed external, Apache-2.0)",
		EnvVersion:     "DOCTL_VERSION",
		DefaultVersion: defaultVersion,
		Long: `doctl (digitalocean/doctl, Apache-2.0) is the DigitalOcean cloud CLI —
downloaded from GitHub releases, sha256-verified against pinned digests, and cached
by binmgr (not compiled in). $DOCTL_VERSION pins the release; all args pass through
to doctl. Authenticate with 'bashy doctl auth init' (token via 'bashy secrets').`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			if version == "" {
				version = defaultVersion
			}
			return binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
				Name:    "doctl",
				Repo:    "digitalocean/doctl",
				Version: version,
				Member:  binmgr.BinaryName("doctl"),
			})
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
