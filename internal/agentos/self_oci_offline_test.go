// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/qiangli/yoke/pkg/artifact"
	"github.com/qiangli/yoke/pkg/broker/door"
)

const (
	fakePodmanStoreEnv = "BASHY_TEST_FAKE_PODMAN_STORE"
	fakePodmanLogEnv   = "BASHY_TEST_FAKE_PODMAN_LOG"
)

type fakeStoredImage struct {
	ManifestDigest string   `json:"manifest_digest"`
	Architecture   string   `json:"architecture"`
	Refs           []string `json:"refs"`
	ArchiveBytes   []byte   `json:"archive_bytes"`
}

type fakeEngineDB struct {
	Images map[string]fakeStoredImage `json:"images"` // manifestDigest -> image
}

func loadFakeEngineDB(storeDir string) (fakeEngineDB, error) {
	p := filepath.Join(storeDir, "images.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fakeEngineDB{Images: map[string]fakeStoredImage{}}, nil
		}
		return fakeEngineDB{}, err
	}
	var db fakeEngineDB
	if err := json.Unmarshal(raw, &db); err != nil {
		return fakeEngineDB{}, err
	}
	if db.Images == nil {
		db.Images = map[string]fakeStoredImage{}
	}
	return db, nil
}

func saveFakeEngineDB(storeDir string, db fakeEngineDB) error {
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(storeDir, "images.json"), raw, 0o600)
}

func matchFakeStoredImage(db fakeEngineDB, query string) (fakeStoredImage, bool) {
	q := strings.TrimSpace(query)
	if img, ok := db.Images[q]; ok {
		return img, true
	}
	if _, dgst, ok := strings.Cut(q, "@"); ok {
		if img, exists := db.Images[dgst]; exists {
			return img, true
		}
	}
	for _, img := range db.Images {
		if slices.Contains(img.Refs, q) {
			return img, true
		}
	}
	return fakeStoredImage{}, false
}

func parseArchiveForFakeEngine(archiveBytes []byte) (manifestDigest, arch string, refs []string, err error) {
	files := map[string][]byte{}
	tr := tar.NewReader(bytes.NewReader(archiveBytes))
	for {
		hdr, rErr := tr.Next()
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return "", "", nil, rErr
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		data, rErr := io.ReadAll(tr)
		if rErr != nil {
			return "", "", nil, rErr
		}
		files[name] = data
	}
	idxRaw, ok := files[ocispec.ImageIndexFile]
	if !ok {
		return "", "", nil, errors.New("missing index.json in oci-archive")
	}
	var idx ocispec.Index
	if err := json.Unmarshal(idxRaw, &idx); err != nil || len(idx.Manifests) == 0 {
		return "", "", nil, errors.New("invalid index.json in oci-archive")
	}
	mdesc := idx.Manifests[0]
	manifestDigest = string(mdesc.Digest)
	manifestRaw, ok := files["blobs/sha256/"+strings.TrimPrefix(manifestDigest, "sha256:")]
	if !ok || artifact.DigestBytes(manifestRaw) != manifestDigest {
		return "", "", nil, errors.New("manifest blob missing or corrupt in oci-archive")
	}
	var m ocispec.Manifest
	if err := json.Unmarshal(manifestRaw, &m); err != nil {
		return "", "", nil, err
	}
	cfgDigest := string(m.Config.Digest)
	cfgRaw, ok := files["blobs/sha256/"+strings.TrimPrefix(cfgDigest, "sha256:")]
	if !ok || artifact.DigestBytes(cfgRaw) != cfgDigest {
		return "", "", nil, errors.New("config blob missing or corrupt in oci-archive")
	}
	for _, l := range m.Layers {
		ld := string(l.Digest)
		lRaw, lOK := files["blobs/sha256/"+strings.TrimPrefix(ld, "sha256:")]
		if !lOK || artifact.DigestBytes(lRaw) != ld {
			return "", "", nil, fmt.Errorf("layer blob %s missing or corrupt in oci-archive", ld)
		}
	}
	var cfg ocispec.Image
	_ = json.Unmarshal(cfgRaw, &cfg)
	arch = cfg.Architecture
	if arch == "" {
		arch = "amd64"
	}
	for _, k := range []string{ocispec.AnnotationRefName, "io.dhnt.bashy.requested-ref"} {
		if v := strings.TrimSpace(mdesc.Annotations[k]); v != "" && !slices.Contains(refs, v) {
			refs = append(refs, v)
		}
	}
	return manifestDigest, arch, refs, nil
}

