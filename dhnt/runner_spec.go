package dhnt

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const RunnerSpecSchema = "dhnt.runner-spec/v1"

// RunnerArtifact binds one typed portable artifact to a workspace-relative
// filesystem path.
type RunnerArtifact struct {
	Name            string          `json:"name"`
	Kind            ArtifactKind    `json:"kind"`
	DigestAlgorithm DigestAlgorithm `json:"digestAlgorithm"`
	SHA256          string          `json:"sha256"`
	Path            string          `json:"path"`
}

// RunnerSpec is the canonical, non-secret execution contract embedded by the
// Argo v2 lowerer. Dynamic executor identity is supplied separately through the
// Kubernetes Downward API.
type RunnerSpec struct {
	Schema             string           `json:"schema"`
	Pipeline           string           `json:"pipeline"`
	Task               string           `json:"task"`
	Source             Source           `json:"source"`
	Argv               []string         `json:"argv"`
	WorkingDirectory   string           `json:"workingDirectory"`
	Environment        []Environment    `json:"environment"`
	Inputs             []RunnerArtifact `json:"inputs"`
	Outputs            []RunnerArtifact `json:"outputs"`
	EvidenceDirectory  string           `json:"evidenceDirectory"`
	CommitManifestPath string           `json:"commitManifestPath"`
	NonzeroClass       ResultClass      `json:"nonzeroClass"`
	Platform           Platform         `json:"platform"`
}

var secretEnvironmentName = regexp.MustCompile(`(?i)(secret|token|password|passwd|credential|private[_-]?key|api[_-]?key)`)

func DecodeRunnerSpec(data []byte) (RunnerSpec, error) {
	var spec RunnerSpec
	if err := decodeStrict(data, &spec); err != nil {
		return spec, err
	}
	if err := spec.Validate(); err != nil {
		return spec, err
	}
	canonical, err := MarshalRunnerSpec(spec)
	if err != nil {
		return spec, err
	}
	if !bytes.Equal(data, canonical) {
		return spec, errors.New("runner spec: input must use canonical JSON encoding")
	}
	return spec, nil
}

func MarshalRunnerSpec(spec RunnerSpec) ([]byte, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	spec.Argv = append([]string(nil), spec.Argv...)
	spec.Environment = append([]Environment(nil), spec.Environment...)
	sort.Slice(spec.Environment, func(i, j int) bool { return spec.Environment[i].Name < spec.Environment[j].Name })
	spec.Inputs = sortedRunnerArtifacts(spec.Inputs)
	spec.Outputs = sortedRunnerArtifacts(spec.Outputs)
	return marshalLine(spec)
}

