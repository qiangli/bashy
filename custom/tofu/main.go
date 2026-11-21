package main

import (
	"context"
	"os"
	"strings"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

const defaultVersion = "v1.13.1"

func tofuAssetMatch(name, goos, goarch string) bool {
	n := strings.ToLower(name)
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	if !strings.HasSuffix(n, ext) {
		return false
	}
	if !strings.Contains(n, "_"+goos+"_") {
		return false
	}
	return strings.Contains(n, "_"+goarch+".") || strings.HasSuffix(n, "_"+goarch+ext)
}

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:           "tofu",
		Tier:           6,
		License:        "MPL-2.0",
		Synopsis:       "OpenTofu infrastructure-as-code CLI (managed external, MPL-2.0)",
		EnvVersion:     "TOFU_VERSION",
		DefaultVersion: defaultVersion,
		Long: `tofu (opentofu/opentofu, MPL-2.0) is the OpenTofu infrastructure-as-code
CLI — downloaded from GitHub releases, sha256-verified against pinned digests,
and cached by binmgr (not compiled in). $TOFU_VERSION pins the release; all args
pass through to tofu. It is also the processor of the Bash# '~~~tf as iac' text
fence: iac.init(), iac.plan(), iac.apply(), iac.destroy().`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			if version == "" {
				version = defaultVersion
			}
			return binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
				Name:       "tofu",
				Repo:       "opentofu/opentofu",
				Version:    version,
				Member:     binmgr.BinaryName("tofu"),
				AssetMatch: tofuAssetMatch,
			})
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