func makeOCIArchiveBytes(refName, arch string, layerPayloads []string) ([]byte, string, error) {
	blobs := map[string][]byte{}
	putBlob := func(b []byte) string {
		d := artifact.DigestBytes(b)
		blobs[strings.TrimPrefix(d, "sha256:")] = b
		return d
	}

	var layerDescs []ocispec.Descriptor
	var diffIDs []digest.Digest
	fixedTime := time.Unix(0, 0).UTC()

	for i, content := range layerPayloads {
		var lTar bytes.Buffer
		ltw := tar.NewWriter(&lTar)
		entryName := fmt.Sprintf("layer-%d.txt", i)
		_ = ltw.WriteHeader(&tar.Header{
			Name:     entryName,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(content)),
			ModTime:  fixedTime,
		})
		_, _ = ltw.Write([]byte(content))
		_ = ltw.Close()
		diffID := artifact.DigestBytes(lTar.Bytes())

		var gz bytes.Buffer
		gw := gzip.NewWriter(&gz)
		gw.ModTime = fixedTime
		_, _ = gw.Write(lTar.Bytes())
		_ = gw.Close()

		ld := putBlob(gz.Bytes())
		diffIDs = append(diffIDs, digest.Digest(diffID))
		layerDescs = append(layerDescs, ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageLayerGzip,
			Digest:    digest.Digest(ld),
			Size:      int64(gz.Len()),
		})
	}

	cfgObj := ocispec.Image{
		Platform: ocispec.Platform{OS: "linux", Architecture: arch},
		Config:   ocispec.ImageConfig{Entrypoint: []string{"/bashy"}, WorkingDir: "/work"},
		RootFS:   ocispec.RootFS{Type: "layers", DiffIDs: diffIDs},
	}
	cfgBytes, _ := json.Marshal(cfgObj)
	cfgDigest := putBlob(cfgBytes)

	manifestObj := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    digest.Digest(cfgDigest),
			Size:      int64(len(cfgBytes)),
		},
		Layers: layerDescs,
	}
	manifestBytes, _ := json.Marshal(manifestObj)
	manifestDigest := putBlob(manifestBytes)

	idxObj := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{
			{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    digest.Digest(manifestDigest),
				Size:      int64(len(manifestBytes)),
				Annotations: map[string]string{
					ocispec.AnnotationRefName: refName,
				},
			},
		},
	}
	idxBytes, _ := json.MarshalIndent(idxObj, "", "  ")
	layoutBytes, _ := json.Marshal(ocispec.ImageLayout{Version: ocispec.ImageLayoutVersion})

	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	writeReg := func(name string, b []byte) {
		_ = tw.WriteHeader(&tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(b)),
			ModTime:  fixedTime,
		})
		_, _ = tw.Write(b)
	}
	writeReg(ocispec.ImageLayoutFile, layoutBytes)
	writeReg(ocispec.ImageIndexFile, idxBytes)
	var hexKeys []string
	for k := range blobs {
		hexKeys = append(hexKeys, k)
	}
	slices.Sort(hexKeys)
	for _, k := range hexKeys {
		writeReg("blobs/sha256/"+k, blobs[k])
	}
	_ = tw.Close()
	return out.Bytes(), manifestDigest, nil
}

