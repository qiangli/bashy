// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/qiangli/yoke/pkg/otelquery"
	"github.com/spf13/cobra"
)

func engineReleaseRepo() string {
	if r := strings.TrimSpace(os.Getenv("BASHY_ENGINE_REPO")); r != "" {
		return r
	}
	return "qiangli/bashy"
}

func engineCacheDir() string {
	d, err := binmgr.CacheDir()
	if err != nil {
		return ""
	}
	return d
}

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
		return "", fmt.Errorf(
			"otel stack binary unavailable for this platform "+
				"(checked bashy's managed cache and %s releases) — "+
				"set $BASHY_OTEL_BIN to a locally built one: %w",
			spec.ReleaseRepo, err)
	}
	return p, nil
}

func execOtel(bin string, args []string) error {
	c := exec.Command(bin, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func newOtelCmd() *cobra.Command {
	cmd := otelquery.NewCommand()
	adapter.DisableCompletion(cmd)
	cmd.AddCommand(&cobra.Command{
		Use:                "serve",
		Short:              "Start the OTEL stack (runs the provisioned `otel` binary)",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			bin, err := resolveOtelBinary()
			if err != nil {
				return err
			}
			return execOtel(bin, append([]string{"serve"}, args...))
		},
	})
	return cmd
}

func run(args []string) int {
	cmd := newOtelCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		if !otelquery.ErrorAlreadyPrinted(err) {
			fmt.Fprintln(os.Stderr, "bashy otel:", err)
		}
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:]))
}
