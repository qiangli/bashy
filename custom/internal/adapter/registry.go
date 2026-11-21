// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package adapter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qiangli/yoke/external/notrack"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
)

// RegistryEntry mirrors yoke/external/registry.Entry so each standalone
// registry CLI adapter under bashy/custom/<cmd> builds independently without
// importing yoke/external/registry (which registers and links every other CLI).
type RegistryEntry struct {
	Name           string
	Tier           int
	License        string
	Synopsis       string
	Long           string
	EnvVersion     string
	DefaultVersion string
	Resolve        func(ctx context.Context, version string) (binmgr.Tool, error)
	ExecEnv        func() []string
	PreferHost     bool
}

// Ensure provisions or locates the managed external binary with identical
// semantics to yoke/external/registry.Entry.Ensure (PreferHost, cache-first,
// fail-closed PinnedSHA256 verification).
func (e RegistryEntry) Ensure(ctx context.Context) (string, error) {
	version := ""
	if e.EnvVersion != "" {
		version = strings.TrimSpace(os.Getenv(e.EnvVersion))
	}
	if version == "" {
		version = e.DefaultVersion
	}
	if version == "" {
		if e.PreferHost {
			if p, err := exec.LookPath(e.Name); err == nil {
				return p, nil
			}
		} else if p := binmgr.CachedBinary(e.Name); p != "" {
			return p, nil
		}
	}
	tool, err := e.Resolve(ctx, version)
	if err != nil {
		return "", fmt.Errorf("%s: %w", e.Name, err)
	}
	if !e.PreferHost {
		if _, ok := binmgr.PinnedSHA256(tool.Name, tool.Version, binmgr.Platform()); !ok {
			return "", fmt.Errorf("registry: %s %s has no committed sha256 pin for %s — refusing unverified download (fail-closed)", tool.Name, tool.Version, binmgr.Platform())
		}
	}
	return binmgr.Ensure(ctx, tool)
}

// NewCmd builds the transparent passthrough Cobra command identical to
// yoke/external/registry.Entry.NewCmd.
func (e RegistryEntry) NewCmd() *cobra.Command {
	short := e.Synopsis
	long := e.Long
	if long == "" {
		long = e.Synopsis
	}
	return &cobra.Command{
		Use:                e.Name,
		Short:              short,
		Long:               long,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			bin, err := e.Ensure(cmd.Context())
			if err != nil {
				return err
			}
			c := binmgr.Command(cmd.Context(), bin, args...)
			if e.ExecEnv != nil {
				c.Env = notrack.Env(e.ExecEnv())
			} else {
				c.Env = notrack.Env(os.Environ())
			}
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			return c.Run()
		},
	}
}
