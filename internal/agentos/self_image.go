// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/yoke/pkg/binmgr"
)

// `bashy self image` — the offline bashy image, from the release download alone.
//
// The release ships the static `bashy_scratch` linux artifact
// (bashy-scratch-linux-{amd64,arm64}, in checksums.txt). This verb fetches the
// one matching the running version through the same checksum→cache path as
// `self fetch`, writes a FROM-scratch Containerfile around it, and builds
// through `bashy podman` — the engine bashy provisions for itself
// (engines_podman.go). No source checkout, no git, no go, no host engine.
//
//	bashy self image
//	bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<ver>-linux-<arch> --bashsharp ./script.bsh
//
// Overrides: --version (release tag; default the running version, then its
// -dev prerelease), --arch, --tag, BASHY_SCRATCH_BIN (a local artifact, the
// repo/CI form — `dag build-image`), BASHY_OCI (another engine command).

const scratchAssetPrefix = "bashy-scratch-linux-"

func selfImageCmd() *cobra.Command {
	var version, arch, tag, engine string
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Build the offline FROM-scratch image of a released bashy through bashy podman",
		Long: `bashy self image fetches the static linux artifact of a bashy release
(bashy-scratch-linux-<arch>, checksum-verified into bashy's cache), wraps it in a
FROM-scratch Containerfile and builds localhost/bashy:<version>-linux-<arch>
through bashy's own podman. Run your script offline with:

  bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<version>-linux-<arch> --bashsharp ./script.bsh

BASHY_SCRATCH_BIN=<path> builds from a local artifact instead of fetching one
(what the repo's dag build-image target does); BASHY_OCI overrides the engine.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return buildSelfImage(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), version, arch, tag, engine)
		},
	}
	cmd.Flags().StringVar(&version, "version", envOr("BASHY_SELF_VERSION", ""), "release tag of the artifact (default: the running version)")
	cmd.Flags().StringVar(&arch, "arch", defaultImageArch(), "image architecture: amd64 or arm64")
	cmd.Flags().StringVar(&tag, "tag", "", "image tag (default localhost/bashy:<version>-linux-<arch>)")
	cmd.Flags().StringVar(&engine, "engine", envOr("BASHY_OCI", ""), "engine command (default: this bashy's podman)")
	return cmd
}

// defaultImageArch is the host's arch (the one podman runs natively, including
// inside the macOS/Windows machine); anything else is emulated or refused.
func defaultImageArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	default:
		return "amd64"
	}
}

func buildSelfImage(ctx context.Context, stdout, stderr io.Writer, version, arch, tag, engine string) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("self image: --arch must be amd64 or arm64, got %q", arch)
	}
	artifact, version, err := scratchArtifact(ctx, stderr, version, arch)
	if err != nil {
		return err
	}
	if tag == "" {
		tag = "localhost/bashy:" + strings.TrimPrefix(version, "v") + "-linux-" + arch
	}
	ctxDir, err := os.MkdirTemp("", "bashy-image-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(ctxDir)
	if err := copyFileMode(artifact, filepath.Join(ctxDir, "bashy"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(ctxDir, "tmp", "bashy"), 0o1777); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(ctxDir, "Containerfile"), []byte(scratchContainerfile(version)), 0o644); err != nil {
		return err
	}
	argv, err := engineArgv(engine)
	if err != nil {
		return err
	}
	argv = append(argv, "build", "--platform", "linux/"+arch, "-t", tag, ctxDir)
	fmt.Fprintf(stderr, "bashy self image: building %s from %s\n", tag, filepath.Base(artifact))
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, stderr, stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("self image: %s build failed: %w", filepath.Base(argv[0]), err)
	}
	fmt.Fprintln(stdout, tag)
	fmt.Fprintf(stderr, "run a script offline:\n  %s run --rm --network=none -v \"$PWD:/work\" -w /work %s --bashsharp ./script.bsh\n",
		strings.Join(engineDisplay(argv[:len(argv)-6]), " "), tag)
	return nil
}

// scratchContainerfile is the whole image: one static binary, a writable /tmp,
// /work for the user's mount. HOME is a dir OF its own under /tmp, not /tmp
// itself: Stage 0 output canonicalization rewrites the home prefix to `$HOME`
// on non-tty sinks (output_reduce.go), and a container's stdout is never a tty
// — with HOME=/tmp every /tmp path a script printed would come out as $HOME.
// Telemetry off — there is nowhere to send it from --network=none.
func scratchContainerfile(version string) string {
	return "FROM scratch\n" +
		"COPY bashy /bashy\n" +
		"COPY tmp /tmp\n" +
		"ENV HOME=/tmp/bashy PATH=/ OTEL_TRACES_EXPORTER=none BASHY_TELEMETRY_QUIET=1\n" +
		"WORKDIR /work\n" +
		"LABEL org.opencontainers.image.title=\"bashy\" org.opencontainers.image.version=\"" + version + "\" org.opencontainers.image.source=\"https://github.com/" + bashyReleaseRepo + "\"\n" +
		"ENTRYPOINT [\"/bashy\"]\n"
}

// scratchArtifact returns the static linux artifact to put in the image and
// the version it belongs to: $BASHY_SCRATCH_BIN when set (local build), else
// the release asset for version (default: the running version, falling back
// to its -dev prerelease while a release is still on its candidate tag).
func scratchArtifact(ctx context.Context, stderr io.Writer, version, arch string) (string, string, error) {
	if local := strings.TrimSpace(os.Getenv("BASHY_SCRATCH_BIN")); local != "" {
		if !isExecutable(local) {
			return "", "", fmt.Errorf("self image: BASHY_SCRATCH_BIN=%s is not an executable file", local)
		}
		if version == "" {
			version = firstNonEmpty(cli.BashyVersion(), "dev")
		}
		return local, version, nil
	}
	candidates := []string{version}
	if version == "" {
		v := cli.BashyVersion()
		if v == "" {
			return "", "", fmt.Errorf("self image: this bashy has no release version stamp; pass --version or BASHY_SCRATCH_BIN")
		}
		candidates = []string{v, v + "-dev"}
	}
	var lastErr error
	for _, v := range candidates {
		tool, err := binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
			Name:    "bashy-scratch-" + arch,
			Repo:    bashyReleaseRepo,
			Version: v,
			AssetMatch: func(name, _, _ string) bool {
				return strings.EqualFold(name, scratchAssetPrefix+arch)
			},
		})
		if err != nil {
			lastErr = err
			continue
		}
		fmt.Fprintf(stderr, "bashy self image: artifact %s@%s\n", scratchAssetPrefix+arch, tool.Version)
		p, err := binmgr.Ensure(ctx, tool)
		if err != nil {
			return "", "", err
		}
		return p, tool.Version, nil
	}
	return "", "", fmt.Errorf("self image: no %s%s asset for %s: %w", scratchAssetPrefix, arch, strings.Join(candidates, " / "), lastErr)
}

// engineArgv is the build engine: BASHY_OCI/--engine split into words, else
// this very executable + "podman" (never a stale bashy elsewhere on PATH).
func engineArgv(engine string) ([]string, error) {
	if f := strings.Fields(engine); len(f) > 0 {
		return f, nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return []string{self, "podman"}, nil
}

// engineDisplay shortens the self-exec argv for the printed one-liner.
func engineDisplay(argv []string) []string {
	if len(argv) == 2 && argv[1] == "podman" {
		return []string{"bashy", "podman"}
	}
	return argv
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
