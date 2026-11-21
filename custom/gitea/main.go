package main

import (
	"context"
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

const defaultVersion = "v28.1.0"

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:           "gitea",
		Tier:           2,
		License:        "MIT",
		Synopsis:       "Gitea lightweight Git forge (managed external, MIT)",
		EnvVersion:     "GITEA_VERSION",
		DefaultVersion: defaultVersion,
		Long: `gitea (go-gitea/gitea, MIT) is the lightweight Git forge — downloaded from
GitHub releases, sha256-verified against pinned digests, and cached by binmgr (not
compiled in). $GITEA_VERSION pins the release; all args pass through to gitea.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			if version == "" {
				version = defaultVersion
			}
			return binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
				Name:    "gitea",
				Repo:    "go-gitea/gitea",
				Version: version,
			})
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