// TestFakePodmanCLI is invoked via `<agentos.test> -test.run=^TestFakePodmanCLI$ -- <podman-args...>`
// from the scratch shim script installed in BASHY_OCI during S4 tests.
func TestFakePodmanCLI(t *testing.T) {
	storeDir := os.Getenv(fakePodmanStoreEnv)
	if storeDir == "" {
		return // normal `go test` pass — no-op
	}
	sepIdx := slices.Index(os.Args, "--")
	if sepIdx < 0 || sepIdx+1 >= len(os.Args) {
		os.Exit(2)
	}
	args := os.Args[sepIdx+1:]
	if logFile := os.Getenv(fakePodmanLogEnv); logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintln(f, strings.Join(args, " "))
			_ = f.Close()
		}
	}

	db, err := loadFakeEngineDB(storeDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch {
	case len(args) >= 3 && args[0] == "image" && args[1] == "exists":
		if _, ok := matchFakeStoredImage(db, args[2]); ok {
			os.Exit(0)
		}
		os.Exit(1)

	case len(args) >= 2 && args[0] == "image" && args[1] == "load":
		inPath := ""
		for i := 2; i < len(args); i++ {
			if args[i] == "-i" && i+1 < len(args) {
				inPath = args[i+1]
				i++
			}
		}
		raw, err := os.ReadFile(inPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		dgst, arch, refs, err := parseArchiveForFakeEngine(raw)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		prev := db.Images[dgst]
		for _, r := range prev.Refs {
			if !slices.Contains(refs, r) {
				refs = append(refs, r)
			}
		}
		db.Images[dgst] = fakeStoredImage{
			ManifestDigest: dgst,
			Architecture:   arch,
			Refs:           refs,
			ArchiveBytes:   raw,
		}
		if err := saveFakeEngineDB(storeDir, db); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("Loaded image: %s\n", dgst)
		os.Exit(0)

	case len(args) >= 3 && args[0] == "image" && args[1] == "inspect":
		ref := args[len(args)-1]
		img, ok := matchFakeStoredImage(db, ref)
		if !ok {
			fmt.Fprintf(os.Stderr, "image not known: %s\n", ref)
			os.Exit(125)
		}
		fmt.Println(img.Architecture)
		os.Exit(0)

	case len(args) >= 3 && args[0] == "image" && args[1] == "save":
		outPath := ""
		ref := args[len(args)-1]
		for i := 2; i < len(args); i++ {
			if args[i] == "-o" && i+1 < len(args) {
				outPath = args[i+1]
				i++
			}
		}
		img, ok := matchFakeStoredImage(db, ref)
		if !ok {
			fmt.Fprintf(os.Stderr, "cannot save missing image %s\n", ref)
			os.Exit(125)
		}
		if err := os.WriteFile(outPath, img.ArchiveBytes, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)

	case len(args) >= 1 && args[0] == "build":
		tag := "localhost/built:latest"
		for i := 1; i < len(args); i++ {
			if (args[i] == "-t" || args[i] == "--tag") && i+1 < len(args) {
				tag = args[i+1]
				i++
			}
		}
		ctxDir := args[len(args)-1]
		cfData, _ := os.ReadFile(filepath.Join(ctxDir, "Containerfile"))
		archBytes, dgst, err := makeOCIArchiveBytes(tag, defaultImageArch(), []string{
			"built-offline:" + string(cfData),
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		db.Images[dgst] = fakeStoredImage{
			ManifestDigest: dgst,
			Architecture:   defaultImageArch(),
			Refs:           []string{tag},
			ArchiveBytes:   archBytes,
		}
		if err := saveFakeEngineDB(storeDir, db); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)

	case len(args) >= 1 && args[0] == "run":
		if !slices.Contains(args, "--network=none") {
			fmt.Fprintln(os.Stderr, "expected --network=none on sandbox run")
			os.Exit(125)
		}
		// Verify referenced image is present locally in fake engine store.
		foundImg := false
		for _, a := range args[1:] {
			if _, ok := matchFakeStoredImage(db, a); ok {
				foundImg = true
				break
			}
		}
		if !foundImg {
			fmt.Fprintln(os.Stderr, "image not in local engine storage")
			os.Exit(125)
		}
		fmt.Println("sandbox-offline-ok")
		os.Exit(0)

	case len(args) >= 1 && args[0] == "rm":
		os.Exit(0)

	case len(args) >= 1 && args[0] == "pull":
		fmt.Fprintln(os.Stderr, "ERR_EXTERNAL_PULL_INVOKED")
		os.Exit(125)

	default:
		fmt.Fprintf(os.Stderr, "unsupported fake podman command: %v\n", args)
		os.Exit(2)
	}
}

func setupFakePodmanShim(t *testing.T) (shimPath, storeDir, logFile string) {
	t.Helper()
	testExe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	base := t.TempDir()
	storeDir = filepath.Join(base, "podman-store")
	logFile = filepath.Join(base, "podman-calls.log")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakePodmanStoreEnv, storeDir)
	t.Setenv(fakePodmanLogEnv, logFile)

	if runtime.GOOS == "windows" {
		shimPath = filepath.Join(base, "fake-podman.bat")
		script := fmt.Sprintf("@echo off\r\n\"%s\" -test.run=^TestFakePodmanCLI$ -- %%*\r\n", testExe)
		if err := os.WriteFile(shimPath, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		shimPath = filepath.Join(base, "fake-podman")
		script := fmt.Sprintf("#!/bin/sh\nexec '%s' -test.run=^TestFakePodmanCLI$ -- \"$@\"\n", testExe)
		if err := os.WriteFile(shimPath, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("BASHY_OCI", shimPath)
	return shimPath, storeDir, logFile
}

func writeMinimalStaticLinuxELF(t *testing.T, arch string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bashy-static-linux")
	var hdr [64]byte
	// ELF64 little-endian static ET_EXEC header with 0 program headers (no PT_INTERP).
	copy(hdr[0:4], []byte{0x7f, 'E', 'L', 'F'})
	hdr[4] = byte(elf.ELFCLASS64)
	hdr[5] = byte(elf.ELFDATA2LSB)
	hdr[6] = byte(elf.EV_CURRENT)
	binary.LittleEndian.PutUint16(hdr[16:18], uint16(elf.ET_EXEC))
	mach := elf.EM_X86_64
	if arch == "arm64" {
		mach = elf.EM_AARCH64
	}
	binary.LittleEndian.PutUint16(hdr[18:20], uint16(mach))
	binary.LittleEndian.PutUint32(hdr[20:24], uint32(elf.EV_CURRENT))
	binary.LittleEndian.PutUint16(hdr[52:54], 64) // e_ehsize
	if err := os.WriteFile(p, hdr[:], 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSelfOCICaptureWipeAndOfflineContainSandboxRestore(t *testing.T) {
	_, artRoot, _ := isolateOfflineAgentOSEnv(t, t.TempDir())
	_, podmanStoreDir, podmanLog := setupFakePodmanShim(t)

	arch := defaultImageArch()
	archiveBytes, manifestDigest, err := makeOCIArchiveBytes("localhost/bashy-sandbox:v1.0.0", arch, []string{
		"layer-0-bashy-runtime",
		"layer-1-go-and-bashsharp",
	})
	if err != nil {
		t.Fatal(err)
	}
	srcTarPath := filepath.Join(t.TempDir(), "bashy-sandbox.oci.tar")
	if err := os.WriteFile(srcTarPath, archiveBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	// 1. Capture OCI archive into protected CAS via `bashy self oci capture`.
	capOut, err := runSelfCLI(t, "oci", "capture", srcTarPath,
		"--name", "bashy-sandbox",
		"--version", "v1.0.0",
		"--artifact-root", artRoot,
		"--json")
	if err != nil {
		t.Fatalf("self oci capture: %v\n%s", err, capOut)
	}
	var capRes selfOCICaptureResult
	if err := json.Unmarshal([]byte(capOut), &capRes); err != nil {
		t.Fatalf("decode capture json: %v", err)
	}
	if capRes.Ingest == nil || capRes.Ingest.ManifestDigest != manifestDigest {
		t.Fatalf("captured digest = %+v, want %s", capRes.Ingest, manifestDigest)
	}

	// Seal the artifact store!
	if err := artifact.SaveSealState(artRoot, artifact.SealStateV1{
		SchemaVersion:    artifact.SealStateSchemaV1,
		Sealed:           true,
		ActiveGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}

	// 2. Simulate complete Podman engine storage wipe!
	if err := os.RemoveAll(podmanStoreDir); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(srcTarPath)

	// 3. Run contained image call (`runContainedImage` -> `containEnsureImage`) in sealed mode:
	//    it must automatically restore the wiped image from protected CAS via `podman image load`,
	//    run with `--network=none`, and invoke ZERO `podman pull` commands!
	staticBashy := writeMinimalStaticLinuxELF(t, arch)
	pinnedRef := "localhost/bashy-sandbox@" + manifestDigest
	spec := &containImageSpec{
		Image:   pinnedRef,
		Workdir: "/work",
		Argv:    []string{"--bashsharp", "-c", "echo ok"},
		Environ: map[string]string{
			"BASHY_CONTAIN_BASHY": staticBashy,
			"BASHY_OCI":           os.Getenv("BASHY_OCI"),
		},
	}
	var stderr bytes.Buffer
	var ranArgv []string
	code := runContainedImage(spec, &stderr, func(argv []string) int {
		ranArgv = slices.Clone(argv)
		cmd := execCommandForTest(argv[0], argv[1:]...)
		if err := cmd.Run(); err != nil {
			return 125
		}
		return 0
	})
	if code != 0 {
		t.Fatalf("runContainedImage after Podman storage wipe failed (code=%d): %s", code, stderr.String())
	}
	if !slices.Contains(ranArgv, "--network=none") {
		t.Fatalf("expected --network=none in sandbox run argv: %v", ranArgv)
	}

	rawLog, err := os.ReadFile(podmanLog)
	if err != nil {
		t.Fatalf("read podman log: %v", err)
	}
	logStr := string(rawLog)
	if strings.Contains(logStr, "pull") {
		t.Fatalf("podman pull was invoked in sealed mode!\nlog:\n%s", logStr)
	}
	if !strings.Contains(logStr, "image load -q -i") {
		t.Fatalf("expected automatic `image load -q -i` restore from CAS, got log:\n%s", logStr)
	}
}

func TestContainEnsureImageRefusesUncachedInSealedModeWithoutPull(t *testing.T) {
	_, artRoot, _ := isolateOfflineAgentOSEnv(t, t.TempDir())
	_, _, podmanLog := setupFakePodmanShim(t)

	if err := artifact.SaveSealState(artRoot, artifact.SealStateV1{
		SchemaVersion:    artifact.SealStateSchemaV1,
		Sealed:           true,
		ActiveGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}

	uncachedRef := "registry.example.invalid/foreign/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err := containEnsureImage(uncachedRef)
	if !errors.Is(err, artifact.ErrSealedNetworkDenied) {
		t.Fatalf("containEnsureImage uncached in sealed mode = %v, want ErrSealedNetworkDenied", err)
	}

	rawLog, _ := os.ReadFile(podmanLog)
	if strings.Contains(string(rawLog), "pull") {
		t.Fatalf("podman pull MUST NEVER be invoked in sealed mode! log:\n%s", string(rawLog))
	}
}

func TestContainEgressSealedStripsExternalHostsPreservesLoopback(t *testing.T) {
	_, artRoot, _ := isolateOfflineAgentOSEnv(t, t.TempDir())
	t.Setenv(containAllowHostEnv, "https://api.external.example:8443/v1, 127.0.0.1:18888, localhost:19999")

	// Unsealed: external host is included in Remote.
	unsealedEg := loadContainEgress()
	if !slices.Contains(unsealedEg.Remote, "api.external.example:8443") {
		t.Fatalf("expected external host when unsealed, got %+v", unsealedEg)
	}

	// Sealed: ALL non-loopback Remote endpoints must be stripped while keeping door.Port() & loopback ports!
	if err := artifact.SaveSealState(artRoot, artifact.SealStateV1{
		SchemaVersion:    artifact.SealStateSchemaV1,
		Sealed:           true,
		ActiveGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}

	sealedEg := loadContainEgress()
	if len(sealedEg.Remote) != 0 {
		t.Fatalf("sealed loadContainEgress Remote = %v, want empty", sealedEg.Remote)
	}
	for _, wantPort := range []int{door.Port(), 18888, 19999} {
		if wantPort > 0 && !slices.Contains(sealedEg.LoopbackPorts, wantPort) {
			t.Fatalf("sealed loadContainEgress LoopbackPorts = %v, missing loopback port %d", sealedEg.LoopbackPorts, wantPort)
		}
	}
}

func TestSelfOCIBuildRestoreLockStatusAndPacksSealed(t *testing.T) {
	_, artRoot, _ := isolateOfflineAgentOSEnv(t, t.TempDir())
	_, podmanStoreDir, podmanLog := setupFakePodmanShim(t)

	// Seal offline mode.
	prof := artifact.SelfHostProfileV1(artifact.CurrentPlatform())
	if err := artifact.SaveProfile(artRoot, prof); err != nil {
		t.Fatal(err)
	}
	if err := artifact.SaveSealState(artRoot, artifact.SealStateV1{
		SchemaVersion:    artifact.SealStateSchemaV1,
		Sealed:           true,
		ActiveGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}

	// 1. `self oci build` in sealed mode passes --pull=never and --network=none, and captures built image into CAS!
	ctxDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(ctxDir, "Containerfile"), []byte("FROM scratch\nCOPY app /app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctxDir, "app"), []byte("sandbox-app"), 0o755); err != nil {
		t.Fatal(err)
	}

	buildOut, err := runSelfCLI(t, "oci", "build", ctxDir,
		"--tag", "localhost/offline-built:v1",
		"--artifact-root", artRoot,
		"--json")
	if err != nil {
		t.Fatalf("self oci build: %v\n%s", err, buildOut)
	}
	var buildRes selfOCIBuildResult
	if err := json.Unmarshal([]byte(buildOut), &buildRes); err != nil {
		t.Fatalf("decode buildRes: %v", err)
	}
	if !slices.Contains(buildRes.BuildArgv, "--pull=never") || !slices.Contains(buildRes.BuildArgv, "--network=none") {
		t.Fatalf("expected --pull=never and --network=none in BuildArgv, got %v", buildRes.BuildArgv)
	}
	if buildRes.Captured == nil || !artifact.ValidDigest(buildRes.Captured.ManifestDigest) {
		t.Fatalf("expected captured built image in CAS, got %+v", buildRes.Captured)
	}

	// Wipe fake Podman store and test `self oci restore` directly into engine!
	if err := os.RemoveAll(podmanStoreDir); err != nil {
		t.Fatal(err)
	}
	restoreOut, err := runSelfCLI(t, "oci", "restore", buildRes.Captured.ManifestDigest,
		"--artifact-root", artRoot,
		"--json")
	if err != nil {
		t.Fatalf("self oci restore: %v\n%s", err, restoreOut)
	}
	var restoreRes selfOCIRestoreResult
	if err := json.Unmarshal([]byte(restoreOut), &restoreRes); err != nil || !restoreRes.LoadedEngine {
		t.Fatalf("unexpected restoreRes: %+v (err=%v)", restoreRes, err)
	}

	// 2. Nondeterministic `--retention reconstructible` without `--deterministic` is refused!
	if _, err := runSelfCLI(t, "oci", "build", ctxDir,
		"--tag", "localhost/bad-reconstructible:v1",
		"--retention", "reconstructible",
		"--artifact-root", artRoot); !errors.Is(err, artifact.ErrNondeterministicImageEviction) {
		t.Fatalf("nondeterministic reconstructible build err = %v, want ErrNondeterministicImageEviction", err)
	}

	// 3. `self oci packs`: mandatory go & bashsharp allowed; unconfigured python refused with ErrSealedNetworkDenied!
	if _, err := runSelfCLI(t, "oci", "packs", "--artifact-root", artRoot, "--check", "go", "--json"); err != nil {
		t.Fatalf("packs --check go: %v", err)
	}
	if _, err := runSelfCLI(t, "oci", "packs", "--artifact-root", artRoot, "--check", "python"); !errors.Is(err, artifact.ErrSealedNetworkDenied) {
		t.Fatalf("packs --check python in sealed self-host profile = %v, want ErrSealedNetworkDenied", err)
	}

	// 4. `self oci lock-status` against repo's real tools/bashy-build/image.lock!
	lockFile := filepath.Join("..", "..", "tools", "bashy-build", "image.lock")
	lockOut, err := runSelfCLI(t, "oci", "lock-status", "--lock", lockFile, "--artifact-root", artRoot, "--json")
	if err != nil {
		t.Fatalf("self oci lock-status: %v\n%s", err, lockOut)
	}
	var lockSt artifact.BuildImageLockStatus
	if err := json.Unmarshal([]byte(lockOut), &lockSt); err != nil {
		t.Fatalf("decode lock-status: %v", err)
	}
	if lockSt.Lock.GoVersion == "" || !artifact.ValidDigest(lockSt.Lock.ImageDigest) {
		t.Fatalf("unexpected lock-status output: %+v", lockSt)
	}

	rawLog, _ := os.ReadFile(podmanLog)
	if strings.Contains(string(rawLog), "pull ") {
		t.Fatalf("unexpected pull in podman log:\n%s", string(rawLog))
	}
}

func execCommandForTest(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
