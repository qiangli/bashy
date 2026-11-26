// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/artifact"
)

func selfOCICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "oci",
		Short:        "Protect OCI base/build images and run offline container sandboxes",
		SilenceUsage: true,
	}
	cmd.AddCommand(
		selfOCICaptureCmd(),
		selfOCIRestoreCmd(),
		selfOCIBuildCmd(),
		selfOCILockStatusCmd(),
		selfOCIPacksCmd(),
	)
	return cmd
}

func resolveOCIArtifactRoot(explicit string) (string, error) {
	r := strings.TrimSpace(explicit)
	if r == "" {
		r = artifact.DefaultRoot()
	}
	return filepath.Abs(r)
}

func parseRetentionFlag(raw string) (artifact.RetentionClass, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "protected":
		return artifact.RetentionProtectedRoot, nil
	case "reconstructible":
		return artifact.RetentionReconstructible, nil
	case "scratch", "disposable":
		return artifact.RetentionScratch, nil
	default:
		return "", fmt.Errorf("self oci: invalid --retention %q (want protected, reconstructible, or scratch)", raw)
	}
}

func appendRootToLatestManifestIfPresent(ctx context.Context, backend artifact.Backend, root artifact.RootDescriptor) (uint64, error) {
	manifests, err := backend.ListManifests(ctx)
	if err != nil {
		return 0, err
	}
	if len(manifests) == 0 {
		return 0, nil
	}
	slices.SortFunc(manifests, func(a, b artifact.ManifestV1) int {
		if a.Generation < b.Generation {
			return -1
		}
		if a.Generation > b.Generation {
			return 1
		}
		return 0
	})
	latest := manifests[len(manifests)-1]
	nextRoots := make([]artifact.RootDescriptor, 0, len(latest.Roots)+1)
	replaced := false
	for _, r := range latest.Roots {
		if r.Kind == root.Kind && r.Name == root.Name && r.Version == root.Version {
			nextRoots = append(nextRoots, root)
			replaced = true
		} else {
			nextRoots = append(nextRoots, r)
		}
	}
	if !replaced {
		nextRoots = append(nextRoots, root)
	}
	next := latest
	next.Generation = latest.Generation + 1
	next.CreatedAt = nowUTC()
	next.Roots = nextRoots
	if err := backend.PutManifest(ctx, next); err != nil {
		return 0, err
	}
	return next.Generation, nil
}

type selfOCICaptureResult struct {
	ArtifactRoot    string                    `json:"artifact_root"`
	Ingest          *artifact.OCIIngestResult `json:"ingest"`
	AddedGeneration uint64                    `json:"added_generation,omitempty"`
}

