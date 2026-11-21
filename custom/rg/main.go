package main

import (
	"context"
	"os"
	"strings"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

const defaultVersion = "15.1.0"

func rgAssetMatch(name, goos, goarch string) bool {
	n := strings.ToLower(name)
	if !strings.HasPrefix(n, "ripgrep-") {
		return false
	}
	if goos == "darwin" {
		return strings.Contains(n, "apple-darwin") && rgArchMatch(n, goarch)
	}
	if goos == "windows" {
		return strings.Contains(n, "pc-windows-msvc") && rgArchMatch(n, goarch)
	}
	if goos == "linux" {
		return strings.Contains(n, "unknown-linux") && rgArchMatch(n, goarch)
	}
	return strings.Contains(n, goos) && rgArchMatch(n, goarch)
}

func rgArchMatch(assetName, goarch string) bool {
	switch goarch {
	case "amd64":
		return strings.Contains(assetName, "x86_64")
	case "arm64":
		return strings.Contains(assetName, "aarch64")
	case "386":
		return strings.Contains(assetName, "i686")
	default:
		return strings.Contains(assetName, goarch)
	}
}

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:           "rg",
		Tier:           1,
		License:        "MIT OR Unlicense",
		Synopsis:       "ripgrep fast recursive text search (managed external, MIT OR Unlicense)",
		EnvVersion:     "RG_VERSION",
		DefaultVersion: defaultVersion,
		Long: `rg runs ripgrep (BurntSushi/ripgrep) - downloaded from GitHub
releases, sha256-verified against pinned digests, and cached by binmgr (not compiled in).
$RG_VERSION pins the release; all args pass through to rg.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			if version == "" {
				version = defaultVersion
			}
			return binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
				Name:       "rg",
				Repo:       "BurntSushi/ripgrep",
				Version:    version,
				Member:     "rg",
				AssetMatch: rgAssetMatch,
			})
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
