package dhnt

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ArgoBindingSchema   = "dhnt.argo-binding/v1"
	ArgoBindingSchemaV2 = "dhnt.argo-binding/v2"
)

type ArgoWorkspace struct {
	ClaimName string `json:"claimName"`
	MountPath string `json:"mountPath"`
}

type ArgoArtifactBinding struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type ArgoTaskBinding struct {
	ID                 string                `json:"id"`
	Image              string                `json:"image"`
	RunnerPath         string                `json:"runnerPath,omitempty"`
	Artifacts          []ArgoArtifactBinding `json:"artifacts"`
	EvidenceDirectory  string                `json:"evidenceDirectory,omitempty"`
	CommitManifestPath string                `json:"commitManifestPath,omitempty"`
	NonzeroClass       ResultClass           `json:"nonzeroClass,omitempty"`
	TimeoutSeconds     *int                  `json:"timeoutSeconds,omitempty"`
	RetryLimit         *int                  `json:"retryLimit,omitempty"`
}

// ArgoBinding supplies execution facts that deliberately do not belong in the
// portable dhnt.pipeline/v1 evidence contract.
type ArgoBinding struct {
	Schema    string            `json:"schema"`
	Workspace ArgoWorkspace     `json:"workspace"`
	Tasks     []ArgoTaskBinding `json:"tasks"`
}

func DecodeArgoBinding(data []byte) (ArgoBinding, error) {
	var binding ArgoBinding
	if err := decodeStrict(data, &binding); err != nil {
		return binding, err
	}
	if err := binding.Validate(); err != nil {
		return binding, err
	}
	return binding, nil
}

