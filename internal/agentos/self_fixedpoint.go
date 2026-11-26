// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/artifact"
	"github.com/qiangli/yoke/pkg/gomod"
)

func selfFixedPointCmd() *cobra.Command {
	var prevBashy, sourceDir, targetPkg, version, buildID, lockPath, artifactRoot, loomDir, outputBinary string
	var smokeArgs []string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "fixed-point",
		Short: "Run the 3-stage offline self-hosting fixed-point release gate",
		Long: `Execute the 3-stage sealed offline self-hosting fixed-point gate:
  Stage 0: --prev-bashy builds candidate_0 with empty GOCACHE and GOMODCACHE
  Stage 1: candidate_0 builds candidate_1 with empty GOCACHE and GOMODCACHE
  Stage 2: candidate_1 builds candidate_2 with empty GOCACHE and GOMODCACHE

Fails closed unless candidate_1 and candidate_2 are byte-for-byte identical.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			artRoot, err := resolveOCIArtifactRoot(artifactRoot)
			if err != nil {
				return err
			}
			lDir := strings.TrimSpace(loomDir)
			if lDir == "" {
				lDir = gomod.DefaultLoomRepoDir(artRoot)
			} else if abs, lErr := filepath.Abs(lDir); lErr == nil {
				lDir = abs
			}

			srcDir, err := resolveBootstrapSourceDir(sourceDir)
			if err != nil {
				return err
			}

			effectiveLock := strings.TrimSpace(lockPath)
			if !cmd.Flags().Changed("lock") {
				cand := filepath.Join(srcDir, filepath.FromSlash("tools/bashy-build/image.lock"))
				if isRegularFile(cand) {
					effectiveLock = cand
				} else {
					effectiveLock = ""
				}
			}

			startProxy := func(pctx context.Context, aRoot, lmDir string) (string, func() error, error) {
				dbPath := filepath.Join(aRoot, "catalog.db")
				if fi, statErr := os.Stat(dbPath); statErr != nil || fi.IsDir() {
					return "off", nil, nil
				}
				srv, srvErr := gomod.StartProxyServer(pctx, gomod.ProxyServerOptions{
					Addr:         "127.0.0.1:0",
					ArtifactRoot: aRoot,
					LoomRepoDir:  lmDir,
					Sealed:       true,
				})
				if srvErr != nil {
					// Fall back to read-only store if the artifact directory is mounted read-only.
					roStore, roErr := artifact.OpenStore(pctx, artifact.StoreOptions{Root: aRoot, ReadOnly: true})
					if roErr != nil {
						return "", nil, srvErr
					}
					srv2, srv2Err := gomod.StartProxyServer(pctx, gomod.ProxyServerOptions{
						Addr:                   "127.0.0.1:0",
						ArtifactRoot:           aRoot,
						LoomRepoDir:            lmDir,
						Backend:                roStore,
						CloseBackendOnShutdown: true,
						Sealed:                 true,
					})
					if srv2Err != nil {
						_ = roStore.Close()
						return "", nil, srv2Err
					}
					return srv2.URL(), srv2.Close, nil
				}
				return srv.URL(), srv.Close, nil
			}

			report, runErr := artifact.RunSelfHostFixedPoint(ctx, artifact.FixedPointOptions{
				PrevBashyExe:  prevBashy,
				SourceDir:     srcDir,
				TargetPackage: targetPkg,
				ArtifactRoot:  artRoot,
				LoomDir:       lDir,
				Version:       version,
				BuildID:       buildID,
				BuildLockPath: effectiveLock,
				StartProxy:    startProxy,
				SmokeArgs:     smokeArgs,
				OutputBinary:  outputBinary,
			})
			if asJSON && report != nil {
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(report)
				return runErr
			}
			if runErr != nil {
				return runErr
			}
			fmt.Fprintf(cmd.OutOrStdout(), "fixed-point verified: %s (%d bytes, %d ms, peak workspace %d bytes)\n",
				report.Candidate2SHA256, report.Candidate2Size, report.TotalDurationMS, report.PeakWorkspaceBytes)
			return nil
		},
	}

	cmd.Flags().StringVar(&prevBashy, "prev-bashy", "", "Previous/seed bashy executable for Stage 0 (default current executable)")
	cmd.Flags().StringVar(&sourceDir, "source", "", "Source checkout directory containing go.mod")
	cmd.Flags().StringVar(&targetPkg, "pkg", "./cmd/bashy", "Target main package to build")
	cmd.Flags().StringVar(&version, "version", "", "Candidate version stamp")
	cmd.Flags().StringVar(&buildID, "build-id", "", "Deterministic candidate build ID")
	cmd.Flags().StringVar(&lockPath, "lock", filepath.FromSlash("tools/bashy-build/image.lock"), "Build image lock path")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&loomDir, "loom-dir", "", "Durable bare Loom Git repository directory")
	cmd.Flags().StringVar(&outputBinary, "output-binary", "", "Write verified candidate_2 binary to this path")
	cmd.Flags().StringSliceVar(&smokeArgs, "smoke-arg", nil, "Smoke arguments passed to candidate_2 (default --version)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func selfRunnerStageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "workflow-helpers",
		Aliases:      []string{"runner-stage"},
		Short:        "Capture and materialize local workflow actions and runner helper scripts",
		SilenceUsage: true,
	}
	cmd.AddCommand(
		selfWorkflowHelpersCaptureCmd(),
		selfWorkflowHelpersRestoreCmd(),
	)
	return cmd
}

type selfWorkflowHelpersCaptureResult struct {
	ArtifactRoot    string                                `json:"artifact_root"`
	Capture         *artifact.WorkflowHelperCaptureResult `json:"capture"`
	AddedGeneration uint64                                `json:"added_generation,omitempty"`
}

func selfWorkflowHelpersCaptureCmd() *cobra.Command {
	var name, version, commitsFile, lockPath, artifactRoot string
	var updateManifest, asJSON bool

	cmd := &cobra.Command{
		Use:   "capture <dir>",
		Short: "Capture local workflow actions, runner helper scripts, and build locks into protected CAS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			srcDir, err := filepath.Abs(strings.TrimSpace(args[0]))
			if err != nil {
				return err
			}
			artRoot, err := resolveOCIArtifactRoot(artifactRoot)
			if err != nil {
				return err
			}
			backend, err := artifact.ConnectOrOpen(ctx, artRoot)
			if err != nil {
				return err
			}
			defer backend.Close()

			capRes, err := artifact.CaptureWorkflowHelpers(ctx, backend, artifact.WorkflowHelperCaptureOptions{
				Name:          name,
				Version:       version,
				SourceDir:     srcDir,
				CommitsFile:   commitsFile,
				BuildLockPath: lockPath,
			})
			if err != nil {
				return err
			}

			var addedGen uint64
			if updateManifest {
				addedGen, err = appendRootToLatestManifestIfPresent(ctx, backend, capRes.Root)
				if err != nil {
					return err
				}
			}

			res := selfWorkflowHelpersCaptureResult{
				ArtifactRoot:    artRoot,
				Capture:         capRes,
				AddedGeneration: addedGen,
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "captured %s@%s (%s, %d files) -> %s\n",
				capRes.Root.Name, capRes.Root.Version, capRes.Digest, capRes.FileCount, artRoot)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Logical workflow-helper bundle name")
	cmd.Flags().StringVar(&version, "version", "v1", "Logical workflow-helper version")
	cmd.Flags().StringVar(&commitsFile, "commits", "", "Optional commits.txt manifest path")
	cmd.Flags().StringVar(&lockPath, "lock", "", "Optional tools/bashy-build/image.lock path")
	cmd.Flags().BoolVar(&updateManifest, "update-manifest", false, "Append captured protected root as a new manifest generation if one exists")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func selfWorkflowHelpersRestoreCmd() *cobra.Command {
	var destDir, version, digest, artifactRoot string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "restore <name>",
		Short: "Materialize protected workflow helpers from CAS into a local workspace with SHA-256 verification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if strings.TrimSpace(destDir) == "" {
				return errors.New("self workflow-helpers restore: --dest is required")
			}
			artRoot, err := resolveOCIArtifactRoot(artifactRoot)
			if err != nil {
				return err
			}

			backend, closeFn, err := openReadOnlyArtifactBackendIfExists(ctx, artRoot)
			if err != nil {
				return err
			}
			if backend == nil {
				return fmt.Errorf("%w: no artifact store at %s", artifact.ErrNotFound, artRoot)
			}
			defer closeFn()

			matRes, err := artifact.MaterializeWorkflowHelpers(ctx, backend, artifact.WorkflowHelperMaterializeOptions{
				Name:    strings.TrimSpace(args[0]),
				Version: version,
				Digest:  digest,
				DestDir: destDir,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(matRes)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "restored %s@%s (%d files) -> %s\n",
				matRes.Name, matRes.Version, matRes.FileCount, matRes.DestDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&destDir, "dest", "", "Destination workspace/runner staging directory")
	cmd.Flags().StringVar(&version, "version", "", "Optional logical version override")
	cmd.Flags().StringVar(&digest, "digest", "", "Optional expected sha256:<64hex> digest pin")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}
