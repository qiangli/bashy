// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/mod/modfile"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/binmgr"
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
	cmd.AddCommand(selfFetchCmd(), selfBuildCmd(), selfInstallCmd(), selfCheckCmd(), selfImageCmd(), selfSeedCmd())
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
	var version, dir, seed string
	var source, service, userMode, systemMode bool
	cmd := &cobra.Command{Use: "install [path]", Short: "Install the bashy product and optional outpost service", Long: `Install bashy, outpost, bash and sh together. By default, use the four files
beside this executable (works offline). --version fetches a verified release.
--dir selects the install directory; [path] selects the bashy executable path.
--source preserves the developer-only single-binary build/install workflow.`, Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if dir != "" && len(args) > 0 {
			return errors.New("--dir and a target path are mutually exclusive")
		}
		if userMode && systemMode {
			return errors.New("--user and --system are mutually exclusive")
		}
		if (userMode || systemMode) && !service {
			return errors.New("--user and --system require --service")
		}
		if source && service {
			return errors.New("--source builds only bashy; use the paired release to install a service")
		}
		if service && selfInstallEUID() == 0 {
			return errors.New("refusing --service as root: a root install writes root-owned files the non-root service user cannot use.\n" +
				"Install as the regular user, then register the service:\n" +
				"  bashy self install --service --user\n" +
				"  or: bashy self install && sudo ~/.local/bin/outpost service install --system --run-as <user>")
		}
		if seed != "" {
			if err := binmgr.ImportSeed(cmd.Context(), seed); err != nil {
				return err
			}
		}
		target := ""
		if len(args) == 1 {
			target = args[0]
		}
		if dir != "" {
			target = filepath.Join(dir, releaseBinaryName())
		}
		target, err := resolveSelfInstallTarget(target)
		if err != nil {
			return err
		}
		if source {
			if !cmd.Flags().Changed("version") {
				version = "dev"
			}
			tmp, err := os.MkdirTemp("", "bashy-self-build-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			cached := filepath.Join(tmp, releaseBinaryName())
			if err := buildSelfBinary(cmd.Context(), cached, version); err != nil {
				return err
			}
			if err := installExecutable(cached, target); err != nil {
				return err
			}
		} else {
			if filepath.Base(target) != releaseBinaryName() {
				return errors.New("paired install target must be named " + releaseBinaryName() + "; use --dir")
			}
			var members map[string]string
			if !cmd.Flags().Changed("version") && os.Getenv("BASHY_SELF_VERSION") == "" {
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				members, err = adjacentProduct(exe)
				if err != nil {
					return err
				}
			}
			if members == nil {
				tool, err := resolveBashyRelease(cmd.Context(), version)
				if err != nil {
					return err
				}
				members, err = binmgr.EnsureMembers(cmd.Context(), tool, productMemberNames())
				if err != nil {
					return err
				}
			}
			if err := installProduct(cmd.Context(), members, target, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
				return err
			}
		}
		if service {
			args := []string{"service", "install"}
			if userMode {
				args = append(args, "--user")
			}
			if systemMode {
				args = append(args, "--system")
			}
			child := exec.CommandContext(cmd.Context(), filepath.Join(filepath.Dir(target), binmgr.BinaryName("outpost")), args...)
			child.Stdin, child.Stdout, child.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
			if err := child.Run(); err != nil {
				return fmt.Errorf("register service: %w", err)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "installed %s\n", target)
		return nil
	}}
	cmd.Flags().StringVar(&version, "version", envOr("BASHY_SELF_VERSION", "latest"), "Release tag to fetch instead of using adjacent files")
	cmd.Flags().StringVar(&dir, "dir", "", "Install all product executables into this directory")
	cmd.Flags().StringVar(&seed, "seed", "", "Import a verified offline tool seed before installation")
	cmd.Flags().BoolVar(&source, "source", false, "Build only bashy from the current source checkout")
	cmd.Flags().BoolVar(&service, "service", false, "Register the installed outpost service")
	cmd.Flags().BoolVar(&userMode, "user", false, "Register a per-user service")
	cmd.Flags().BoolVar(&systemMode, "system", false, "Register a system service")
	return cmd
}

