package dhnt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	runnerInfraExit      = 70
	runnerIncompleteExit = 0
	runnerLaunchExit     = 127
)

var podUIDRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type RunnerMetadata struct {
	Node   string
	PodUID string
}

// ExecuteTask runs one strict v2 task inside an opened workspace boundary and
// atomically writes its evidence record. File outputs are sealed with an
// immutable hard link; tree outputs are copied into a private staging tree,
// re-hashed, and atomically renamed into their content-addressed destination.
func ExecuteTask(ctx context.Context, workspace string, spec RunnerSpec, argv []string, metadata RunnerMetadata) (Run, error) {
	if err := spec.Validate(); err != nil {
		return Run{}, fmt.Errorf("runner spec: %w", err)
	}
	if !runnerSpecEqualArgv(spec, argv) {
		return Run{}, errors.New("runner argv does not exactly match the signed runner spec")
	}
	if !cleanAbsolutePath(filepath.ToSlash(workspace)) {
		return Run{}, errors.New("workspace: must be a clean absolute path")
	}
	if !kubeNameRE.MatchString(metadata.Node) || len(metadata.Node) > 253 {
		return Run{}, errors.New("executor node: malformed Kubernetes node name")
	}
	if !podUIDRE.MatchString(metadata.PodUID) {
		return Run{}, errors.New("pod UID: must be a lowercase Kubernetes UUID")
	}
	workspaceInfo, err := os.Lstat(workspace)
	if err != nil {
		return Run{}, err
	}
	if !workspaceInfo.IsDir() || workspaceInfo.Mode()&os.ModeSymlink != 0 {
		return Run{}, errors.New("workspace: must be a real directory, not a symlink")
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return Run{}, err
	}
	defer root.Close()

	started := time.Now().UTC()
	run := newRunnerRun(spec, metadata, started)
	finish := func(class ResultClass, exitCode int, commit *OutputCommit) (Run, error) {
		run.Result = Result{Class: class, ExitCode: intPtr(exitCode)}
		run.OutputCommit = commit
		run.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := publishRunEvidence(root, spec.EvidenceDirectory, metadata.PodUID, run); err != nil {
			return Run{}, err
		}
		return run, nil
	}

	if err := validateRunnerWorkspacePaths(root, spec); err != nil {
		return finish(ResultInfraFail, runnerInfraExit, nil)
	}
	stage, err := createRunnerStaging(root, metadata.PodUID)
	if err != nil {
		return Run{}, err
	}
	defer root.RemoveAll(stage)

	preexistingOutputs := make(map[string]bool, len(spec.Outputs))
	for _, artifact := range spec.Outputs {
		exists, err := verifyExistingDestination(root, artifact)
		if err != nil {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
		preexistingOutputs[artifact.Name] = exists
		if err := ensureParent(root, artifact.Path); err != nil {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
	}
	commitManifest, err := MarshalOutputCommitManifest(runnerArtifacts(spec.Outputs))
	if err != nil {
		return Run{}, err
	}
	commitExisted, err := verifyExistingBytes(root, spec.CommitManifestPath, commitManifest)
	if err != nil {
		return finish(ResultInfraFail, runnerInfraExit, nil)
	}
	if err := ensureParent(root, spec.CommitManifestPath); err != nil {
		return finish(ResultInfraFail, runnerInfraExit, nil)
	}
	if err := ensureEvidenceDirectory(root, spec.EvidenceDirectory); err != nil {
		return Run{}, err
	}
	evidencePath := path.Join(spec.EvidenceDirectory, metadata.PodUID+".json")
	if exists, err := rootPathExists(root, evidencePath); err != nil {
		return Run{}, err
	} else if exists {
		return Run{}, fmt.Errorf("evidence destination %q already exists", evidencePath)
	}

	for _, artifact := range spec.Inputs {
		digest, err := hashRootArtifact(root, artifact)
		if err != nil || digest != artifact.SHA256 {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
	}

	outputStage := make(map[string]string, len(spec.Outputs))
	if err := root.MkdirAll(path.Join(stage, "outputs"), 0o700); err != nil {
		return Run{}, err
	}
	for i, artifact := range sortedRunnerArtifacts(spec.Outputs) {
		outputStage[artifact.Name] = path.Join(stage, "outputs", fmt.Sprintf("%03d", i))
	}
	childEnv, err := runnerChildEnvironment(root, stage, spec, outputStage)
	if err != nil {
		return Run{}, err
	}
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir = filepath.Join(root.Name(), filepath.FromSlash(spec.WorkingDirectory))
	command.Env = childEnv
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	configureRunnerProcess(command)
	if ctx.Err() != nil {
		return finish(ResultCanceled, 130, nil)
	}
	if err := command.Start(); err != nil {
		if ctx.Err() != nil {
			return finish(ResultCanceled, 130, nil)
		}
		return finish(ResultInfraFail, runnerLaunchExit, nil)
	}
	waitErr := command.Wait()
	stopRunnerProcessGroup(command)
	if waitErr != nil {
		exitCode, signaled := runnerExitStatus(waitErr)
		if ctx.Err() != nil || signaled {
			if exitCode == 0 {
				exitCode = 130
			}
			return finish(ResultCanceled, exitCode, nil)
		}
		if exitCode < 1 {
			exitCode = 1
		}
		return finish(spec.NonzeroClass, exitCode, nil)
	}
	if ctx.Err() != nil {
		return finish(ResultCanceled, 130, nil)
	}
	for _, artifact := range spec.Inputs {
		digest, err := hashRootArtifact(root, artifact)
		if err != nil || digest != artifact.SHA256 {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
	}

	sealed := make(map[string]string, len(spec.Outputs))
	if err := root.MkdirAll(path.Join(stage, "sealed"), 0o700); err != nil {
		return Run{}, err
	}
	for i, artifact := range sortedRunnerArtifacts(spec.Outputs) {
		source := outputStage[artifact.Name]
		target := path.Join(stage, "sealed", fmt.Sprintf("%03d", i))
		digest, err := snapshotRootArtifact(root, artifact, source, target)
		if err != nil || digest != artifact.SHA256 {
			return finish(ResultIncomplete, runnerIncompleteExit, nil)
		}
		sealed[artifact.Name] = target
	}
	for _, artifact := range sortedRunnerArtifacts(spec.Outputs) {
		if preexistingOutputs[artifact.Name] {
			digest, err := hashRootArtifact(root, artifact)
			if err != nil || digest != artifact.SHA256 {
				return finish(ResultInfraFail, runnerInfraExit, nil)
			}
			continue
		}
		if exists, err := rootPathExists(root, artifact.Path); err != nil {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		} else if exists {
			_ = root.Remove(artifact.Path)
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
		switch artifact.Kind {
		case ArtifactFile:
			if err := root.Link(sealed[artifact.Name], artifact.Path); err != nil {
				return finish(ResultInfraFail, runnerInfraExit, nil)
			}
		case ArtifactTree:
			if err := root.Rename(sealed[artifact.Name], artifact.Path); err != nil {
				return finish(ResultInfraFail, runnerInfraExit, nil)
			}
		default:
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
		if err := syncRootParent(root, artifact.Path); err != nil {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
	}

	manifestStage := path.Join(stage, "output-commit.json")
	if err := writeSealedRootFile(root, manifestStage, commitManifest); err != nil {
		return Run{}, err
	}
	if commitExisted {
		exists, err := verifyExistingBytes(root, spec.CommitManifestPath, commitManifest)
		if err != nil || !exists {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
	} else {
		if exists, err := rootPathExists(root, spec.CommitManifestPath); err != nil {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		} else if exists {
			_ = root.Remove(spec.CommitManifestPath)
			_ = syncRootParent(root, spec.CommitManifestPath)
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
		if err := root.Link(manifestStage, spec.CommitManifestPath); err != nil {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
		if err := syncRootParent(root, spec.CommitManifestPath); err != nil {
			return finish(ResultInfraFail, runnerInfraExit, nil)
		}
	}
	commit, err := NewOutputCommit(run.Outputs)
	if err != nil {
		return Run{}, err
	}
	return finish(ResultPass, 0, &commit)
}

func newRunnerRun(spec RunnerSpec, metadata RunnerMetadata, started time.Time) Run {
	trace := sha256.Sum256([]byte(metadata.PodUID))
	return Run{
		Schema:     RunSchemaV2,
		Pipeline:   spec.Pipeline,
		Task:       spec.Task,
		Run:        "pod-" + metadata.PodUID,
		Source:     spec.Source,
		Inputs:     runnerArtifacts(spec.Inputs),
		Executor:   Executor{Node: metadata.Node, Backend: spec.Platform.Backend, OS: spec.Platform.OS, Arch: spec.Platform.Arch},
		Outputs:    runnerArtifacts(spec.Outputs),
		StartedAt:  started.Format(time.RFC3339Nano),
		FinishedAt: started.Format(time.RFC3339Nano),
		TraceID:    hex.EncodeToString(trace[:16]),
	}
}

func runnerArtifacts(items []RunnerArtifact) []Artifact {
	result := make([]Artifact, 0, len(items))
	for _, item := range items {
		result = append(result, item.Artifact())
	}
	return result
}

func createRunnerStaging(root *os.Root, podUID string) (string, error) {
	if err := ensureNoSymlinkPath(root, ".dhnt-staging", true); err != nil {
		return "", err
	}
	if err := root.MkdirAll(".dhnt-staging", 0o700); err != nil {
		return "", err
	}
	if err := ensureNoSymlinkPath(root, ".dhnt-staging", false); err != nil {
		return "", err
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	name := podUID + "-" + hex.EncodeToString(random)
	stage := path.Join(".dhnt-staging", name)
	if err := root.Mkdir(stage, 0o700); err != nil {
		return "", err
	}
	return stage, nil
}

func validateRunnerWorkspacePaths(root *os.Root, spec RunnerSpec) error {
	if err := ensureNoSymlinkPath(root, spec.WorkingDirectory, false); err != nil {
		return fmt.Errorf("working directory: %w", err)
	}
	for _, artifact := range spec.Inputs {
		if err := ensureNoSymlinkPath(root, artifact.Path, false); err != nil {
			return fmt.Errorf("input %q: %w", artifact.Name, err)
		}
	}
	for _, artifact := range spec.Outputs {
		if err := ensureNoSymlinkPath(root, artifact.Path, true); err != nil {
			return fmt.Errorf("output %q: %w", artifact.Name, err)
		}
	}
	if err := ensureNoSymlinkPath(root, spec.EvidenceDirectory, true); err != nil {
		return fmt.Errorf("evidence directory: %w", err)
	}
	if err := ensureNoSymlinkPath(root, spec.CommitManifestPath, true); err != nil {
		return fmt.Errorf("commit manifest: %w", err)
	}
	return nil
}

func ensureNoSymlinkPath(root *os.Root, name string, allowMissing bool) error {
	if name == "." {
		info, err := root.Stat(".")
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("workspace root is not a directory")
		}
		return nil
	}
	parts := strings.Split(name, "/")
	current := ""
	for i, part := range parts {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) && allowMissing {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%q: symlink components are forbidden", current)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("%q: path ancestor is not a directory", current)
		}
	}
	return nil
}

func hashRootFile(root *os.Root, name string) (string, error) {
	if err := ensureNoSymlinkPath(root, name, false); err != nil {
		return "", err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("artifact is not a regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return "", errors.New("artifact changed identity while being opened")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashRootArtifact(root *os.Root, artifact RunnerArtifact) (string, error) {
	switch {
	case artifact.Kind == ArtifactFile && artifact.DigestAlgorithm == DigestSHA256FileV1:
		return hashRootFile(root, artifact.Path)
	case artifact.Kind == ArtifactTree && artifact.DigestAlgorithm == DigestSHA256TreeV1:
		return hashRootTree(root, artifact.Path)
	default:
		return "", fmt.Errorf("unsupported artifact kind/digest combination %q/%q",
			artifact.Kind, artifact.DigestAlgorithm)
	}
}

func hashRootTree(root *os.Root, name string) (string, error) {
	if err := ensureNoSymlinkPath(root, name, false); err != nil {
		return "", err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("artifact is not a real directory")
	}
	var entries []treeEntry
	err = fs.WalkDir(root.FS(), name, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == name {
			return nil
		}
		relative := strings.TrimPrefix(current, name+"/")
		if err := validateTreePath(relative); err != nil {
			return err
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%q: tree artifacts reject symlinks", current)
		}
		if entryInfo.IsDir() {
			return nil
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("%q: tree artifacts reject special files", current)
		}
		digest, err := hashRootFile(root, current)
		if err != nil {
			return err
		}
		decoded, err := hex.DecodeString(digest)
		if err != nil {
			return err
		}
		var sum [sha256.Size]byte
		copy(sum[:], decoded)
		entries = append(entries, treeEntry{path: relative, digest: sum})
		return nil
	})
	if err != nil {
		return "", err
	}
	return canonicalTreeDigest(entries)
}

func snapshotRootArtifact(root *os.Root, artifact RunnerArtifact, source, target string) (string, error) {
	switch artifact.Kind {
	case ArtifactFile:
		return snapshotRootFile(root, source, target)
	case ArtifactTree:
		return snapshotRootTree(root, source, target)
	default:
		return "", fmt.Errorf("unsupported artifact kind %q", artifact.Kind)
	}
}

func snapshotRootTree(root *os.Root, source, target string) (digest string, retErr error) {
	if err := ensureNoSymlinkPath(root, source, false); err != nil {
		return "", err
	}
	info, err := root.Lstat(source)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("staged tree output is not a real directory")
	}
	if err := root.Mkdir(target, 0o700); err != nil {
		return "", err
	}
	defer func() {
		if retErr != nil {
			_ = root.RemoveAll(target)
		}
	}()
	err = fs.WalkDir(root.FS(), source, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == source {
			return nil
		}
		relative := strings.TrimPrefix(current, source+"/")
		if err := validateTreePath(relative); err != nil {
			return err
		}
		destination := path.Join(target, relative)
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%q: tree artifacts reject symlinks", current)
		}
		if entryInfo.IsDir() {
			if err := root.Mkdir(destination, 0o700); err != nil {
				return err
			}
			return nil
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("%q: tree artifacts reject special files", current)
		}
		input, err := root.Open(current)
		if err != nil {
			return err
		}
		opened, err := input.Stat()
		if err != nil {
			_ = input.Close()
			return err
		}
		if !opened.Mode().IsRegular() || !os.SameFile(entryInfo, opened) {
			_ = input.Close()
			return fmt.Errorf("%q: tree entry changed identity while being opened", current)
		}
		output, err := root.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o400)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		syncErr := output.Sync()
		closeOutputErr := output.Close()
		closeInputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeOutputErr != nil {
			return closeOutputErr
		}
		return closeInputErr
	})
	if err != nil {
		return "", err
	}
	return hashRootTree(root, target)
}

func snapshotRootFile(root *os.Root, source, target string) (string, error) {
	if err := ensureNoSymlinkPath(root, source, false); err != nil {
		return "", err
	}
	info, err := root.Lstat(source)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("staged output is not a regular file")
	}
	input, err := root.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	opened, err := input.Stat()
	if err != nil {
		return "", err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return "", errors.New("staged output changed identity while being opened")
	}
	output, err := root.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o400)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if syncErr != nil {
		return "", syncErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func runnerChildEnvironment(root *os.Root, stage string, spec RunnerSpec, outputs map[string]string) ([]string, error) {
	home := path.Join(stage, "home")
	tmp := path.Join(stage, "tmp")
	if err := root.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	if err := root.MkdirAll(tmp, 0o700); err != nil {
		return nil, err
	}
	values := map[string]string{
		"HOME":               filepath.Join(root.Name(), filepath.FromSlash(home)),
		"PATH":               os.Getenv("PATH"),
		"TMPDIR":             filepath.Join(root.Name(), filepath.FromSlash(tmp)),
		"DHNT_PIPELINE":      spec.Pipeline,
		"DHNT_SOURCE_COMMIT": spec.Source.Commit,
		"DHNT_SOURCE_SHA256": spec.Source.SHA256,
		"DHNT_TASK":          spec.Task,
	}
	for _, item := range spec.Environment {
		values[item.Name] = item.Value
	}
	used := map[string]string{}
	for _, artifact := range spec.Inputs {
		base := "DHNT_INPUT_" + artifactEnvName(artifact.Name)
		if prior := used[base]; prior != "" {
			return nil, fmt.Errorf("artifact environment collision between %q and %q", prior, artifact.Name)
		}
		used[base] = artifact.Name
		values[base+"_PATH"] = filepath.Join(root.Name(), filepath.FromSlash(artifact.Path))
		values[base+"_SHA256"] = artifact.SHA256
	}
	for _, artifact := range spec.Outputs {
		base := "DHNT_OUTPUT_" + artifactEnvName(artifact.Name)
		if prior := used[base]; prior != "" {
			return nil, fmt.Errorf("artifact environment collision between %q and %q", prior, artifact.Name)
		}
		used[base] = artifact.Name
		values[base+"_PATH"] = filepath.Join(root.Name(), filepath.FromSlash(outputs[artifact.Name]))
		values[base+"_SHA256"] = artifact.SHA256
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	environment := make([]string, 0, len(names))
	for _, name := range names {
		environment = append(environment, name+"="+values[name])
	}
	return environment, nil
}

func verifyExistingDestination(root *os.Root, artifact RunnerArtifact) (bool, error) {
	exists, err := rootPathExists(root, artifact.Path)
	if err != nil || !exists {
		return false, err
	}
	digest, err := hashRootArtifact(root, artifact)
	if err != nil {
		return false, err
	}
	if digest != artifact.SHA256 {
		return false, fmt.Errorf("preexisting immutable destination %q has mismatched digest", artifact.Path)
	}
	return true, nil
}

func verifyExistingBytes(root *os.Root, name string, expected []byte) (bool, error) {
	exists, err := rootPathExists(root, name)
	if err != nil || !exists {
		return false, err
	}
	if err := ensureNoSymlinkPath(root, name, false); err != nil {
		return false, err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("preexisting immutable destination %q is not a regular file", name)
	}
	actual, err := root.ReadFile(name)
	if err != nil {
		return false, err
	}
	if !equalBytes(actual, expected) {
		return false, fmt.Errorf("preexisting immutable destination %q has mismatched bytes", name)
	}
	return true, nil
}

func rootPathExists(root *os.Root, name string) (bool, error) {
	_, err := root.Lstat(name)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func ensureParent(root *os.Root, name string) error {
	parent := path.Dir(name)
	if parent == "." {
		return nil
	}
	if err := ensureNoSymlinkPath(root, parent, true); err != nil {
		return err
	}
	if err := root.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	return ensureNoSymlinkPath(root, parent, false)
}

func writeSealedRootFile(root *os.Root, name string, data []byte) error {
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o400)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func syncRootParent(root *os.Root, name string) error {
	parent := path.Dir(name)
	file, err := root.Open(parent)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func publishRunEvidence(root *os.Root, evidenceDirectory, podUID string, run Run) error {
	data, err := MarshalRun(run)
	if err != nil {
		return err
	}
	if err := ensureEvidenceDirectory(root, evidenceDirectory); err != nil {
		return err
	}
	evidencePath := path.Join(evidenceDirectory, podUID+".json")
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	stage := path.Join(evidenceDirectory, "."+podUID+"."+hex.EncodeToString(random)+".tmp")
	if err := writeSealedRootFile(root, stage, data); err != nil {
		return err
	}
	defer root.Remove(stage)
	if exists, err := rootPathExists(root, evidencePath); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("evidence destination %q already exists", evidencePath)
	}
	if err := root.Link(stage, evidencePath); err != nil {
		return err
	}
	return syncRootParent(root, evidencePath)
}

func ensureEvidenceDirectory(root *os.Root, directory string) error {
	if err := ensureNoSymlinkPath(root, directory, true); err != nil {
		return err
	}
	if err := root.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	if err := ensureNoSymlinkPath(root, directory, false); err != nil {
		return err
	}
	info, err := root.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("evidence directory %q is not a directory", directory)
	}
	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var different byte
	for i := range a {
		different |= a[i] ^ b[i]
	}
	return different == 0
}