var (
	kubeNameRE  = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$`)
	imageDigest = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
)

// LowerArgo emits one deterministic Argo Workflow. It only lowers DKS tasks
// whose execution semantics are fully represented by pipeline plus binding.
func LowerArgo(p Pipeline, binding ArgoBinding) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("pipeline: %w", err)
	}
	if err := binding.Validate(); err != nil {
		return nil, fmt.Errorf("binding: %w", err)
	}
	if p.Schema == PipelineSchemaV2 {
		if binding.Schema != ArgoBindingSchemaV2 {
			return nil, fmt.Errorf("pipeline schema %q requires binding schema %q", p.Schema, ArgoBindingSchemaV2)
		}
		return lowerArgoV2(p, binding)
	}
	if binding.Schema != ArgoBindingSchema {
		return nil, fmt.Errorf("pipeline schema %q requires binding schema %q", p.Schema, ArgoBindingSchema)
	}
	if !kubeNameRE.MatchString(p.Pipeline) || len(p.Pipeline) > 52 {
		return nil, fmt.Errorf("pipeline %q is not a Kubernetes-safe name (lowercase DNS name, at most 52 characters)", p.Pipeline)
	}

	matrix := make(map[string]MatrixEntry, len(p.Tasks))
	for _, entry := range p.Matrix {
		if _, exists := matrix[entry.Task]; exists {
			return nil, fmt.Errorf("task %q has multiple matrix rows; Argo lowering requires one already-expanded task per row", entry.Task)
		}
		matrix[entry.Task] = entry
	}
	bindings := make(map[string]ArgoTaskBinding, len(binding.Tasks))
	for _, task := range binding.Tasks {
		bindings[task.ID] = task
	}
	if len(bindings) != len(p.Tasks) {
		return nil, fmt.Errorf("binding task coverage mismatch: got %d, want %d", len(bindings), len(p.Tasks))
	}

	workflow := argoWorkflow{
		APIVersion: "argoproj.io/v1alpha1",
		Kind:       "Workflow",
		Metadata: argoMetadata{
			GenerateName: p.Pipeline + "-",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "bashy-dhnt",
				"dhnt.io/pipeline":             p.Pipeline,
			},
			Annotations: map[string]string{
				"dhnt.io/source-commit": p.Source.Commit,
				"dhnt.io/source-sha256": p.Source.SHA256,
			},
		},
		Spec: argoSpec{
			Entrypoint: "pipeline",
			Volumes: []argoVolume{{
				Name:                  "workspace",
				PersistentVolumeClaim: argoPVC{ClaimName: binding.Workspace.ClaimName},
			}},
		},
	}

	tasks := append([]Task(nil), p.Tasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	dag := argoTemplate{Name: "pipeline"}
	for _, task := range tasks {
		if !kubeNameRE.MatchString(task.ID) || len(task.ID) > 63 {
			return nil, fmt.Errorf("task %q is not a Kubernetes-safe lowercase DNS name", task.ID)
		}
		if task.Lane != LaneCluster && task.Lane != LaneContainer {
			return nil, fmt.Errorf("task %q: lane %q cannot run on DKS Argo without changing its declared semantics", task.ID, task.Lane)
		}
		if task.Distribution != DistributionSingle && task.Distribution != DistributionShardable {
			return nil, fmt.Errorf("task %q: distribution %q is unsupported (no fake replication, DDP, or gang scheduling)", task.ID, task.Distribution)
		}
		entry := matrix[task.ID]
		if entry.Platform.Backend != "k3s" || entry.Platform.OS != "linux" {
			return nil, fmt.Errorf("task %q: DKS Argo requires platform backend k3s and OS linux, got %s/%s", task.ID, entry.Platform.Backend, entry.Platform.OS)
		}
		if task.Distribution == DistributionShardable && entry.Chunk == nil {
			return nil, fmt.Errorf("task %q: shardable task is not pre-expanded with an immutable chunk", task.ID)
		}
		taskBinding, ok := bindings[task.ID]
		if !ok {
			return nil, fmt.Errorf("task %q: missing binding", task.ID)
		}
		env, err := argoEnvironment(p, task, entry, taskBinding, binding.Workspace.MountPath)
		if err != nil {
			return nil, err
		}
		dag.DAG.Tasks = append(dag.DAG.Tasks, argoDAGTask{
			Name: task.ID, Template: task.ID, Dependencies: sortedStrings(task.Needs),
		})
		template := argoTemplate{
			Name: task.ID,
			Metadata: &argoTemplateMetadata{Annotations: map[string]string{
				"dhnt.io/distribution": string(task.Distribution),
			}},
			NodeSelector: map[string]string{
				"kubernetes.io/arch":      entry.Platform.Arch,
				"kubernetes.io/os":        "linux",
				"outpost.dhnt.io/backend": "k3s",
			},
			Container: &argoContainer{
				Image:      taskBinding.Image,
				Command:    []string{task.Argv[0]},
				Args:       append([]string(nil), task.Argv[1:]...),
				WorkingDir: path.Join(binding.Workspace.MountPath, task.WorkingDirectory),
				Env:        env,
				VolumeMounts: []argoVolumeMount{{
					Name: "workspace", MountPath: binding.Workspace.MountPath,
				}},
			},
		}
		if taskBinding.TimeoutSeconds != nil {
			template.ActiveDeadlineSeconds = taskBinding.TimeoutSeconds
		}
		if taskBinding.RetryLimit != nil {
			template.RetryStrategy = &argoRetryStrategy{Limit: strconv.Itoa(*taskBinding.RetryLimit)}
		}
		workflow.Spec.Templates = append(workflow.Spec.Templates, template)
	}
	workflow.Spec.Templates = append([]argoTemplate{dag}, workflow.Spec.Templates...)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(workflow); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func lowerArgoV2(p Pipeline, binding ArgoBinding) ([]byte, error) {
	if !kubeNameRE.MatchString(p.Pipeline) || len(p.Pipeline) > 52 {
		return nil, fmt.Errorf("pipeline %q is not a Kubernetes-safe name (lowercase DNS name, at most 52 characters)", p.Pipeline)
	}
	matrix := make(map[string]MatrixEntry, len(p.Tasks))
	for _, entry := range p.Matrix {
		if _, exists := matrix[entry.Task]; exists {
			return nil, fmt.Errorf("task %q has multiple matrix rows; Argo lowering requires one already-expanded task per row", entry.Task)
		}
		matrix[entry.Task] = entry
	}
	bindings := make(map[string]ArgoTaskBinding, len(binding.Tasks))
	for _, task := range binding.Tasks {
		bindings[task.ID] = task
	}
	if len(bindings) != len(p.Tasks) {
		return nil, fmt.Errorf("binding task coverage mismatch: got %d, want %d", len(bindings), len(p.Tasks))
	}
	workflow := argoWorkflow{
		APIVersion: "argoproj.io/v1alpha1",
		Kind:       "Workflow",
		Metadata: argoMetadata{
			GenerateName: p.Pipeline + "-",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "bashy-dhnt",
				"dhnt.io/pipeline":             p.Pipeline,
			},
			Annotations: map[string]string{
				"dhnt.io/source-commit": p.Source.Commit,
				"dhnt.io/source-sha256": p.Source.SHA256,
				"dhnt.io/evidence":      RunSchemaV2,
			},
		},
		Spec: argoSpec{
			Entrypoint: "pipeline",
			Volumes: []argoVolume{{
				Name:                  "workspace",
				PersistentVolumeClaim: argoPVC{ClaimName: binding.Workspace.ClaimName},
			}},
		},
	}
	claimedRuntimePaths := map[string]string{}
	workflowPaths := map[string]string{}
	workflowArtifacts := map[string]Artifact{}
	tasks := append([]Task(nil), p.Tasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	dag := argoTemplate{Name: "pipeline"}
	for _, task := range tasks {
		if !kubeNameRE.MatchString(task.ID) || len(task.ID) > 63 {
			return nil, fmt.Errorf("task %q is not a Kubernetes-safe lowercase DNS name", task.ID)
		}
		if task.Lane != LaneCluster && task.Lane != LaneContainer {
			return nil, fmt.Errorf("task %q: lane %q cannot run on DKS Argo without changing its declared semantics", task.ID, task.Lane)
		}
		if task.Distribution != DistributionSingle && task.Distribution != DistributionShardable {
			return nil, fmt.Errorf("task %q: distribution %q is unsupported (no fake replication, DDP, or gang scheduling)", task.ID, task.Distribution)
		}
		entry := matrix[task.ID]
		if entry.Platform.Backend != "k3s" || entry.Platform.OS != "linux" {
			return nil, fmt.Errorf("task %q: DKS Argo requires platform backend k3s and OS linux, got %s/%s",
				task.ID, entry.Platform.Backend, entry.Platform.OS)
		}
		taskBinding, ok := bindings[task.ID]
		if !ok {
			return nil, fmt.Errorf("task %q: missing binding", task.ID)
		}
		spec, err := runnerSpecForTask(p, task, entry, taskBinding)
		if err != nil {
			return nil, fmt.Errorf("task %q: %w", task.ID, err)
		}
		for _, artifact := range append(append([]RunnerArtifact(nil), spec.Inputs...), spec.Outputs...) {
			if prior, exists := workflowPaths[artifact.Path]; !exists {
				workflowPaths[artifact.Path] = "task " + task.ID + " artifact " + artifact.Name
				workflowArtifacts[artifact.Path] = artifact.Artifact()
			} else if !strings.Contains(prior, " artifact ") {
				return nil, fmt.Errorf("task %q: artifact path %q aliases %s", task.ID, artifact.Path, prior)
			} else if workflowArtifacts[artifact.Path] != artifact.Artifact() {
				return nil, fmt.Errorf("task %q: artifact path %q has conflicting immutable identities", task.ID, artifact.Path)
			}
		}
		for _, item := range []struct {
			path string
			kind string
		}{
			{taskBinding.EvidenceDirectory, "evidence directory"},
			{taskBinding.CommitManifestPath, "commit manifest"},
		} {
			if prior, exists := claimedRuntimePaths[item.path]; exists {
				return nil, fmt.Errorf("task %q: %s path %q is already used by %s", task.ID, item.kind, item.path, prior)
			}
			if prior, exists := workflowPaths[item.path]; exists {
				return nil, fmt.Errorf("task %q: %s path %q aliases %s", task.ID, item.kind, item.path, prior)
			}
			claimedRuntimePaths[item.path] = "task " + task.ID + " " + item.kind
			workflowPaths[item.path] = "task " + task.ID + " " + item.kind
		}
		specJSON, err := MarshalRunnerSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("task %q: runner spec: %w", task.ID, err)
		}
		dag.DAG.Tasks = append(dag.DAG.Tasks, argoDAGTask{
			Name: task.ID, Template: task.ID, Dependencies: sortedStrings(task.Needs),
		})
		template := argoTemplate{
			Name: task.ID,
			Metadata: &argoTemplateMetadata{Annotations: map[string]string{
				"dhnt.io/distribution":    string(task.Distribution),
				"dhnt.io/runner-contract": RunnerSpecSchema,
			}},
			NodeSelector: map[string]string{
				"kubernetes.io/arch":      entry.Platform.Arch,
				"kubernetes.io/os":        "linux",
				"outpost.dhnt.io/backend": "k3s",
			},
			Container: &argoContainer{
				Image:   taskBinding.Image,
				Command: []string{taskBinding.RunnerPath},
				Args: append([]string{
					"dhnt", "run-task",
					"--workspace", binding.Workspace.MountPath,
					"--spec-base64", base64.StdEncoding.EncodeToString(specJSON),
					"--",
				}, task.Argv...),
				WorkingDir: path.Join(binding.Workspace.MountPath, task.WorkingDirectory),
				Env: []argoEnv{
					{Name: "DHNT_EXECUTOR_NODE", ValueFrom: &argoEnvSource{
						FieldRef: &argoFieldRef{FieldPath: "spec.nodeName"},
					}},
					{Name: "DHNT_POD_UID", ValueFrom: &argoEnvSource{
						FieldRef: &argoFieldRef{FieldPath: "metadata.uid"},
					}},
				},
				VolumeMounts: []argoVolumeMount{{
					Name: "workspace", MountPath: binding.Workspace.MountPath,
				}},
			},
		}
		if taskBinding.TimeoutSeconds != nil {
			template.ActiveDeadlineSeconds = taskBinding.TimeoutSeconds
		}
		if taskBinding.RetryLimit != nil {
			template.RetryStrategy = &argoRetryStrategy{Limit: strconv.Itoa(*taskBinding.RetryLimit)}
		}
		workflow.Spec.Templates = append(workflow.Spec.Templates, template)
	}
	if err := rejectPathAliases(workflowPaths); err != nil {
		return nil, fmt.Errorf("workflow paths: %w", err)
	}
	workflow.Spec.Templates = append([]argoTemplate{dag}, workflow.Spec.Templates...)
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(workflow); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func runnerSpecForTask(p Pipeline, task Task, entry MatrixEntry, binding ArgoTaskBinding) (RunnerSpec, error) {
	paths := make(map[string]string, len(binding.Artifacts))
	for _, artifact := range binding.Artifacts {
		paths[artifact.Name] = artifact.Path
	}
	expected := map[string]bool{}
	convert := func(direction string, artifacts []Artifact) ([]RunnerArtifact, error) {
		result := make([]RunnerArtifact, 0, len(artifacts))
		for _, artifact := range artifacts {
			artifactPath, ok := paths[artifact.Name]
			if !ok {
				return nil, fmt.Errorf("binding lacks %s artifact %q", direction, artifact.Name)
			}
			if expected[artifact.Name] {
				return nil, fmt.Errorf("artifact %q is declared as both input and output; immutable runner paths cannot be in-place", artifact.Name)
			}
			expected[artifact.Name] = true
			result = append(result, RunnerArtifact{
				Name:            artifact.Name,
				Kind:            artifact.Kind,
				DigestAlgorithm: artifact.DigestAlgorithm,
				SHA256:          artifact.SHA256,
				Path:            artifactPath,
			})
		}
		return result, nil
	}
	inputs, err := convert("input", entry.Inputs)
	if err != nil {
		return RunnerSpec{}, err
	}
	outputs, err := convert("output", entry.Outputs)
	if err != nil {
		return RunnerSpec{}, err
	}
	for name := range paths {
		if !expected[name] {
			return RunnerSpec{}, fmt.Errorf("binding has undeclared artifact %q", name)
		}
	}
	if len(paths) != len(expected) {
		return RunnerSpec{}, fmt.Errorf("artifact binding coverage mismatch")
	}
	return RunnerSpec{
		Schema:             RunnerSpecSchema,
		Pipeline:           p.Pipeline,
		Task:               task.ID,
		Source:             p.Source,
		Argv:               append([]string{}, task.Argv...),
		WorkingDirectory:   task.WorkingDirectory,
		Environment:        append([]Environment{}, task.Environment...),
		Inputs:             inputs,
		Outputs:            outputs,
		EvidenceDirectory:  binding.EvidenceDirectory,
		CommitManifestPath: binding.CommitManifestPath,
		NonzeroClass:       binding.NonzeroClass,
		Platform:           entry.Platform,
	}, nil
}

func (binding ArgoBinding) Validate() error {
	switch binding.Schema {
	case ArgoBindingSchema, ArgoBindingSchemaV2:
	default:
		return fmt.Errorf("schema: got %q, want %q or %q", binding.Schema, ArgoBindingSchema, ArgoBindingSchemaV2)
	}
	if !kubeNameRE.MatchString(binding.Workspace.ClaimName) || len(binding.Workspace.ClaimName) > 253 {
		return errors.New("workspace.claimName: must be a Kubernetes-safe DNS name")
	}
	if !cleanAbsolutePath(binding.Workspace.MountPath) {
		return errors.New("workspace.mountPath: must be a clean absolute path")
	}
	if len(binding.Tasks) == 0 {
		return errors.New("tasks: must not be empty")
	}
	seen := map[string]bool{}
	for i, task := range binding.Tasks {
		if seen[task.ID] {
			return fmt.Errorf("tasks[%d].id: duplicate %q", i, task.ID)
		}
		seen[task.ID] = true
		imageName, _, hasDigest := strings.Cut(task.Image, "@sha256:")
		if !hasDigest || strings.Count(task.Image, "@sha256:") != 1 || imageName == "" ||
			strings.ContainsAny(imageName, " \t\r\n") || !imageDigest.MatchString(task.Image) {
			return fmt.Errorf("tasks[%d].image: must be pinned by lowercase sha256 digest", i)
		}
		if task.TimeoutSeconds != nil && *task.TimeoutSeconds < 1 {
			return fmt.Errorf("tasks[%d].timeoutSeconds: must be positive", i)
		}
		if task.RetryLimit != nil && (*task.RetryLimit < 0 || *task.RetryLimit > 10) {
			return fmt.Errorf("tasks[%d].retryLimit: must be between 0 and 10", i)
		}
		if binding.Schema == ArgoBindingSchema {
			if task.RunnerPath != "" || task.EvidenceDirectory != "" || task.CommitManifestPath != "" || task.NonzeroClass != "" {
				return fmt.Errorf("tasks[%d]: runnerPath, evidenceDirectory, commitManifestPath, and nonzeroClass must be absent from v1", i)
			}
		} else {
			if task.TimeoutSeconds == nil {
				return fmt.Errorf("tasks[%d].timeoutSeconds: must be explicitly bounded in v2", i)
			}
			if task.RetryLimit == nil {
				return fmt.Errorf("tasks[%d].retryLimit: must be explicitly declared in v2", i)
			}
			if !cleanAbsolutePath(task.RunnerPath) {
				return fmt.Errorf("tasks[%d].runnerPath: must be a clean absolute path", i)
			}
			if !cleanRelativePath(task.EvidenceDirectory) {
				return fmt.Errorf("tasks[%d].evidenceDirectory: must be a clean workspace-relative path", i)
			}
			if !cleanRelativePath(task.CommitManifestPath) {
				return fmt.Errorf("tasks[%d].commitManifestPath: must be a clean workspace-relative path", i)
			}
			switch task.NonzeroClass {
			case ResultTestFail, ResultInfraFail:
			default:
				return fmt.Errorf("tasks[%d].nonzeroClass: must be %q or %q", i, ResultTestFail, ResultInfraFail)
			}
		}
		artifactSeen := map[string]bool{}
		pathSeen := map[string]string{}
		for j, artifact := range task.Artifacts {
			if artifactSeen[artifact.Name] {
				return fmt.Errorf("tasks[%d].artifacts[%d].name: duplicate %q", i, j, artifact.Name)
			}
			artifactSeen[artifact.Name] = true
			if !cleanRelativePath(artifact.Path) {
				return fmt.Errorf("tasks[%d].artifacts[%d].path: must be a clean workspace-relative path", i, j)
			}
			if prior, exists := pathSeen[artifact.Path]; exists {
				return fmt.Errorf("tasks[%d].artifacts[%d].path: shared by artifacts %q and %q", i, j, prior, artifact.Name)
			}
			pathSeen[artifact.Path] = artifact.Name
		}
		if binding.Schema == ArgoBindingSchemaV2 {
			allPaths := make(map[string]string, len(pathSeen)+2)
			for artifactPath, name := range pathSeen {
				allPaths[artifactPath] = "artifact " + name
			}
			if prior, exists := allPaths[task.EvidenceDirectory]; exists {
				return fmt.Errorf("tasks[%d].evidenceDirectory: aliases %s", i, prior)
			}
			allPaths[task.EvidenceDirectory] = "evidence directory"
			if prior, exists := allPaths[task.CommitManifestPath]; exists {
				return fmt.Errorf("tasks[%d].commitManifestPath: aliases %s", i, prior)
			}
			allPaths[task.CommitManifestPath] = "commit manifest"
			if err := rejectPathAliases(allPaths); err != nil {
				return fmt.Errorf("tasks[%d]: %w", i, err)
			}
		}
	}
	return nil
}

func argoEnvironment(p Pipeline, task Task, entry MatrixEntry, binding ArgoTaskBinding, workspaceMount string) ([]argoEnv, error) {
	paths := make(map[string]string, len(binding.Artifacts))
	for _, artifact := range binding.Artifacts {
		paths[artifact.Name] = artifact.Path
	}
	expected := make(map[string]bool, len(entry.Inputs)+len(entry.Outputs))
	envNames := map[string]string{}
	var env []argoEnv
	for _, item := range task.Environment {
		if strings.HasPrefix(item.Name, "DHNT_") {
			return nil, fmt.Errorf("task %q: environment name %q is reserved by the Argo runtime contract", task.ID, item.Name)
		}
		env = append(env, argoEnv{Name: item.Name, Value: item.Value})
	}
	env = append(env,
		argoEnv{Name: "DHNT_PIPELINE", Value: p.Pipeline},
		argoEnv{Name: "DHNT_SOURCE_COMMIT", Value: p.Source.Commit},
		argoEnv{Name: "DHNT_SOURCE_SHA256", Value: p.Source.SHA256},
		argoEnv{Name: "DHNT_TASK", Value: task.ID},
	)
	addArtifacts := func(direction string, artifacts []Artifact) error {
		for _, artifact := range artifacts {
			artifactPath, ok := paths[artifact.Name]
			if !ok {
				return fmt.Errorf("task %q: binding lacks %s artifact %q", task.ID, strings.ToLower(direction), artifact.Name)
			}
			expected[artifact.Name] = true
			base := "DHNT_" + direction + "_" + artifactEnvName(artifact.Name)
			if prior, collision := envNames[base]; collision && prior != artifact.Name {
				return fmt.Errorf("task %q: artifact names %q and %q collide as environment variables", task.ID, prior, artifact.Name)
			}
			envNames[base] = artifact.Name
			env = append(env,
				argoEnv{Name: base + "_PATH", Value: path.Join(workspaceMount, artifactPath)},
				argoEnv{Name: base + "_SHA256", Value: artifact.SHA256},
			)
		}
		return nil
	}
	if err := addArtifacts("INPUT", entry.Inputs); err != nil {
		return nil, err
	}
	if err := addArtifacts("OUTPUT", entry.Outputs); err != nil {
		return nil, err
	}
	for name := range paths {
		if !expected[name] {
			return nil, fmt.Errorf("task %q: binding has undeclared artifact %q", task.ID, name)
		}
	}
	if len(paths) != len(expected) {
		return nil, fmt.Errorf("task %q: artifact binding coverage mismatch", task.ID)
	}
	sort.Slice(env, func(i, j int) bool { return env[i].Name < env[j].Name })
	return env, nil
}

func artifactEnvName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func cleanAbsolutePath(value string) bool {
	return strings.HasPrefix(value, "/") && !strings.ContainsRune(value, 0) && path.Clean(value) == value && value != "/"
}

func cleanRelativePath(value string) bool {
	return value != "" && value != "." && !strings.HasPrefix(value, "/") && !strings.ContainsRune(value, 0) &&
		!strings.Contains(value, `\`) && path.Clean(value) == value &&
		value != ".." && !strings.HasPrefix(value, "../")
}

type argoWorkflow struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   argoMetadata `yaml:"metadata"`
	Spec       argoSpec     `yaml:"spec"`
}
type argoMetadata struct {
	GenerateName string            `yaml:"generateName"`
	Labels       map[string]string `yaml:"labels"`
	Annotations  map[string]string `yaml:"annotations"`
}
type argoSpec struct {
	Entrypoint         string         `yaml:"entrypoint"`
	ServiceAccountName string         `yaml:"serviceAccountName,omitempty"`
	Templates          []argoTemplate `yaml:"templates"`
	Volumes            []argoVolume   `yaml:"volumes"`
}
type argoTemplate struct {
	Name                  string                `yaml:"name"`
	Metadata              *argoTemplateMetadata `yaml:"metadata,omitempty"`
	DAG                   argoDAG               `yaml:"dag,omitempty"`
	NodeSelector          map[string]string     `yaml:"nodeSelector,omitempty"`
	ActiveDeadlineSeconds *int                  `yaml:"activeDeadlineSeconds,omitempty"`
	RetryStrategy         *argoRetryStrategy    `yaml:"retryStrategy,omitempty"`
	Container             *argoContainer        `yaml:"container,omitempty"`
	Steps                 [][]argoStep          `yaml:"steps,omitempty"`
	Resource              *argoResource         `yaml:"resource,omitempty"`
}
type argoTemplateMetadata struct {
	Annotations map[string]string `yaml:"annotations"`
}
type argoDAG struct {
	Tasks []argoDAGTask `yaml:"tasks,omitempty"`
}
type argoDAGTask struct {
	Name         string   `yaml:"name"`
	Template     string   `yaml:"template"`
	Dependencies []string `yaml:"dependencies"`
}
type argoStep struct {
	Name     string `yaml:"name"`
	Template string `yaml:"template"`
}
type argoResource struct {
	Action            string `yaml:"action"`
	SetOwnerReference bool   `yaml:"setOwnerReference"`
	SuccessCondition  string `yaml:"successCondition"`
	FailureCondition  string `yaml:"failureCondition"`
	Manifest          string `yaml:"manifest"`
}
type argoRetryStrategy struct {
	Limit string `yaml:"limit"`
}
type argoContainer struct {
	Image        string                    `yaml:"image"`
	Command      []string                  `yaml:"command"`
	Args         []string                  `yaml:"args"`
	WorkingDir   string                    `yaml:"workingDir"`
	Env          []argoEnv                 `yaml:"env"`
	VolumeMounts []argoVolumeMount         `yaml:"volumeMounts"`
	Resources    *argoResourceRequirements `yaml:"resources,omitempty"`
}
type argoResourceRequirements struct {
	Requests map[string]string `yaml:"requests"`
}
type argoEnv struct {
	Name      string         `yaml:"name"`
	Value     string         `yaml:"value"`
	ValueFrom *argoEnvSource `yaml:"valueFrom,omitempty"`
}

func (environment argoEnv) MarshalYAML() (any, error) {
	if environment.ValueFrom != nil {
		return struct {
			Name      string         `yaml:"name"`
			ValueFrom *argoEnvSource `yaml:"valueFrom"`
		}{Name: environment.Name, ValueFrom: environment.ValueFrom}, nil
	}
	return struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	}{Name: environment.Name, Value: environment.Value}, nil
}

type argoEnvSource struct {
	FieldRef *argoFieldRef `yaml:"fieldRef,omitempty"`
}
type argoFieldRef struct {
	FieldPath string `yaml:"fieldPath"`
}
type argoVolumeMount struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
}
type argoVolume struct {
	Name                  string  `yaml:"name"`
	PersistentVolumeClaim argoPVC `yaml:"persistentVolumeClaim"`
}
type argoPVC struct {
	ClaimName string `yaml:"claimName"`
}
