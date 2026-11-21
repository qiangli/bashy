package main

import (
	"context"
	"os"
	"strings"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

const defaultVersion = "v1.19.0"

func karmadactlAssetMatch(name, goos, goarch string) bool {
	want := "karmadactl-" + goos + "-" + goarch + ".tgz"
	return strings.ToLower(name) == want
}

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:           "karmadactl",
		Tier:           5,
		License:        "Apache-2.0",
		Synopsis:       "Karmada federation CLI (managed external, Apache-2.0)",
		EnvVersion:     "KARMADACTL_VERSION",
		DefaultVersion: defaultVersion,
		Long: `karmadactl (karmada-io/karmada, Apache-2.0) is the Karmada
multi-cluster federation CLI — downloaded from GitHub releases,
sha256-verified against pinned digests, and cached by binmgr (not compiled
in). $KARMADACTL_VERSION pins the release; all args pass through to
karmadactl. Reached through 'bashy dks federation karmadactl ...', which
selects DKS_PROFILE=federation for its kubectl calls.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			if version == "" {
				version = defaultVersion
			}
			return binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
				Name:       "karmadactl",
				Repo:       "karmada-io/karmada",
				Version:    version,
				Member:     binmgr.BinaryName("karmadactl"),
				AssetMatch: karmadactlAssetMatch,
			})
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