func selfOCICaptureCmd() *cobra.Command {
	var name, version, refName, retentionStr, expectedDigest, artifactRoot, engine string
	var inputDigests []string
	var deterministic, updateManifest, asJSON bool

	cmd := &cobra.Command{
		Use:   "capture <oci-archive.tar | oci-dir | image-ref>",
		Short: "Ingest an OCI image manifest, config, and all layers into protected CAS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			target := strings.TrimSpace(args[0])
			artRoot, err := resolveOCIArtifactRoot(artifactRoot)
			if err != nil {
				return err
			}
			retention, err := parseRetentionFlag(retentionStr)
			if err != nil {
				return err
			}

			var archivePath, layoutDir string
			if fi, statErr := os.Stat(target); statErr == nil {
				if fi.IsDir() {
					layoutDir = target
				} else {
					archivePath = target
				}
			} else {
				// Save from local container engine into a temporary oci-archive tar.
				tmpTar, tErr := os.CreateTemp("", "bashy-self-oci-save-*.tar")
				if tErr != nil {
					return tErr
				}
				tmpPath := tmpTar.Name()
				_ = tmpTar.Close()
				defer os.Remove(tmpPath)

				engArgv, eErr := engineArgv(engine)
				if eErr != nil {
					return eErr
				}
				saveArgs := append(engArgv[1:], "image", "save", "--format", "oci-archive", "-o", tmpPath, target)
				saveCmd := exec.CommandContext(ctx, engArgv[0], saveArgs...)
				var stderr bytes.Buffer
				saveCmd.Stderr = &stderr
				if err := saveCmd.Run(); err != nil {
					return fmt.Errorf("self oci capture: save %s: %w: %s", target, err, strings.TrimSpace(stderr.String()))
				}
				archivePath = tmpPath
				if refName == "" {
					refName = target
				}
			}

			backend, err := artifact.ConnectOrOpen(ctx, artRoot)
			if err != nil {
				return err
			}
			defer backend.Close()

			var ingestRes *artifact.OCIIngestResult
			if deterministic || retention != artifact.RetentionProtectedRoot || expectedDigest != "" || len(inputDigests) > 0 {
				ingestRes, err = artifact.RegisterGeneratedOCIImage(ctx, backend, artifact.GeneratedOCIImageOptions{
					ArchivePath:    archivePath,
					LayoutDir:      layoutDir,
					LogicalName:    name,
					Version:        version,
					RefName:        refName,
					Deterministic:  deterministic,
					ExpectedDigest: expectedDigest,
					InputDigests:   inputDigests,
					Retention:      retention,
				})
			} else if layoutDir != "" {
				ingestRes, err = artifact.IngestOCILayout(ctx, backend, layoutDir, artifact.OCIIngestOptions{
					LogicalName: name,
					Version:     version,
					RefName:     refName,
					Retention:   artifact.RetentionProtectedRoot,
				})
			} else {
				ingestRes, err = artifact.IngestOCIArchive(ctx, backend, archivePath, artifact.OCIIngestOptions{
					LogicalName: name,
					Version:     version,
					RefName:     refName,
					Retention:   artifact.RetentionProtectedRoot,
				})
			}
			if err != nil {
				return err
			}

			var addedGen uint64
			if updateManifest && retention == artifact.RetentionProtectedRoot {
				addedGen, err = appendRootToLatestManifestIfPresent(ctx, backend, ingestRes.Root)
				if err != nil {
					return err
				}
			}

			res := selfOCICaptureResult{
				ArtifactRoot:    artRoot,
				Ingest:          ingestRes,
				AddedGeneration: addedGen,
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "captured %s@%s (%s, %d layers) -> %s\n",
				ingestRes.Root.Name, ingestRes.Root.Version, ingestRes.ManifestDigest, len(ingestRes.LayerDigests), artRoot)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Logical image name (e.g. bashy-base, bashy-build)")
	cmd.Flags().StringVar(&version, "version", "", "Logical image version or tag")
	cmd.Flags().StringVar(&refName, "ref", "", "Optional OCI image ref annotation")
	cmd.Flags().BoolVar(&deterministic, "deterministic", false, "Mark generated image as bit-for-bit reproducible")
	cmd.Flags().StringVar(&retentionStr, "retention", "protected", "Retention class: protected, reconstructible, or scratch")
	cmd.Flags().StringVar(&expectedDigest, "expected-digest", "", "Expected manifest sha256 digest for deterministic reconstructible builds")
	cmd.Flags().StringSliceVar(&inputDigests, "input-digest", nil, "Protected input blob digests for deterministic reconstructible builds")
	cmd.Flags().BoolVar(&updateManifest, "update-manifest", false, "Append captured protected root as a new manifest generation if one exists")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&engine, "engine", envOr("BASHY_OCI", ""), "Container engine override")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

type selfOCIRestoreResult struct {
	ArtifactRoot string                    `json:"artifact_root"`
	LoadedEngine bool                      `json:"loaded_engine"`
	EngineOutput string                    `json:"engine_output,omitempty"`
	Export       *artifact.OCIExportResult `json:"export"`
}

func selfOCIRestoreCmd() *cobra.Command {
	var outputTar, artifactRoot, engine string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "restore <ref-or-digest>",
		Short: "Restore an OCI image from protected CAS to an archive or local Podman storage without network",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			refOrDigest := strings.TrimSpace(args[0])
			artRoot, err := resolveOCIArtifactRoot(artifactRoot)
			if err != nil {
				return err
			}

			backend, err := artifact.ConnectOrOpen(ctx, artRoot)
			if err != nil {
				return err
			}
			defer backend.Close()

			tarPath := strings.TrimSpace(outputTar)
			removeTemp := false
			if tarPath == "" {
				tmp, tErr := os.CreateTemp("", "bashy-oci-restore-*.oci.tar")
				if tErr != nil {
					return tErr
				}
				tarPath = tmp.Name()
				_ = tmp.Close()
				removeTemp = true
				defer os.Remove(tarPath)
			}

			expRes, err := artifact.ExportOCIArchive(ctx, backend, refOrDigest, tarPath)
			if err != nil {
				return err
			}

			res := selfOCIRestoreResult{
				ArtifactRoot: artRoot,
				Export:       expRes,
			}
			if removeTemp {
				engArgv, eErr := engineArgv(engine)
				if eErr != nil {
					return eErr
				}
				loadArgs := append(engArgv[1:], "image", "load", "-q", "-i", expRes.OutputPath)
				loadCmd := exec.CommandContext(ctx, engArgv[0], loadArgs...)
				var stdout, stderr bytes.Buffer
				loadCmd.Stdout = &stdout
				loadCmd.Stderr = &stderr
				if err := loadCmd.Run(); err != nil {
					return fmt.Errorf("self oci restore: image load failed: %w: %s", err, strings.TrimSpace(stderr.String()))
				}
				res.LoadedEngine = true
				res.EngineOutput = strings.TrimSpace(stdout.String())
			}

			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			if res.LoadedEngine {
				fmt.Fprintf(cmd.OutOrStdout(), "restored %s into engine (%d layers)\n", expRes.ManifestDigest, len(expRes.LayerDigests))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", expRes.OutputPath, expRes.ManifestDigest)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&outputTar, "output-tar", "", "Write reconstructed oci-archive .tar to path instead of loading into engine")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&engine, "engine", envOr("BASHY_OCI", ""), "Container engine override")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

type selfOCIBuildResult struct {
	Tag          string                    `json:"tag"`
	BuildArgv    []string                  `json:"build_argv"`
	ArtifactRoot string                    `json:"artifact_root"`
	Captured     *artifact.OCIIngestResult `json:"captured,omitempty"`
}

func validateInternalLoopbackURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("self oci build: invalid --internal-proxy-url %q", raw)
	}
	if !containIsLoopbackHost(strings.ToLower(u.Hostname())) {
		return "", fmt.Errorf("%w: non-loopback proxy %q forbidden in offline OCI build", artifact.ErrSealedNetworkDenied, raw)
	}
	return trimmed, nil
}

func selfOCIBuildCmd() *cobra.Command {
	var tag, containerfile, artifactRoot, engine, retentionStr, expectedDigest, aptSnapshot, internalProxyURL string
	var inputDigests []string
	var deterministic, allowInternalProxy, noCapture, asJSON bool

	cmd := &cobra.Command{
		Use:   "build <context-dir>",
		Short: "Run a sealed offline container build (--pull=never --network=none) and record the image in CAS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ctxDir, err := filepath.Abs(strings.TrimSpace(args[0]))
			if err != nil {
				return err
			}
			artRoot, err := resolveOCIArtifactRoot(artifactRoot)
			if err != nil {
				return err
			}
			retention, err := parseRetentionFlag(retentionStr)
			if err != nil {
				return err
			}
			if !deterministic && retention == artifact.RetentionReconstructible {
				return fmt.Errorf("%w: --retention reconstructible requires --deterministic", artifact.ErrNondeterministicImageEviction)
			}

			proxyURL, err := validateInternalLoopbackURL(internalProxyURL)
			if err != nil {
				return err
			}
			if proxyURL != "" && !allowInternalProxy {
				return errors.New("self oci build: --internal-proxy-url requires --allow-internal-proxy")
			}

			if strings.TrimSpace(tag) == "" {
				tag = "localhost/bashy-sandbox:local"
			}

			engArgv, err := engineArgv(engine)
			if err != nil {
				return err
			}

			buildArgs := append([]string{}, engArgv[1:]...)
			buildArgs = append(buildArgs, "build", "--pull=never")
			if !allowInternalProxy {
				buildArgs = append(buildArgs, "--network=none")
			} else {
				buildArgs = append(buildArgs, "--network=host")
				if proxyURL != "" {
					buildArgs = append(buildArgs, "--build-arg", "GOPROXY="+proxyURL, "--build-arg", "GONOSUMDB=*")
				}
			}

			if strings.TrimSpace(aptSnapshot) != "" {
				backend, bErr := artifact.ConnectOrOpen(ctx, artRoot)
				if bErr != nil {
					return bErr
				}
				aptTmpDir, tErr := os.MkdirTemp("", "bashy-oci-apt-*")
				if tErr != nil {
					_ = backend.Close()
					return tErr
				}
				defer os.RemoveAll(aptTmpDir)
				_, rErr := artifact.RestoreVMAndHelperAssets(ctx, backend, artifact.HelperRestoreOptions{
					AptSnapshot: aptSnapshot,
					AptDestDir:  aptTmpDir,
				})
				_ = backend.Close()
				if rErr != nil {
					return fmt.Errorf("self oci build: restore apt snapshot %s: %w", aptSnapshot, rErr)
				}
				buildArgs = append(buildArgs, "-v", aptTmpDir+":/var/cache/bashy-apt:ro")
			}

			if strings.TrimSpace(containerfile) != "" {
				cfAbs, fErr := filepath.Abs(containerfile)
				if fErr != nil {
					return fErr
				}
				buildArgs = append(buildArgs, "-f", cfAbs)
			}
			buildArgs = append(buildArgs, "-t", tag, ctxDir)

			buildCmd := exec.CommandContext(ctx, engArgv[0], buildArgs...)
			var stderr bytes.Buffer
			buildCmd.Stdout = cmd.ErrOrStderr()
			buildCmd.Stderr = &stderr
			if err := buildCmd.Run(); err != nil {
				return fmt.Errorf("self oci build: %w: %s", err, strings.TrimSpace(stderr.String()))
			}

			fullArgv := append([]string{engArgv[0]}, buildArgs...)
			res := selfOCIBuildResult{
				Tag:          tag,
				BuildArgv:    fullArgv,
				ArtifactRoot: artRoot,
			}

			if !noCapture {
				tmpTar, tErr := os.CreateTemp("", "bashy-oci-built-*.tar")
				if tErr != nil {
					return tErr
				}
				tmpTarPath := tmpTar.Name()
				_ = tmpTar.Close()
				defer os.Remove(tmpTarPath)

				saveArgs := append(engArgv[1:], "image", "save", "--format", "oci-archive", "-o", tmpTarPath, tag)
				saveCmd := exec.CommandContext(ctx, engArgv[0], saveArgs...)
				var saveErr bytes.Buffer
				saveCmd.Stderr = &saveErr
				if err := saveCmd.Run(); err != nil {
					return fmt.Errorf("self oci build: save built image %s: %w: %s", tag, err, strings.TrimSpace(saveErr.String()))
				}

				backend, bErr := artifact.ConnectOrOpen(ctx, artRoot)
				if bErr != nil {
					return bErr
				}
				defer backend.Close()

				capRes, cErr := artifact.RegisterGeneratedOCIImage(ctx, backend, artifact.GeneratedOCIImageOptions{
					ArchivePath:    tmpTarPath,
					LogicalName:    tag,
					RefName:        tag,
					Deterministic:  deterministic,
					ExpectedDigest: expectedDigest,
					InputDigests:   inputDigests,
					Retention:      retention,
				})
				if cErr != nil {
					return cErr
				}
				res.Captured = capRes
			}

			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			if res.Captured != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "built %s (%s)\n", tag, res.Captured.ManifestDigest)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "built %s\n", tag)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&tag, "tag", "", "Image tag to build (default localhost/bashy-sandbox:local)")
	cmd.Flags().StringVar(&containerfile, "file", "", "Containerfile path")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().BoolVar(&deterministic, "deterministic", false, "Mark built image as bit-for-bit reproducible")
	cmd.Flags().StringVar(&retentionStr, "retention", "protected", "Retention class: protected, reconstructible, or scratch")
	cmd.Flags().StringVar(&expectedDigest, "expected-digest", "", "Expected manifest sha256 digest for deterministic reconstructible builds")
	cmd.Flags().StringSliceVar(&inputDigests, "input-digest", nil, "Protected input blob digests for deterministic reconstructible builds")
	cmd.Flags().StringVar(&aptSnapshot, "apt-snapshot", "", "Mount retained apt snapshot archive from CAS at /var/cache/bashy-apt:ro")
	cmd.Flags().BoolVar(&allowInternalProxy, "allow-internal-proxy", false, "Allow loopback-only local proxy access during build")
	cmd.Flags().StringVar(&internalProxyURL, "internal-proxy-url", "", "Loopback Go proxy URL when --allow-internal-proxy is set")
	cmd.Flags().BoolVar(&noCapture, "no-capture", false, "Skip saving built image back into artifact CAS")
	cmd.Flags().StringVar(&engine, "engine", envOr("BASHY_OCI", ""), "Container engine override")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func openReadOnlyArtifactBackendIfExists(ctx context.Context, artRoot string) (artifact.Backend, func(), error) {
	if client, ok := artifact.TryConnectService(ctx, artRoot); ok {
		return client, func() { _ = client.Close() }, nil
	}
	dbPath := filepath.Join(artRoot, "catalog.db")
	if fi, err := os.Stat(dbPath); err != nil || fi.IsDir() {
		return nil, func() {}, nil
	}
	st, err := artifact.OpenStore(ctx, artifact.StoreOptions{Root: artRoot, ReadOnly: true})
	if err != nil {
		return nil, func() {}, err
	}
	return st, func() { _ = st.Close() }, nil
}

