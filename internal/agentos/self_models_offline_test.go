// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/yoke/pkg/artifact"
	"github.com/qiangli/yoke/pkg/prune"
)

func runModelCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newModelsResourcesCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func seedDiskOllamaModelForCLI(t *testing.T, modelsDir, modelRef string) (manifestPath string, manifestRaw []byte) {
	t.Helper()
	ref, err := artifact.ParseOllamaModelRef(modelRef)
	if err != nil {
		t.Fatal(err)
	}
	blobsDir := filepath.Join(modelsDir, "blobs")
	if err := os.MkdirAll(blobsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	putLayer := func(mt string, body []byte) artifact.OllamaManifestLayer {
		dgst := artifact.DigestBytes(body)
		fname, err := artifact.OllamaBlobFilename(dgst)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(blobsDir, fname), body, 0o644); err != nil {
			t.Fatal(err)
		}
		return artifact.OllamaManifestLayer{
			MediaType: mt,
			Digest:    dgst,
			Size:      int64(len(body)),
		}
	}

	cfg := putLayer("application/vnd.docker.container.image.v1+json", []byte(`{"model_format":"gguf","model_family":"qwen3"}`))
	weights := putLayer("application/vnd.ollama.image.model", bytes.Repeat([]byte("GGUF-"+ref.Canonical()), 128))
	tmpl := putLayer("application/vnd.ollama.image.template", []byte("{{ .Prompt }}"))
	params := putLayer("application/vnd.ollama.image.params", []byte(`{"num_ctx":4096,"temperature":0}`))

	raw, err := json.MarshalIndent(artifact.OllamaDiskManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.docker.distribution.manifest.v2+json",
		Config:        cfg,
		Layers:        []artifact.OllamaManifestLayer{weights, tmpl, params},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath = filepath.Join(modelsDir, filepath.FromSlash(ref.ManifestRelPath()))
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return manifestPath, raw
}

func startLocalOllamaSimServer(t *testing.T, modelsDir string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	loaded := map[string]bool{
		"qwen3.5:4b":           true,
		"qwen3-embedding:0.6b": true,
	}
	var unloadedLog []string

	hasModelManifestOnDisk := func(modelName string) bool {
		ref, err := artifact.ParseOllamaModelRef(modelName)
		if err != nil {
			return false
		}
		_, sErr := os.Stat(filepath.Join(modelsDir, filepath.FromSlash(ref.ManifestRelPath())))
		return sErr == nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !hasModelManifestOnDisk(req.Model) {
			http.Error(w, "model manifest missing from OLLAMA_MODELS", http.StatusNotFound)
			return
		}
		mu.Lock()
		loaded[req.Model] = true
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": req.Model,
			"done":  true,
			"message": map[string]any{
				"role": "assistant",
				"tool_calls": []map[string]any{
					{
						"function": map[string]any{
							"name":      "read_file",
							"arguments": map[string]any{"path": "main.go"},
						},
					},
				},
			},
		})
	})
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		modelName, _ := req["model"].(string)
		if ka, ok := req["keep_alive"]; ok {
			if f, isFloat := ka.(float64); isFloat && f == 0 {
				mu.Lock()
				delete(loaded, modelName)
				unloadedLog = append(unloadedLog, modelName)
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{"model": modelName, "done": true})
				return
			}
		}
		if !hasModelManifestOnDisk(modelName) {
			http.Error(w, "model manifest missing from OLLAMA_MODELS", http.StatusNotFound)
			return
		}
		mu.Lock()
		loaded[modelName] = true
		mu.Unlock()
		prompt, _ := req["prompt"].(string)
		respText := "func Add(a, b int) int { return a + b }"
		if strings.Contains(prompt, "undefined: fmt") {
			respText = "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"ready\") }"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":    modelName,
			"response": respText,
			"done":     true,
		})
	})
	mux.HandleFunc("/api/embed", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !hasModelManifestOnDisk(req.Model) {
			http.Error(w, "embedding manifest missing from OLLAMA_MODELS", http.StatusNotFound)
			return
		}
		mu.Lock()
		loaded[req.Model] = true
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": req.Model,
			"embeddings": [][]float64{
				{0.92, 0.31, 0.08},
				{0.90, 0.34, 0.09},
				{-0.50, 0.70, 0.45},
			},
		})
	})
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		var list []artifact.LoadedOllamaModel
		for name := range loaded {
			list = append(list, artifact.LoadedOllamaModel{Name: name, Model: name, Size: 4096})
		}
		mu.Unlock()
		slices.SortFunc(list, func(a, b artifact.LoadedOllamaModel) int { return strings.Compare(a.Name, b.Name) })
		_ = json.NewEncoder(w).Encode(map[string]any{"models": list})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(unloadedLog)
	}
}