func (spec RunnerSpec) Validate() error {
	if spec.Schema != RunnerSpecSchema {
		return fmt.Errorf("schema: got %q, want %q", spec.Schema, RunnerSpecSchema)
	}
	if err := validateID("pipeline", spec.Pipeline); err != nil {
		return err
	}
	if err := validateSource(spec.Source); err != nil {
		return err
	}
	task := Task{
		ID:               spec.Task,
		Lane:             LaneCluster,
		Distribution:     DistributionSingle,
		Argv:             spec.Argv,
		WorkingDirectory: spec.WorkingDirectory,
		Environment:      spec.Environment,
	}
	if err := validateTask(task); err != nil {
		return fmt.Errorf("task: %w", err)
	}
	if err := validateNonSecretEnvironment(spec.Environment); err != nil {
		return err
	}
	if spec.Platform.Backend != "k3s" || spec.Platform.OS != "linux" {
		return fmt.Errorf("platform: trusted Argo runner requires k3s/linux, got %s/%s", spec.Platform.Backend, spec.Platform.OS)
	}
	if err := validatePlatform(spec.Platform); err != nil {
		return err
	}
	switch spec.NonzeroClass {
	case ResultTestFail, ResultInfraFail:
	default:
		return fmt.Errorf("nonzeroClass: must be %q or %q", ResultTestFail, ResultInfraFail)
	}
	if !cleanRelativePath(spec.EvidenceDirectory) {
		return fmt.Errorf("evidenceDirectory: must be a clean workspace-relative path")
	}
	if !cleanRelativePath(spec.CommitManifestPath) {
		return fmt.Errorf("commitManifestPath: must be a clean workspace-relative path")
	}
	if reservedRunnerPath(spec.EvidenceDirectory) || reservedRunnerPath(spec.CommitManifestPath) {
		return fmt.Errorf("runner paths must not use reserved .dhnt-staging")
	}
	if spec.EvidenceDirectory == spec.CommitManifestPath {
		return fmt.Errorf("path alias: evidence directory and commit manifest both use %q", spec.EvidenceDirectory)
	}
	allPaths := map[string]string{
		spec.EvidenceDirectory:  "evidence directory",
		spec.CommitManifestPath: "commit manifest",
	}
	validate := func(field string, artifacts []RunnerArtifact, output bool) error {
		if len(artifacts) == 0 {
			return fmt.Errorf("%s: must not be empty", field)
		}
		portable := make([]Artifact, 0, len(artifacts))
		names := map[string]bool{}
		for i, artifact := range artifacts {
			if names[artifact.Name] {
				return fmt.Errorf("%s[%d].name: duplicate %q", field, i, artifact.Name)
			}
			names[artifact.Name] = true
			if artifact.Kind != ArtifactFile || artifact.DigestAlgorithm != DigestSHA256FileV1 {
				return fmt.Errorf("%s[%d]: runner v1 supports only file/%s; tree publication remains fail-closed",
					field, i, DigestSHA256FileV1)
			}
			if !cleanRelativePath(artifact.Path) || reservedRunnerPath(artifact.Path) {
				return fmt.Errorf("%s[%d].path: must be a clean non-reserved workspace-relative path", field, i)
			}
			if output && path.Base(artifact.Path) != artifact.SHA256 {
				return fmt.Errorf("%s[%d].path: output final basename must equal its sha256 for content-addressed publication", field, i)
			}
			portable = append(portable, artifact.Artifact())
			if prior, exists := allPaths[artifact.Path]; exists {
				return fmt.Errorf("%s[%d].path: aliases %s", field, i, prior)
			}
			allPaths[artifact.Path] = field + " " + artifact.Name
		}
		return validateArtifacts(field, portable, true, RunSchemaV2)
	}
	if err := validate("inputs", spec.Inputs, false); err != nil {
		return err
	}
	if err := validate("outputs", spec.Outputs, true); err != nil {
		return err
	}
	if err := rejectPathAliases(allPaths); err != nil {
		return err
	}
	return nil
}

func validateNonSecretEnvironment(environment []Environment) error {
	for i, item := range environment {
		if strings.HasPrefix(item.Name, "DHNT_") {
			return fmt.Errorf("environment[%d].name: %q is reserved by the trusted runner", i, item.Name)
		}
		if secretEnvironmentName.MatchString(item.Name) {
			return fmt.Errorf("environment[%d].name: %q looks secret-bearing; pipeline environment is literal and non-secret only", i, item.Name)
		}
	}
	return nil
}

func (artifact RunnerArtifact) Artifact() Artifact {
	return Artifact{
		Name:            artifact.Name,
		Kind:            artifact.Kind,
		DigestAlgorithm: artifact.DigestAlgorithm,
		SHA256:          artifact.SHA256,
	}
}

func sortedRunnerArtifacts(in []RunnerArtifact) []RunnerArtifact {
	out := append([]RunnerArtifact(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func rejectPathAliases(paths map[string]string) error {
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		for _, other := range names[i+1:] {
			if pathWithin(name, other) || pathWithin(other, name) {
				return fmt.Errorf("path alias: %s %q and %s %q are equal or ancestor-related",
					paths[name], name, paths[other], other)
			}
		}
	}
	return nil
}

func pathWithin(parent, child string) bool {
	return parent == child || (parent != "." && strings.HasPrefix(child, parent+"/"))
}

func reservedRunnerPath(value string) bool {
	return value == ".dhnt-staging" || strings.HasPrefix(value, ".dhnt-staging/")
}

func runnerSpecEqualArgv(spec RunnerSpec, argv []string) bool {
	return slices.Equal(spec.Argv, argv)
}