// selfInstallEUID is a seam so the root refusal is testable unprivileged.
var selfInstallEUID = os.Geteuid

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
	ldflags := "-s -w -X github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-" + version +
		" -X github.com/qiangli/bashy/internal/cli.buildID=" + selfBuildID(ctx)
	if version := selfShellRuntimeStamp(); version != "" {
		ldflags += " -X github.com/bashsharp/bashsharp/transpile.ShellRuntimeCommit=" + version
	}
	c := exec.CommandContext(ctx, exe, "go", "build", "-trimpath", "-ldflags", ldflags, "-o", target, "./cmd/bashy")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func selfBuildID(ctx context.Context) string {
	if _, err := os.Stat(".git"); err != nil {
		return ""
	}
	if err := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return ""
	}
	out, err := exec.CommandContext(ctx, "git", "describe", "--tags", "--exact-match", "HEAD").Output()
	if err != nil {
		out, err = exec.CommandContext(ctx, "git", "rev-parse", "--short=7", "HEAD").Output()
	}
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return ""
	}
	if exec.CommandContext(ctx, "git", "diff", "--quiet", "--ignore-submodules", "--").Run() != nil ||
		exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet", "--ignore-submodules", "--").Run() != nil {
		id += "-dirty"
	}
	return id
}

// selfShellRuntimeStamp is the sh fork version bashy pins in go.mod
// (replace mvdan.cc/sh/v3 => github.com/qiangli/sh/v3 VERSION): the
// transpiler writes it verbatim into a standalone program's go.mod.
func selfShellRuntimeStamp() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return ""
	}
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return ""
	}
	for _, r := range f.Replace {
		if r.Old.Path == "mvdan.cc/sh/v3" && r.New.Path == "github.com/qiangli/sh/v3" {
			return r.New.Version
		}
	}
	return ""
}

func ensureBashyRelease(ctx context.Context, version string) (string, error) {
	tool, err := resolveBashyRelease(ctx, version)
	if err != nil {
		return "", err
	}
	members, err := binmgr.EnsureMembers(ctx, tool, productMemberNames())
	if err != nil {
		return "", err
	}
	return members[releaseBinaryName()], nil
}

func resolveBashyRelease(ctx context.Context, version string) (binmgr.Tool, error) {
	if strings.TrimSpace(version) == "" {
		version = "latest"
	}
	return binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
		Name:           "bashy",
		RequireArchive: true,
		Repo:           bashyReleaseRepo,
		Version:        version,
		Member:         releaseBinaryName(),
		AssetMatch:     bashyArchiveMatch,
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
	if !strings.HasPrefix(n, "bashy-") || strings.HasPrefix(n, scratchAssetPrefix) {
		return false // bashy-scratch-linux-<arch> is the image artifact (self image), not a shell release
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
		// Windows refuses to replace a running image ("Access is denied") —
		// and with no path given, dst IS the bashy that is running this
		// command. A running image may still be RENAMED, so move it aside and
		// put the new file in its place (the Windows self-update idiom).
		if runtime.GOOS != "windows" {
			return err
		}
		if err := replaceAside(tmpName, dst); err != nil {
			return err
		}
	}
	removeTmp = false
	return nil
}

// replaceAside installs tmp at dst by renaming the existing dst to a
// sibling `.old` name first. The `.old` left behind is the image that may
// still be executing; it is removed on the next install (a stale one that
// is no longer running deletes fine, one that is still running gets a
// unique name instead). If the second rename fails the original is put
// back, so dst never disappears.
func replaceAside(tmp, dst string) error {
	old := dst + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		// still running from a previous upgrade — park this one beside it
		f, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".old-*")
		if err != nil {
			return err
		}
		old = f.Name()
		_ = f.Close()
		_ = os.Remove(old)
	}
	if err := os.Rename(dst, old); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Rename(old, dst)
		return err
	}
	return nil
}

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
