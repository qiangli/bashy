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
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/artifact"
)

const defaultManagedOllamaPort = 11435

func resolveModelStoreDir(explicit string) string {
	if s := strings.TrimSpace(explicit); s != "" {
		if abs, err := filepath.Abs(s); err == nil {
			return abs
		}
		return s
	}
	return genieModelStoreDir()
}

func resolveArtifactRootDir(explicit string) (string, error) {
	r := strings.TrimSpace(explicit)
	if r == "" {
		r = artifact.DefaultRoot()
	}
	return filepath.Abs(r)
}

func resolveManagedOllamaBaseURL(explicit string) string {
	if u := strings.TrimSpace(explicit); u != "" {
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return "http://" + u
		}
		return u
	}
	if u := strings.TrimSpace(os.Getenv("OLLAMA_HOST")); u != "" {
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return "http://" + u
		}
		return u
	}
	port := defaultManagedOllamaPort
	if v := strings.TrimSpace(os.Getenv("BASHY_OLLAMA_PORT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			port = n
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func resolveModelOfflineProfile(artRoot, profileSpec string) (artifact.ProfileV1, error) {
	spec := strings.TrimSpace(profileSpec)
	if spec != "" {
		return artifact.ResolveProfile(spec, artifact.CurrentPlatform())
	}
	if saved, ok, err := artifact.LoadSavedProfile(artRoot); err == nil && ok {
		return saved, nil
	}
	return artifact.ResolveProfile("default", artifact.CurrentPlatform())
}

type modelOfflineCaptureResult struct {
	SchemaVersion  string                  `json:"schema_version"`
	ArtifactRoot   string                  `json:"artifact_root"`
	ModelsDir      string                  `json:"models_dir"`
	Model          string                  `json:"model"`
	Root           artifact.RootDescriptor `json:"root"`
	ManifestGen    uint64                  `json:"manifest_generation,omitempty"`
	ManifestDigest string                  `json:"manifest_digest,omitempty"`
}

type modelOfflineRestoreResult struct {
	SchemaVersion string                  `json:"schema_version"`
	ArtifactRoot  string                  `json:"artifact_root"`
	ModelsDir     string                  `json:"models_dir"`
	Restored      []artifact.OllamaPackV1 `json:"restored"`
}

type modelOfflineEvalResult struct {
	SchemaVersion string                       `json:"schema_version"`
	ArtifactRoot  string                       `json:"artifact_root"`
	ProfileName   string                       `json:"profile_name"`
	BaseURL       string                       `json:"base_url"`
	Records       []artifact.ModelEvalRecordV1 `json:"records"`
}

type modelOfflineUnloadResult struct {
	SchemaVersion string   `json:"schema_version"`
	BaseURL       string   `json:"base_url"`
	Unloaded      []string `json:"unloaded"`
}

func newModelOfflineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Capture, restore, evaluate, unload, and place protected offline Ollama models",
	}
	cmd.AddCommand(
		newModelOfflineCaptureCmd(),
		newModelOfflineRestoreCmd(),
		newModelOfflineEvalCmd(),
		newModelOfflineUnloadCmd(),
		newModelOfflineTierCmd(),
	)
	return cmd
}

func newModelOfflineCaptureCmd() *cobra.Command {
	var modelsDir, artifactRoot string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "capture <model-tag>",
		Short: "Capture an Ollama model manifest and all layer blobs into protected CAS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			artRoot, err := resolveArtifactRootDir(artifactRoot)
			if err != nil {
				return err
			}
			mDir := resolveModelStoreDir(modelsDir)

			backend, err := artifact.ConnectOrOpen(cmd.Context(), artRoot)
			if err != nil {
				return err
			}
			defer backend.Close()

			rootDesc, err := artifact.CaptureOllamaModel(cmd.Context(), backend, mDir, args[0], nil)
			if err != nil {
				return err
			}
			updatedManifest, err := artifact.AppendManifestRoots(cmd.Context(), backend, rootDesc)
			if err != nil {
				return err
			}

			ref, _ := artifact.ParseOllamaModelRef(args[0])
			res := modelOfflineCaptureResult{
				SchemaVersion: "bashy-model-offline-capture-v1",
				ArtifactRoot:  artRoot,
				ModelsDir:     mDir,
				Model:         ref.Canonical(),
				Root:          rootDesc,
			}
			if updatedManifest != nil {
				res.ManifestGen = updatedManifest.Generation
				res.ManifestDigest, _ = updatedManifest.Digest()
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "captured %s (%s, %d blobs) -> %s\n",
				res.Model, rootDesc.Digest, len(rootDesc.Deps), artRoot)
			return nil
		},
	}
	cmd.Flags().StringVar(&modelsDir, "models-dir", "", "Ollama models directory (defaults to managed OLLAMA_MODELS)")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func openReadModelBackend(ctx context.Context, artRoot string) (artifact.Backend, error) {
	if client, ok := artifact.TryConnectService(ctx, artRoot); ok {
		return client, nil
	}
	return artifact.OpenStore(ctx, artifact.StoreOptions{Root: artRoot, ReadOnly: true})
}