func selfOCILockStatusCmd() *cobra.Command {
	var lockPath, artifactRoot string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "lock-status",
		Short: "Inspect tools/bashy-build/image.lock digests and toolchain/apt pins against local CAS",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			artRoot, err := resolveOCIArtifactRoot(artifactRoot)
			if err != nil {
				return err
			}
			lock, err := artifact.ParseBuildImageLock(lockPath)
			if err != nil {
				return err
			}
			backend, closeFn, err := openReadOnlyArtifactBackendIfExists(ctx, artRoot)
			if err != nil {
				return err
			}
			defer closeFn()

			st, err := artifact.InspectBuildImageLockStatus(ctx, backend, lock)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(st)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "lock: version=%s go=%s apt=%s base=%t image=%t go_cached=%t apt_cached=%t ready=%t\n",
				st.Lock.BashyVersion, st.Lock.GoVersion, st.Lock.AptSnapshot,
				st.BaseImagePresent, st.BuildImagePresent, st.GoToolchainPresent, st.AptArchivePresent, st.Ready)
			return nil
		},
	}

	cmd.Flags().StringVar(&lockPath, "lock", filepath.FromSlash("tools/bashy-build/image.lock"), "Path to image.lock")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

type selfOCIPacksResult struct {
	ProfileName       string                     `json:"profile_name"`
	Sealed            bool                       `json:"sealed"`
	MandatoryPacks    []string                   `json:"mandatory_packs"`
	ConfiguredPacks   []string                   `json:"configured_packs"`
	ConfigurablePacks []string                   `json:"configurable_packs"`
	VerifiedInCAS     []string                   `json:"verified_in_cas"`
	MissingInCAS      []artifact.MissingArtifact `json:"missing_in_cas"`
	CheckedPack       string                     `json:"checked_pack,omitempty"`
	CheckAllowed      bool                       `json:"check_allowed,omitempty"`
}

