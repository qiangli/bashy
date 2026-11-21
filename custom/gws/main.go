package main

import (
	"context"
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/gws"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:       "gws",
		Tier:       6,
		License:    gws.License,
		Synopsis:   "Google Workspace CLI gws (managed external, Apache-2.0; host-first, pre-v1)",
		EnvVersion: "GWS_VERSION",
		PreferHost: true,
		Long: `gws (googleworkspace/cli, Apache-2.0) fronts Drive, Gmail, Calendar, Sheets,
Docs, Chat, Slides, Tasks and Contacts with explicit OAuth or service-account
credentials. $GWS_VERSION pins the release (reviewed: ` + gws.PinnedVersion + `);
unset uses the host's own gws. All args pass through to gws.

gws is pre-v1 and may break between releases, and it is NOT an officially
supported Google product. Gmail watch and Events subscribe/renew go through the
opt-in durable event adapters (live renewal unverified); Admin Directory CRUD is
unclaimed until tested.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			if version == "" {
				version = gws.PinnedVersion
			}
			return binmgr.ResolveGitHub(ctx, gws.Spec(version))
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