func newModelOfflineRestoreCmd() *cobra.Command {
	var all, asJSON bool
	var modelsDir, artifactRoot string

	cmd := &cobra.Command{
		Use:   "restore [<model-tag>]",
		Short: "Restore pinned Ollama model(s) from CAS into OLLAMA_MODELS without network access",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && len(args) > 0 {
				return errors.New("--all cannot be combined with a <model-tag> argument")
			}
			if !all && len(args) == 0 {
				return errors.New("specify <model-tag> or --all")
			}
			artRoot, err := resolveArtifactRootDir(artifactRoot)
			if err != nil {
				return err
			}
			mDir := resolveModelStoreDir(modelsDir)

			backend, err := openReadModelBackend(cmd.Context(), artRoot)
			if err != nil {
				return err
			}
			defer backend.Close()

			var restored []artifact.OllamaPackV1
			if all {
				restored, err = artifact.RestoreAllPinnedModels(cmd.Context(), backend, mDir)
				if err != nil {
					return err
				}
			} else {
				pack, rErr := artifact.RestoreOllamaModel(cmd.Context(), backend, mDir, args[0])
				if rErr != nil {
					return rErr
				}
				restored = []artifact.OllamaPackV1{*pack}
			}

			res := modelOfflineRestoreResult{
				SchemaVersion: "bashy-model-offline-restore-v1",
				ArtifactRoot:  artRoot,
				ModelsDir:     mDir,
				Restored:      restored,
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "restored %d model(s) into %s\n", len(restored), mDir)
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Restore all pinned Ollama models")
	cmd.Flags().StringVar(&modelsDir, "models-dir", "", "Target Ollama models directory")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func newModelOfflineEvalCmd() *cobra.Command {
	var profileSpec, baseURL, modelsDir, artifactRoot string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run hardware readiness and deterministic capability evaluations for local models",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			artRoot, err := resolveArtifactRootDir(artifactRoot)
			if err != nil {
				return err
			}
			prof, err := resolveModelOfflineProfile(artRoot, profileSpec)
			if err != nil {
				return err
			}
			mDir := resolveModelStoreDir(modelsDir)
			targetURL := resolveManagedOllamaBaseURL(baseURL)

			backend, err := artifact.ConnectOrOpen(cmd.Context(), artRoot)
			if err != nil {
				return err
			}
			defer backend.Close()

			records, evalErr := artifact.EvaluateOllamaProfile(cmd.Context(), artifact.EvaluateOllamaOptions{
				Backend:      backend,
				Profile:      prof,
				BaseURL:      targetURL,
				ModelsDir:    mDir,
				ArtifactRoot: artRoot,
			})
			if evalErr != nil {
				return evalErr
			}

			res := modelOfflineEvalResult{
				SchemaVersion: "bashy-model-offline-eval-v1",
				ArtifactRoot:  artRoot,
				ProfileName:   prof.Name,
				BaseURL:       targetURL,
				Records:       records,
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			for _, r := range records {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (%s): hardware_ready=%t passed=%s\n",
					r.Model, r.Role, r.HardwareReady, strings.Join(r.PassedCapabilities, ","))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileSpec, "profile", "", "Profile to evaluate (default, workstation, self-host, or JSON path)")
	cmd.Flags().StringVar(&baseURL, "url", "", "Local Ollama HTTP base URL")
	cmd.Flags().StringVar(&modelsDir, "models-dir", "", "Ollama models directory")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func newModelOfflineUnloadCmd() *cobra.Command {
	var all, asJSON bool
	var baseURL string

	cmd := &cobra.Command{
		Use:   "unload [<model-tag>]",
		Short: "Unload model(s) from RAM/VRAM without deleting protected weights on disk or in CAS",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && len(args) > 0 {
				return errors.New("--all cannot be combined with a <model-tag> argument")
			}
			if !all && len(args) == 0 {
				return errors.New("specify <model-tag> or --all")
			}
			targetURL := resolveManagedOllamaBaseURL(baseURL)

			var unloaded []string
			var err error
			if all {
				unloaded, err = artifact.UnloadAllLoadedModels(cmd.Context(), targetURL, nil)
				if err != nil {
					return err
				}
			} else {
				if err := artifact.UnloadOllamaModelMemory(cmd.Context(), targetURL, args[0], nil); err != nil {
					return err
				}
				unloaded = []string{args[0]}
			}

			res := modelOfflineUnloadResult{
				SchemaVersion: "bashy-model-offline-unload-v1",
				BaseURL:       targetURL,
				Unloaded:      unloaded,
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "unloaded %d model(s) from memory (%s)\n", len(unloaded), strings.Join(unloaded, ", "))
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Unload all currently loaded models reported by /api/ps")
	cmd.Flags().StringVar(&baseURL, "url", "", "Local Ollama HTTP base URL")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func buildTierStatusMap(ctx context.Context, artRoot string, availableTiersFlag []string, flagChanged bool) (map[artifact.Tier]artifact.TierStatus, error) {
	hw, _ := artifact.DefaultHardwareProbe(ctx, artRoot)
	availMem := hw.AvailableMemoryBytes
	if availMem == 0 {
		availMem = 16 << 30
	}

	providesSandbox := func(t artifact.Tier) bool {
		switch t {
		case artifact.TierCloud, artifact.TierCluster, artifact.TierSphere, artifact.TierSandbox:
			return true
		default:
			return false
		}
	}

	status := make(map[artifact.Tier]artifact.TierStatus, 6)
	for _, t := range artifact.DefaultTierOrder() {
		status[t] = artifact.TierStatus{
			Available:         false,
			UnavailableReason: "unavailable",
		}
	}

	if !flagChanged {
		// Default local host readiness: workplace and userland are live on the current host.
		for _, t := range []artifact.Tier{artifact.TierWorkplace, artifact.TierUserland} {
			status[t] = artifact.TierStatus{
				Available:            true,
				OS:                   runtime.GOOS,
				Arch:                 runtime.GOARCH,
				ProvidesSandbox:      false,
				AvailableMemoryBytes: availMem,
			}
		}
		return status, nil
	}

	for _, raw := range availableTiersFlag {
		for _, tok := range strings.Split(raw, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" || strings.EqualFold(tok, "none") {
				continue
			}
			tier := artifact.Tier(tok)
			if !tier.Valid() {
				return nil, fmt.Errorf("unknown tier %q in --available-tiers", tok)
			}
			status[tier] = artifact.TierStatus{
				Available:            true,
				OS:                   runtime.GOOS,
				Arch:                 runtime.GOARCH,
				ProvidesSandbox:      providesSandbox(tier),
				AvailableMemoryBytes: availMem,
			}
		}
	}
	return status, nil
}

func newModelOfflineTierCmd() *cobra.Command {
	var profileSpec, artifactRoot, targetOS, targetArch string
	var requireSandbox, requireGPU, asJSON bool
	var minMemBytes uint64
	var availableTiers []string

	cmd := &cobra.Command{
		Use:   "tier",
		Short: "Evaluate confinement-preserving tier placement across the offline floor policy",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			artRoot, err := resolveArtifactRootDir(artifactRoot)
			if err != nil {
				return err
			}
			prof, err := resolveModelOfflineProfile(artRoot, profileSpec)
			if err != nil {
				return err
			}

			flagChanged := cmd.Flags().Changed("available-tiers")
			status, err := buildTierStatusMap(cmd.Context(), artRoot, availableTiers, flagChanged)
			if err != nil {
				return err
			}

			req := artifact.WorkloadRequirements{
				TargetOS:       strings.TrimSpace(targetOS),
				TargetArch:     strings.TrimSpace(targetArch),
				RequireSandbox: requireSandbox,
				RequireGPU:     requireGPU,
				MemoryBytes:    minMemBytes,
			}
			decision, err := artifact.ResolveWorkloadTier(prof, artRoot, req, status)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(decision)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "selected_tier: %s  sealed: %t  floor: %s\n",
				decision.SelectedTier, decision.Sealed, decision.Floor)
			for _, line := range decision.FallbackTrace {
				fmt.Fprintf(cmd.OutOrStdout(), "  skipped: %s\n", line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileSpec, "profile", "", "Profile to evaluate (default, workstation, self-host, or JSON path)")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().BoolVar(&requireSandbox, "require-sandbox", false, "Require sandbox confinement (never falls through to unconstrained userland)")
	cmd.Flags().StringVar(&targetOS, "target-os", "", "Required target OS")
	cmd.Flags().StringVar(&targetArch, "target-arch", "", "Required target architecture")
	cmd.Flags().BoolVar(&requireGPU, "require-gpu", false, "Require GPU capability")
	cmd.Flags().Uint64Var(&minMemBytes, "min-mem-bytes", 0, "Minimum available memory bytes required")
	cmd.Flags().StringSliceVar(&availableTiers, "available-tiers", nil, "Comma-separated list of currently available tiers (or 'none')")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}