func resolveEffectiveOCIProfile(artRoot, profileSpec string) (artifact.ProfileV1, error) {
	if strings.TrimSpace(profileSpec) != "" {
		return artifact.ResolveProfile(profileSpec, artifact.CurrentPlatform())
	}
	if saved, ok, err := artifact.LoadSavedProfile(artRoot); err == nil && ok {
		return saved, nil
	}
	return artifact.SelfHostProfileV1(artifact.CurrentPlatform()), nil
}

func selfOCIPacksCmd() *cobra.Command {
	var profileSpec, artifactRoot, checkLang string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "packs",
		Short: "List installed sandbox language packs and verify sealed profile confinement",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			artRoot, err := resolveOCIArtifactRoot(artifactRoot)
			if err != nil {
				return err
			}
			profile, err := resolveEffectiveOCIProfile(artRoot, profileSpec)
			if err != nil {
				return err
			}
			sealed := artifact.IsSealed(artRoot)

			if strings.TrimSpace(checkLang) != "" {
				if err := artifact.CheckSandboxPackAllowed(profile, sealed, checkLang); err != nil {
					return err
				}
			}

			backend, closeFn, err := openReadOnlyArtifactBackendIfExists(ctx, artRoot)
			if err != nil {
				return err
			}
			defer closeFn()

			verified, missing, err := artifact.VerifyLanguagePacks(ctx, backend, profile, "")
			if err != nil {
				return err
			}
			if verified == nil {
				verified = []string{}
			}
			if missing == nil {
				missing = []artifact.MissingArtifact{}
			}

			res := selfOCIPacksResult{
				ProfileName:       profile.Name,
				Sealed:            sealed,
				MandatoryPacks:    slices.Clone(artifact.MandatoryLanguagePacks),
				ConfiguredPacks:   slices.Clone(profile.Languages),
				ConfigurablePacks: slices.Clone(artifact.ConfigurableLanguagePacks),
				VerifiedInCAS:     verified,
				MissingInCAS:      missing,
			}
			if strings.TrimSpace(checkLang) != "" {
				res.CheckedPack = strings.ToLower(strings.TrimSpace(checkLang))
				res.CheckAllowed = true
			}

			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "profile: %s  sealed: %t  configured: %s  verified_in_cas: %s\n",
				res.ProfileName, res.Sealed, strings.Join(res.ConfiguredPacks, ","), strings.Join(res.VerifiedInCAS, ","))
			return nil
		},
	}

	cmd.Flags().StringVar(&profileSpec, "profile", "", "Profile override (self-host, default, or JSON path)")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&checkLang, "check", "", "Verify whether a specific language pack is allowed under current seal state")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}
