// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_obs

package agentos

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/binmgr"
	"github.com/qiangli/coreutils/pkg/otelquery"
)

// dispatchObs (default lean build) serves the otel query verbs in-process and
// delegates `serve` to the provisioned single `otel` binary.
//
// The stack embeds ~193 MB of victoria/collector machinery, so linking it into
// every worker binary is not viable — but a feature a shipped bashy advertises
// has to WORK on a shipped bashy. Telling the user to rebuild with
// `-tags bashy_obs` is not a feature, it is a dead end: the tag is invisible to
// anyone who installed a release, and the remedy it names is one they cannot
// perform.
//
// So the stack is provisioned and exec'd exactly the way podman and ollama are
// — a separate artifact, fetched on first use, run as its own process. This is
// what docs/otel-victoria-single-binary-handoff.md asked for in acceptance
// criterion 4 ("bashy otel execs the provisioned single binary; drop the
// -tags bashy_obs in-process link").
func dispatchObs(arg string) {
	if arg != "otel" {
		return
	}
	cmd := otelquery.NewCommand()
	cmd.AddCommand(&cobra.Command{
		Use:                "serve",
		Short:              "Start the OTEL stack (runs the provisioned `otel` binary)",
		DisableFlagParsing: true, // flags belong to the otel binary, not to us
		RunE: func(_ *cobra.Command, args []string) error {
			bin, err := resolveOtelBinary()
			if err != nil {
				return err
			}
			return execOtel(bin, append([]string{"serve"}, args...))
		},
	})
	cmd.SetArgs(os.Args[2:])
	if err := cmd.Execute(); err != nil {
		if !otelquery.ErrorAlreadyPrinted(err) {
			fmt.Fprintln(os.Stderr, "bashy otel:", err)
		}
		os.Exit(1)
	}
	os.Exit(0)
}

// resolveOtelBinary returns the provisioned single-binary otel stack, fetching
// it on first use. Cache first, so the common path costs no network.
//
// $BASHY_OTEL_BIN overrides for development and for a host that builds its own
// stack — without it, testing a locally-built otel means publishing a release.
func resolveOtelBinary() (string, error) {
	if p := os.Getenv("BASHY_OTEL_BIN"); p != "" {
		return p, nil
	}
	if p := binmgr.CachedBinary("otel"); p != "" {
		return p, nil
	}
	spec := binmgr.ManagedSpec{
		Name:        "otel",
		DestDir:     engineCacheDir(),
		ReleaseRepo: engineReleaseRepo(),
		Log:         func(m string) { fmt.Fprintln(os.Stderr, m) },
	}
	p, err := binmgr.ProvisionManaged(context.Background(), spec)
	if err != nil || p == "" {
		// Name something the operator can act on; a build tag is not that.
		return "", fmt.Errorf(
			"otel stack binary unavailable for this platform "+
				"(checked bashy's managed cache and %s releases) — "+
				"set $BASHY_OTEL_BIN to a locally built one: %w",
			spec.ReleaseRepo, err)
	}
	return p, nil
}

// execOtel hands off to the otel binary with stdio forwarded, so `serve`
// behaves as though the stack were built in — the caller should not be able to
// tell it lives in a separate artifact.
func execOtel(bin string, args []string) error {
	c := exec.Command(bin, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}
