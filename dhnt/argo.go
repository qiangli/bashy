package dhnt

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const ArgoBindingSchema = "dhnt.argo-binding/v1"

type ArgoWorkspace struct {
	ClaimName string `json:"claimName"`
	MountPath string `json:"mountPath"`
}

type ArgoArtifactBinding struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type ArgoTaskBinding struct {
	ID             string                `json:"id"`
	Image          string                `json:"image"`
	Artifacts      []ArgoArtifactBinding `json:"artifacts"`
	TimeoutSeconds *int                  `json:"timeoutSeconds,omitempty"`
	RetryLimit     *int                  `json:"retryLimit,omitempty"`
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

func (binding ArgoBinding) Validate() error {
	if binding.Schema != ArgoBindingSchema {
		return fmt.Errorf("schema: got %q, want %q", binding.Schema, ArgoBindingSchema)
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
	Entrypoint string         `yaml:"entrypoint"`
	Templates  []argoTemplate `yaml:"templates"`
	Volumes    []argoVolume   `yaml:"volumes"`
}
type argoTemplate struct {
	Name                  string                `yaml:"name"`
	Metadata              *argoTemplateMetadata `yaml:"metadata,omitempty"`
	DAG                   argoDAG               `yaml:"dag,omitempty"`
	NodeSelector          map[string]string     `yaml:"nodeSelector,omitempty"`
	ActiveDeadlineSeconds *int                  `yaml:"activeDeadlineSeconds,omitempty"`
	RetryStrategy         *argoRetryStrategy    `yaml:"retryStrategy,omitempty"`
	Container             *argoContainer        `yaml:"container,omitempty"`
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
type argoRetryStrategy struct {
	Limit string `yaml:"limit"`
}
type argoContainer struct {
	Image        string            `yaml:"image"`
	Command      []string          `yaml:"command"`
	Args         []string          `yaml:"args"`
	WorkingDir   string            `yaml:"workingDir"`
	Env          []argoEnv         `yaml:"env"`
	VolumeMounts []argoVolumeMount `yaml:"volumeMounts"`
}
type argoEnv struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
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
