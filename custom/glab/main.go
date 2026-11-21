package main

import (
	"context"
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/glab"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:       "glab",
		Tier:       6,
		License:    glab.License,
		Synopsis:   "GitLab CLI glab (managed external, MIT; host-first)",
		PreferHost: true,
		Long: `glab (gitlab-org/cli, MIT) fronts GitLab issues, merge requests and
pipelines with an explicit token (GITLAB_TOKEN or GITLAB_TOKEN_FILE).
bashy uses the host's own install when present; if absent it points at the
vendor install page rather than downloading. All args pass through to glab.
There is no ambient-account fallback, login never implies provisioning
permission, and glab has no dry-run: previews refuse without execution.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return glab.Resolve(ctx, version)
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