func TestSelfModelsOfflineCaptureRestoreEvalSealPruneUnloadAndTier(t *testing.T) {
	hostGo, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	gorootOut, err := exec.Command(hostGo, "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	goVerOut, err := exec.Command(hostGo, "env", "GOVERSION").Output()
	if err != nil {
		t.Fatalf("go env GOVERSION: %v", err)
	}
	minSDK := makeMinimalGoSDKCopy(t, strings.TrimSpace(string(gorootOut)))
	hostGoVer := strings.TrimPrefix(strings.TrimSpace(string(goVerOut)), "go")

	base := t.TempDir()
	_, artRoot, _ := isolateOfflineAgentOSEnv(t, base)
	modelsDir := filepath.Join(base, "ollama-models")
	t.Setenv("OLLAMA_MODELS", modelsDir)
	t.Setenv("BASHY_BOOTSTRAP_GOROOT", minSDK)

	// Verify cobra_schema automatically picks up `model offline` subcommands via newModelsResourcesCmd().
	schemas := cobraSchemas("model")
	for _, wantVerb := range []string{
		"model offline",
		"model offline capture",
		"model offline restore",
		"model offline eval",
		"model offline unload",
		"model offline tier",
	} {
		if _, ok := schemas[wantVerb]; !ok {
			t.Fatalf("cobraSchemas(\"model\") missing %q", wantVerb)
		}
	}

	// 1. Seed minimal self-host source checkout and bootstrap generation 1.
	srcCheckout := filepath.Join(t.TempDir(), "minimal-src")
	if err := os.MkdirAll(filepath.Join(srcCheckout, "cmd", "bashy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcCheckout, "go.mod"), []byte("module example.com/bashy/min\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcCheckout, "cmd", "bashy", "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if bootOut, err := runSelfCLI(t,
		"bootstrap",
		"--profile", "self-host",
		"--artifact-root", artRoot,
		"--source", srcCheckout,
		"--go-version", hostGoVer,
		"--json",
	); err != nil {
		t.Fatalf("self bootstrap failed: %v\n%s", err, bootOut)
	}

	// 2. Seed synthetic Ollama manifests/blobs for DefaultProfileV1 and capture both via `bashy model offline capture`.
	mCodingPath, mCodingRaw := seedDiskOllamaModelForCLI(t, modelsDir, "qwen3.5:4b")
	mEmbedPath, mEmbedRaw := seedDiskOllamaModelForCLI(t, modelsDir, "qwen3-embedding:0.6b")

	for _, tag := range []string{"qwen3.5:4b", "qwen3-embedding:0.6b"} {
		capOut, err := runModelCLI(t, "offline", "capture", tag, "--models-dir", modelsDir, "--artifact-root", artRoot, "--json")
		if err != nil {
			t.Fatalf("model offline capture %s failed: %v\n%s", tag, err, capOut)
		}
		var capRes modelOfflineCaptureResult
		if err := json.Unmarshal([]byte(capOut), &capRes); err != nil {
			t.Fatalf("decode capture result: %v\n%s", err, capOut)
		}
		if capRes.ManifestGen == 0 || capRes.Root.Kind != "ollama-model" {
			t.Fatalf("expected capture to register additive manifest root, got %+v", capRes)
		}
	}

	// 3. Before `model offline eval`, `self seal --profile default` MUST fail (missing ollama-eval)!
	if out, err := runSelfCLI(t, "seal", "--profile", "default", "--artifact-root", artRoot); !errors.Is(err, artifact.ErrIncompleteClosure) {
		t.Fatalf("self seal --profile default before eval = (%v, %s), want ErrIncompleteClosure", err, out)
	}

	// 4. Wipe OLLAMA_MODELS completely and run `model offline restore --all --json`!
	if err := os.RemoveAll(modelsDir); err != nil {
		t.Fatal(err)
	}
	restOut, err := runModelCLI(t, "offline", "restore", "--all", "--models-dir", modelsDir, "--artifact-root", artRoot, "--json")
	if err != nil {
		t.Fatalf("model offline restore --all failed: %v\n%s", err, restOut)
	}
	var restRes modelOfflineRestoreResult
	if err := json.Unmarshal([]byte(restOut), &restRes); err != nil {
		t.Fatal(err)
	}
	if len(restRes.Restored) != 2 {
		t.Fatalf("expected 2 restored models, got %+v", restRes)
	}
	for p, want := range map[string][]byte{mCodingPath: mCodingRaw, mEmbedPath: mEmbedRaw} {
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("restored manifest %s mismatch (err=%v)", p, err)
		}
	}

	// 5. Run `model offline eval --profile default` against local simulated Ollama server, then `self seal --profile default`!
	srv, getUnloaded := startLocalOllamaSimServer(t, modelsDir)
	evalOut, err := runModelCLI(t,
		"offline", "eval",
		"--profile", "default",
		"--url", srv.URL,
		"--models-dir", modelsDir,
		"--artifact-root", artRoot,
		"--json",
	)
	if err != nil {
		t.Fatalf("model offline eval failed: %v\n%s", err, evalOut)
	}
	var evalRes modelOfflineEvalResult
	if err := json.Unmarshal([]byte(evalOut), &evalRes); err != nil {
		t.Fatal(err)
	}
	if len(evalRes.Records) != 2 {
		t.Fatalf("expected 2 eval records, got %+v", evalRes)
	}

	sealOut, err := runSelfCLI(t, "seal", "--profile", "default", "--artifact-root", artRoot, "--json")
	if err != nil {
		t.Fatalf("self seal --profile default after eval failed: %v\n%s", err, sealOut)
	}
	var sealSt artifact.SealStateV1
	if err := json.Unmarshal([]byte(sealOut), &sealSt); err != nil {
		t.Fatal(err)
	}
	if !sealSt.Sealed || sealSt.ProfileName != "default" {
		t.Fatalf("unexpected seal state: %+v", sealSt)
	}

	// 6. Unload memory (`model offline unload --all`), add scratch item, run `self prune --artifacts --apply`,
	// simulate restart with wiped OLLAMA_MODELS + restore, and prove inference + embedding still work!
	unloadOut, err := runModelCLI(t, "offline", "unload", "--all", "--url", srv.URL, "--json")
	if err != nil {
		t.Fatalf("model offline unload --all failed: %v\n%s", err, unloadOut)
	}
	if got := getUnloaded(); len(got) != 2 {
		t.Fatalf("expected 2 models unloaded via keep_alive=0, got %v", got)
	}

	stWrite, err := artifact.OpenStore(context.Background(), artifact.StoreOptions{Root: artRoot})
	if err != nil {
		t.Fatal(err)
	}
	scratch := []byte("scratch-after-unload")
	if _, err := stWrite.PutArtifact(context.Background(), artifact.CatalogRecordV1{
		SchemaVersion:  artifact.CatalogRecordSchemaV1,
		Digest:         artifact.DigestBytes(scratch),
		Size:           int64(len(scratch)),
		MediaType:      "application/octet-stream",
		LogicalName:    "scratch/unload-test",
		LogicalVersion: "v1",
		Provenance:     "test",
		Retention:      artifact.RetentionScratch,
	}, bytes.NewReader(scratch)); err != nil {
		t.Fatal(err)
	}
	_ = stWrite.Close()

	pruneOut, err := runSelfCLI(t, "prune", "--artifacts", "--artifact-root", artRoot, "--apply", "--json")
	if err != nil {
		t.Fatalf("self prune --artifacts --apply failed: %v\n%s", err, pruneOut)
	}
	var pruneRes prune.Result
	if err := json.Unmarshal([]byte(pruneOut), &pruneRes); err != nil {
		t.Fatal(err)
	}
	if pruneRes.Counts.Removed != 1 {
		t.Fatalf("expected only 1 scratch item pruned (0 model weights removed), got %+v", pruneRes)
	}

	// Simulate OLLAMA_MODELS cleanup + restart restore, then re-verify local inference & embeddings!
	if err := os.RemoveAll(modelsDir); err != nil {
		t.Fatal(err)
	}
	if _, err := runModelCLI(t, "offline", "restore", "--all", "--models-dir", modelsDir, "--artifact-root", artRoot); err != nil {
		t.Fatalf("post-prune restore failed: %v", err)
	}
	if _, err := runModelCLI(t, "offline", "eval", "--profile", "default", "--url", srv.URL, "--models-dir", modelsDir, "--artifact-root", artRoot); err != nil {
		t.Fatalf("post-restart inference+embedding verification failed: %v", err)
	}

	// 7. `model offline tier`: verify sealed cloud exclusion, confinement preservation, and floor failure!
	tierOut, err := runModelCLI(t,
		"offline", "tier",
		"--artifact-root", artRoot,
		"--available-tiers", "cloud,workplace,userland",
		"--json",
	)
	if err != nil {
		t.Fatalf("model offline tier failed: %v\n%s", err, tierOut)
	}
	var tierDec artifact.TierPlacementDecision
	if err := json.Unmarshal([]byte(tierOut), &tierDec); err != nil {
		t.Fatal(err)
	}
	if !tierDec.Sealed || tierDec.SelectedTier != artifact.TierWorkplace || !slices.Contains(tierDec.ExcludedTiers, artifact.TierCloud) {
		t.Fatalf("unexpected sealed tier placement: %+v", tierDec)
	}

	// RequireSandbox refuses fallback to unconstrained workplace/userland!
	if _, err := runModelCLI(t,
		"offline", "tier",
		"--artifact-root", artRoot,
		"--require-sandbox",
		"--available-tiers", "cloud,workplace,userland",
	); !errors.Is(err, artifact.ErrConfinementViolation) {
		t.Fatalf("model offline tier --require-sandbox = %v, want ErrConfinementViolation", err)
	}

	// All tiers unavailable -> ErrOfflineFloorUnavailable!
	if _, err := runModelCLI(t,
		"offline", "tier",
		"--artifact-root", artRoot,
		"--available-tiers", "none",
	); !errors.Is(err, artifact.ErrOfflineFloorUnavailable) {
		t.Fatalf("model offline tier --available-tiers=none = %v, want ErrOfflineFloorUnavailable", err)
	}
}
