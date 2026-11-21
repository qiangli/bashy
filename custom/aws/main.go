package main

import (
	"context"
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/aws"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	return adapter.RegistryEntry{
		Name:       "aws",
		Tier:       6,
		License:    aws.License,
		Synopsis:   "AWS CLI v2 (managed external, Apache-2.0; host-first)",
		PreferHost: true,
		Long: `aws (Amazon Web Services CLI v2, Apache-2.0) fronts EC2, S3 and IAM
with an explicit profile (AWS_PROFILE) plus a credential source. bashy
uses the host's own install when present; if absent it points at the vendor
installer rather than downloading. All args pass through to aws. There is
no ambient-profile fallback and login never implies provisioning
permission. EC2/S3 mutations preview with --dry-run/--dryrun; other dry
runs refuse without execution.`,
		Resolve: func(ctx context.Context, version string) (binmgr.Tool, error) {
			return aws.Resolve(ctx, version)
		},
	}.NewCmd()
}

func main() {
	os.Exit(adapter.RunCobra(newCmd(), "", nil))
}
