package main

import (
	"context"
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/gcloud"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:       "gcloud",
		Tier:       6,
		License:    gcloud.License,
		Synopsis:   "Google Cloud SDK CLI (managed external, Apache-2.0; host-first)",
		PreferHost: true,
		Long: `gcloud (Google Cloud SDK, Apache-2.0) is Google's cloud CLI. bashy uses the
host's own install when present — the vendor installer + 'gcloud components
update' is the trusted, self-updating path; if absent it points at the official
installer rather than downloading the SDK. All args pass through to gcloud.
Authenticate with 'bashy gcloud auth login'.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return gcloud.Resolve(ctx, version)
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
