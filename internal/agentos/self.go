// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

const bashyReleaseRepo = "qiangli/bashy"

func selfCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "self",
		Short: "Manage bashy's own released binary",
		Long: `bashy self fetches and caches a released bashy binary using the same
download -> checksum -> cache path as bashy's managed external tools.

It does not replace the running executable unless you explicitly install to a
destination path.`,
		SilenceUsage: true,
	}
	cmd.AddCommand(selfFetchCmd(), selfBuildCmd(), selfInstallCmd(), selfCheckCmd())
	return cmd
}

func selfCheckCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check the self-contained bashy bootstrap/build path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := collectSelfChecks()
			warns := countDoctorWarnings(checks)
			if asJSON {
				b, _ := json.Marshal(map[string]any{
					"schema_version": "bashy-self-check-v1",
					"checks":         checks,
					"warnings":       warns,
				})
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}
			printDoctorChecks(cmd.OutOrStdout(), checks, "bashy self check")
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON")
	return cmd
}

func selfFetchCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Download and cache a released bashy binary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := ensureBashyRelease(cmd.Context(), version)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", envOr("BASHY_SELF_VERSION", "latest"), "Release tag to fetch (default latest)")
	return cmd
}

func selfInstallCmd() *cobra.Command {
	var version string
	var source bool
	cmd := &cobra.Command{
		Use:   "install [path]",
		Short: "Install bashy to a target path",
		Long: `Install bashy to PATH. By default this installs a cached release binary.
Pass --source to build from the current source checkout first. With no path,
install next to the currently running executable. The target is written via a
same-directory temp file and rename.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			target, err := resolveSelfInstallTarget(target)
			if err != nil {
				return err
			}
			cached := ""
			if source {
				if !cmd.Flags().Changed("version") {
					version = "dev"
				}
				tmpDir, err := os.MkdirTemp("", "bashy-self-build-*")
				if err != nil {
					return err
				}
				defer os.RemoveAll(tmpDir)
				cached = filepath.Join(tmpDir, releaseBinaryName())
				if err := buildSelfBinary(cmd.Context(), cached, version); err != nil {
					return err
				}
			} else {
				cached, err = ensureBashyRelease(cmd.Context(), version)
				if err != nil {
					return err
				}
			}
			if err := installExecutable(cached, target); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed %s\n", target)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", envOr("BASHY_SELF_VERSION", "latest"), "Release tag to install (default latest)")
	cmd.Flags().BoolVar(&source, "source", false, "Build from the current source checkout instead of installing a release")
	return cmd
}

func selfBuildCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "build [path]",
		Short: "Build bashy from the current source checkout",
		Long: `Build bashy from the current source checkout using this bashy binary's
managed Go toolchain. With no path, writes bin/bashy or bin/bashy.exe.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := selfBuildDefaultTarget()
			if len(args) == 1 {
				target = args[0]
			}
			target, err := filepath.Abs(target)
			if err != nil {
				return err
			}
			if err := buildSelfBinary(cmd.Context(), target, version); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), target)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", envOr("BASHY_SELF_BUILD_VERSION", "dev"), "Version suffix for the built binary")
	return cmd
}

func selfBuildDefaultTarget() string {
	return filepath.Join("bin", releaseBinaryName())
}

func buildSelfBinary(ctx context.Context, target, version string) error {
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return errors.New("cannot resolve current executable for managed Go build")
	}
	ldflags := "-s -w -X github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-" + version
	c := exec.CommandContext(ctx, exe, "go", "build", "-trimpath", "-ldflags", ldflags, "-o", target, "./cmd/bashy")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func ensureBashyRelease(ctx context.Context, version string) (string, error) {
	tool, err := resolveBashyRelease(ctx, version)
	if err != nil {
		return "", err
	}
	return binmgr.Ensure(ctx, tool)
}

func resolveBashyRelease(ctx context.Context, version string) (binmgr.Tool, error) {
	if strings.TrimSpace(version) == "" {
		version = "latest"
	}
	return binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
		Name:       "bashy",
		Repo:       bashyReleaseRepo,
		Version:    version,
		Member:     releaseBinaryName(),
		AssetMatch: bashyArchiveMatch,
	})
}

func releaseBinaryName() string {
	if runtime.GOOS == "windows" {
		return "bashy.exe"
	}
	return "bashy"
}

func bashyArchiveMatch(name, goos, goarch string) bool {
	n := strings.ToLower(name)
	if !strings.HasPrefix(n, "bashy-") {
		return false
	}
	if !strings.Contains(n, strings.ToLower(goos)) {
		return false
	}
	return strings.Contains(n, strings.ToLower(goarch))
}

func resolveSelfInstallTarget(target string) (string, error) {
	if strings.TrimSpace(target) != "" {
		return filepath.Abs(target)
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "", errors.New("cannot resolve current executable; pass an install path")
	}
	return exe, nil
}

func installExecutable(src, dst string) error {
	if src == "" || dst == "" {
		return errors.New("source and destination are required")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	removeTmp = false
	return nil
}

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
