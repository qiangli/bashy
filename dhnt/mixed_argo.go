package dhnt

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	dag "github.com/qiangli/coreutils/pkg/dag"
	"gopkg.in/yaml.v3"
)

const (
	MixedArgoBindingSchema  = "dhnt.mixed-argo-binding/v1"
	DKSWorkerResolverSchema = "dhnt.dks-worker-resolver/v1"
)

// MixedArgoBinding supplies the execution-only details needed to lower native
// tasks. It is paired with, but deliberately separate from, the runtime worker
// resolver: portable plans never contain Kubernetes node or Outpost identity.
type MixedArgoBinding struct {
	Schema               string              `json:"schema"`
	Execution            ArgoBinding         `json:"execution"`
	ServiceAccountName   string              `json:"serviceAccountName"`
	ResultValidatorImage string              `json:"resultValidatorImage"`
	Native               []NativeTaskBinding `json:"native"`
}

type NativeTaskBinding struct {
	Task                  string               `json:"task"`
	ExecutableURL         string               `json:"executableUrl"`
	ExecutableSHA256      string               `json:"executableSha256"`
	ExecutablePath        string               `json:"executablePath"`
	Inputs                []NativeInputBinding `json:"inputs"`
	TaintKey              string               `json:"taintKey"`
	ActiveDeadlineSeconds int                  `json:"activeDeadlineSeconds"`
}

type NativeInputBinding struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// DKSWorkerResolverBinding is runtime-only private placement data. Worker is
// the privacy-safe logical ID from a placement plan; Node and OutpostHost are
// resolved only while compiling a concrete submission and must not be
// committed with the portable pipeline or plan.
type DKSWorkerResolverBinding struct {
	Schema  string                `json:"schema"`
	Workers []DKSWorkerResolution `json:"workers"`
}

type DKSWorkerResolution struct {
	Worker         string `json:"worker"`
	Node           string `json:"node"`
	Backend        string `json:"backend"`
	OutpostHost    string `json:"outpostHost,omitempty"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	TopologyClass  string `json:"topologyClass,omitempty"`
	TopologyDomain string `json:"topologyDomain,omitempty"`
}

func DecodeMixedArgoBinding(data []byte) (MixedArgoBinding, error) {
	var binding MixedArgoBinding
	if err := decodeStrict(data, &binding); err != nil {
		return binding, err
	}
	if err := binding.Validate(); err != nil {
		return binding, err
	}
	return binding, nil
}

func DecodeDKSWorkerResolver(data []byte) (DKSWorkerResolverBinding, error) {
	var resolver DKSWorkerResolverBinding
	if err := decodeStrict(data, &resolver); err != nil {
		return resolver, err
	}
	if err := resolver.Validate(); err != nil {
		return resolver, err
	}
	return resolver, nil
}

func (binding MixedArgoBinding) Validate() error {
	if binding.Schema != MixedArgoBindingSchema {
		return fmt.Errorf("schema: got %q, want %q", binding.Schema, MixedArgoBindingSchema)
	}
	if binding.Execution.Schema != ArgoBindingSchemaV2 {
		return fmt.Errorf("execution.schema: got %q, want %q", binding.Execution.Schema, ArgoBindingSchemaV2)
	}
	if err := binding.Execution.Validate(); err != nil {
		return fmt.Errorf("execution: %w", err)
	}
	if !kubeNameRE.MatchString(binding.ServiceAccountName) || len(binding.ServiceAccountName) > 253 {
		return errors.New("serviceAccountName: must be a Kubernetes-safe DNS name")
	}
	if !validPinnedImage(binding.ResultValidatorImage) {
		return errors.New("resultValidatorImage: must be pinned by lowercase sha256 digest")
	}
	seen := map[string]bool{}
	for i, native := range binding.Native {
		if seen[native.Task] {
			return fmt.Errorf("native[%d].task: duplicate %q", i, native.Task)
		}
		seen[native.Task] = true
		if err := validateID("task", native.Task); err != nil {
			return fmt.Errorf("native[%d]: %w", i, err)
		}
		if !validArtifactURL(native.ExecutableURL) {
			return fmt.Errorf("native[%d].executableUrl: must be HTTPS or loopback HTTP without credentials", i)
		}
		if !sha256RE.MatchString(native.ExecutableSHA256) {
			return fmt.Errorf("native[%d].executableSha256: must be lowercase SHA-256", i)
		}
		if !cleanRelativePath(native.ExecutablePath) {
			return fmt.Errorf("native[%d].executablePath: must be a clean relative path", i)
		}
		if native.TaintKey == "" || strings.ContainsAny(native.TaintKey, " \t\r\n") {
			return fmt.Errorf("native[%d].taintKey: must be non-empty", i)
		}
		if native.ActiveDeadlineSeconds < 1 {
			return fmt.Errorf("native[%d].activeDeadlineSeconds: must be positive", i)
		}
		inputs := map[string]bool{}
		for j, input := range native.Inputs {
			if inputs[input.Name] {
				return fmt.Errorf("native[%d].inputs[%d].name: duplicate %q", i, j, input.Name)
			}
			inputs[input.Name] = true
			if err := validateID("name", input.Name); err != nil {
				return fmt.Errorf("native[%d].inputs[%d]: %w", i, j, err)
			}
			if !validArtifactURL(input.URL) {
				return fmt.Errorf("native[%d].inputs[%d].url: must be HTTPS or loopback HTTP without credentials", i, j)
			}
		}
	}
	return nil
}

func (resolver DKSWorkerResolverBinding) Validate() error {
	if resolver.Schema != DKSWorkerResolverSchema {
		return fmt.Errorf("schema: got %q, want %q", resolver.Schema, DKSWorkerResolverSchema)
	}
	if len(resolver.Workers) == 0 {
		return errors.New("workers: must not be empty")
	}
	workers, nodes := map[string]bool{}, map[string]bool{}
	for i, worker := range resolver.Workers {
		if strings.TrimSpace(worker.Worker) == "" {
			return fmt.Errorf("workers[%d].worker: must not be empty", i)
		}
		if workers[worker.Worker] {
			return fmt.Errorf("workers[%d].worker: duplicate %q", i, worker.Worker)
		}
		workers[worker.Worker] = true
		if !kubeNameRE.MatchString(worker.Node) || len(worker.Node) > 63 {
			return fmt.Errorf("workers[%d].node: must be a Kubernetes-safe node name", i)
		}
		if nodes[worker.Node] {
			return fmt.Errorf("workers[%d].node: duplicate %q", i, worker.Node)
		}
		nodes[worker.Node] = true
		switch worker.Backend {
		case "k3s":
			if worker.OutpostHost != "" {
				return fmt.Errorf("workers[%d].outpostHost: must be absent for k3s", i)
			}
		case "vk-native":
			if !labelValueRE.MatchString(worker.OutpostHost) {
				return fmt.Errorf("workers[%d].outpostHost: must be a Kubernetes label value", i)
			}
		default:
			return fmt.Errorf("workers[%d].backend: unsupported %q", i, worker.Backend)
		}
		if worker.OS == "" || worker.Arch == "" {
			return fmt.Errorf("workers[%d]: os and arch are required", i)
		}
		if (worker.TopologyClass == "") != (worker.TopologyDomain == "") {
			return fmt.Errorf("workers[%d]: topologyClass and topologyDomain must be declared together", i)
		}
	}
	return nil
}

var labelValueRE = regexp.MustCompile(`^(?:[A-Za-z0-9](?:[-A-Za-z0-9_.]*[A-Za-z0-9])?)$`)

func validPinnedImage(image string) bool {
	name, _, ok := strings.Cut(image, "@sha256:")
	return ok && strings.Count(image, "@sha256:") == 1 && name != "" &&
		!strings.ContainsAny(name, " \t\r\n") && imageDigest.MatchString(image)
}

func validArtifactURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" && parsed.Host != "" {
		return true
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	return parsed.Scheme == "http" && (host == "localhost" || ip != nil && ip.IsLoopback())
}

// LowerMixedDKS emits a deterministic, whole-pipeline Argo Workflow. All
// native placement must already be present in plan and resolved by resolver.
// Multi-worker topology cohorts fail closed until a real gang controller is
// part of the runtime contract.
func LowerMixedDKS(p Pipeline, plan DKSPlacementPlan, binding MixedArgoBinding, resolver DKSWorkerResolverBinding) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("pipeline: %w", err)
	}
	if p.Schema != PipelineSchemaV2 {
		return nil, fmt.Errorf("pipeline: mixed DKS lowering requires %s", PipelineSchemaV2)
	}
	if len(p.Pipeline) > 40 || !kubeNameRE.MatchString(p.Pipeline) {
		return nil, fmt.Errorf("pipeline %q is not a Kubernetes-safe name of at most 40 characters", p.Pipeline)
	}
	if err := binding.Validate(); err != nil {
		return nil, fmt.Errorf("binding: %w", err)
	}
	if err := resolver.Validate(); err != nil {
		return nil, fmt.Errorf("resolver: %w", err)
	}
	if plan.Schema != DKSPlacementPlanSchema || plan.Pipeline != p.Pipeline {
		return nil, errors.New("placement plan schema/pipeline does not match pipeline")
	}

	matrix := map[string]MatrixEntry{}
	tasksByID := map[string]Task{}
	for _, task := range p.Tasks {
		tasksByID[task.ID] = task
	}
	for _, entry := range p.Matrix {
		if _, exists := matrix[entry.Task]; exists {
			return nil, fmt.Errorf("task %q has multiple matrix rows", entry.Task)
		}
		matrix[entry.Task] = entry
	}
	taskBindings := map[string]ArgoTaskBinding{}
	for _, task := range binding.Execution.Tasks {
		taskBindings[task.ID] = task
	}
	if len(taskBindings) != len(p.Tasks) {
		return nil, fmt.Errorf("execution task coverage mismatch: got %d, want %d", len(taskBindings), len(p.Tasks))
	}
	nativeBindings := map[string]NativeTaskBinding{}
	for _, native := range binding.Native {
		nativeBindings[native.Task] = native
	}
	resolutions := map[string]DKSWorkerResolution{}
	for _, worker := range resolver.Workers {
		resolutions[worker.Worker] = worker
	}
	assignments, usedWorkers, err := mixedAssignments(p, plan)
	if err != nil {
		return nil, err
	}
	if len(usedWorkers) != len(resolutions) {
		return nil, fmt.Errorf("resolver worker coverage mismatch: got %d, want %d", len(resolutions), len(usedWorkers))
	}
	for worker := range usedWorkers {
		if _, ok := resolutions[worker]; !ok {
			return nil, fmt.Errorf("resolver lacks logical worker %q", worker)
		}
	}
	for _, cohort := range plan.Cohorts {
		resolution := resolutions[cohort.Workers[0]]
		if cohort.TopologyClass != "" && (resolution.TopologyClass != cohort.TopologyClass ||
			resolution.TopologyDomain != cohort.TopologyDomain) {
			return nil, fmt.Errorf("task %q: runtime resolution does not match planned topology domain", cohort.Task)
		}
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
				"dhnt.io/placement":     DKSPlacementPlanSchema,
			},
		},
		Spec: argoSpec{
			Entrypoint:         "pipeline",
			ServiceAccountName: binding.ServiceAccountName,
			Volumes: []argoVolume{{
				Name: "workspace", PersistentVolumeClaim: argoPVC{ClaimName: binding.Execution.Workspace.ClaimName},
			}},
		},
	}
	dagTemplate := argoTemplate{Name: "pipeline"}
	tasks := append([]Task(nil), p.Tasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	claimedPaths := map[string]string{}
	for index, task := range tasks {
		entry := matrix[task.ID]
		taskBinding, ok := taskBindings[task.ID]
		if !ok {
			return nil, fmt.Errorf("task %q: missing execution binding", task.ID)
		}
		spec, err := runnerSpecForTask(p, task, entry, taskBinding)
		if err != nil {
			return nil, fmt.Errorf("task %q: %w", task.ID, err)
		}
		for _, artifact := range append(append([]RunnerArtifact{}, spec.Inputs...), spec.Outputs...) {
			if prior, exists := claimedPaths[artifact.Path]; exists && prior != artifact.Name+"@"+artifact.SHA256 {
				return nil, fmt.Errorf("task %q: artifact path %q aliases a different identity", task.ID, artifact.Path)
			}
			claimedPaths[artifact.Path] = artifact.Name + "@" + artifact.SHA256
		}
		for _, runtimePath := range []struct {
			value string
			kind  string
		}{
			{taskBinding.EvidenceDirectory, "evidence"},
			{taskBinding.CommitManifestPath, "commit"},
		} {
			if prior, exists := claimedPaths[runtimePath.value]; exists {
				return nil, fmt.Errorf("task %q: %s path %q aliases %s", task.ID, runtimePath.kind, runtimePath.value, prior)
			}
			claimedPaths[runtimePath.value] = task.ID + " " + runtimePath.kind
		}
		dagTemplate.DAG.Tasks = append(dagTemplate.DAG.Tasks, argoDAGTask{
			Name: task.ID, Template: task.ID, Dependencies: sortedStrings(task.Needs),
		})
		switch task.Lane {
		case LaneCluster, LaneContainer:
			template, err := lowerMixedClusterTask(task, entry, taskBinding, spec, assignments, resolutions, binding.Execution.Workspace)
			if err != nil {
				return nil, err
			}
			workflow.Spec.Templates = append(workflow.Spec.Templates, template)
		case LaneNative:
			native, ok := nativeBindings[task.ID]
			if !ok {
				return nil, fmt.Errorf("task %q: missing native binding", task.ID)
			}
			templates, err := lowerMixedNativeTask(index, p, task, entry, taskBinding, native,
				assignments, resolutions, binding)
			if err != nil {
				return nil, err
			}
			workflow.Spec.Templates = append(workflow.Spec.Templates, templates...)
		default:
			return nil, fmt.Errorf("task %q: lane %q is unsupported by mixed DKS", task.ID, task.Lane)
		}
	}
	for task := range nativeBindings {
		if tasksByID[task].Lane != LaneNative {
			return nil, fmt.Errorf("native binding for non-native or unknown task %q", task)
		}
	}
	if err := rejectPathAliases(claimedPaths); err != nil {
		return nil, fmt.Errorf("workflow paths: %w", err)
	}
	workflow.Spec.Templates = append([]argoTemplate{dagTemplate}, workflow.Spec.Templates...)
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(workflow); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func mixedAssignments(p Pipeline, plan DKSPlacementPlan) (map[string]string, map[string]bool, error) {
	assignments := map[string]string{}
	used := map[string]bool{}
	tasks := map[string]Task{}
	matrix := map[string]MatrixEntry{}
	for _, task := range p.Tasks {
		tasks[task.ID] = task
	}
	for _, entry := range p.Matrix {
		matrix[entry.Task] = entry
	}
	for _, cohort := range plan.Cohorts {
		task, ok := tasks[cohort.Task]
		if !ok || task.Placement == nil || task.Distribution == DistributionShardable {
			return nil, nil, fmt.Errorf("placement plan has unexpected cohort task %q", cohort.Task)
		}
		if len(cohort.Workers) != 1 {
			return nil, nil, fmt.Errorf("task %q: multi-host topology cohort of %d is unsupported without gang scheduling",
				cohort.Task, len(cohort.Workers))
		}
		if strings.TrimSpace(cohort.Workers[0]) == "" {
			return nil, nil, fmt.Errorf("task %q has an empty cohort worker", cohort.Task)
		}
		if task.Distribution == DistributionTopologyCoupled &&
			(cohort.TopologyClass != task.Placement.TopologyKey || cohort.TopologyDomain == "") {
			return nil, nil, fmt.Errorf("task %q placement plan lost its topology constraint", cohort.Task)
		}
		if assignments[cohort.Task] != "" {
			return nil, nil, fmt.Errorf("task %q has duplicate placement", cohort.Task)
		}
		assignments[cohort.Task] = cohort.Workers[0]
		used[cohort.Workers[0]] = true
	}
	for _, reduction := range plan.Reductions {
		required := make([]int, 0, len(reduction.Chunks))
		manifest := &dag.ChunkManifest{
			SchemaVersion: 1, Suite: reduction.Reducer, ChunkCount: len(reduction.Chunks),
		}
		for _, chunk := range reduction.Chunks {
			if len(chunk.Members) != 1 {
				return nil, nil, fmt.Errorf("reducer %q chunk %d must name exactly one expanded task", reduction.Reducer, chunk.Index)
			}
			task := chunk.Members[0]
			taskContract, ok := tasks[task]
			if !ok || taskContract.Distribution != DistributionShardable ||
				taskContract.Reducer != reduction.Reducer || matrix[task].Chunk == nil ||
				matrix[task].Chunk.Index != chunk.Index ||
				matrix[task].Chunk.ManifestSHA256 != reduction.ManifestSHA256 {
				return nil, nil, fmt.Errorf("reducer %q chunk %d does not match pipeline shard %q",
					reduction.Reducer, chunk.Index, task)
			}
			if strings.TrimSpace(chunk.Worker) == "" {
				return nil, nil, fmt.Errorf("reducer %q chunk %d has no worker", reduction.Reducer, chunk.Index)
			}
			if assignments[task] != "" {
				return nil, nil, fmt.Errorf("task %q has duplicate placement", task)
			}
			assignments[task] = chunk.Worker
			used[chunk.Worker] = true
			required = append(required, chunk.Index)
			manifest.Chunks = append(manifest.Chunks, dag.Chunk{
				ID: chunk.Index, Fixtures: []dag.Fixture{{Name: task}},
			})
		}
		sort.Ints(required)
		if !slices.Equal(required, reduction.RequiredChunkIndex) {
			return nil, nil, fmt.Errorf("reducer %q required chunk indexes do not match assignments", reduction.Reducer)
		}
		if strings.TrimPrefix(manifest.MembershipHash(), "m") != reduction.MembershipSHA256 {
			return nil, nil, fmt.Errorf("reducer %q membership digest does not match assignments", reduction.Reducer)
		}
	}
	for _, task := range p.Tasks {
		if task.Lane == LaneNative && assignments[task.ID] == "" {
			return nil, nil, fmt.Errorf("native task %q has no placement assignment", task.ID)
		}
		if task.Distribution == DistributionShardable && assignments[task.ID] == "" {
			return nil, nil, fmt.Errorf("shardable task %q has no placement assignment", task.ID)
		}
		if task.Placement != nil && assignments[task.ID] == "" {
			return nil, nil, fmt.Errorf("placed task %q has no placement assignment", task.ID)
		}
	}
	return assignments, used, nil
}

func lowerMixedClusterTask(task Task, entry MatrixEntry, binding ArgoTaskBinding, spec RunnerSpec,
	assignments map[string]string, resolutions map[string]DKSWorkerResolution, workspace ArgoWorkspace,
) (argoTemplate, error) {
	if entry.Platform.Backend != "k3s" || entry.Platform.OS != "linux" {
		return argoTemplate{}, fmt.Errorf("task %q: cluster lane requires k3s/linux, got %s/%s",
			task.ID, entry.Platform.Backend, entry.Platform.OS)
	}
	selector := map[string]string{
		"kubernetes.io/arch": entry.Platform.Arch, "kubernetes.io/os": entry.Platform.OS,
		"outpost.dhnt.io/backend": "k3s",
	}
	if worker := assignments[task.ID]; worker != "" {
		resolution := resolutions[worker]
		if err := validateResolutionForTask(task, entry, resolution); err != nil {
			return argoTemplate{}, err
		}
		selector["kubernetes.io/hostname"] = resolution.Node
	}
	specJSON, err := MarshalRunnerSpec(spec)
	if err != nil {
		return argoTemplate{}, err
	}
	template := argoTemplate{
		Name: task.ID,
		Metadata: &argoTemplateMetadata{Annotations: map[string]string{
			"dhnt.io/distribution": string(task.Distribution), "dhnt.io/runner-contract": RunnerSpecSchema,
		}},
		NodeSelector: selector,
		Container: &argoContainer{
			Image: binding.Image, Command: []string{binding.RunnerPath},
			Args: append([]string{"dhnt", "run-task", "--workspace", workspace.MountPath,
				"--spec-base64", base64.StdEncoding.EncodeToString(specJSON), "--"}, task.Argv...),
			WorkingDir: path.Join(workspace.MountPath, task.WorkingDirectory),
			Env: []argoEnv{
				{Name: "DHNT_EXECUTOR_NODE", ValueFrom: &argoEnvSource{FieldRef: &argoFieldRef{FieldPath: "spec.nodeName"}}},
				{Name: "DHNT_POD_UID", ValueFrom: &argoEnvSource{FieldRef: &argoFieldRef{FieldPath: "metadata.uid"}}},
			},
			VolumeMounts: []argoVolumeMount{{Name: "workspace", MountPath: workspace.MountPath}},
		},
	}
	requests, err := placementResourceRequests(task)
	if err != nil {
		return argoTemplate{}, err
	}
	if len(requests) > 0 {
		template.Container.Resources = &argoResourceRequirements{Requests: requests}
	}
	template.ActiveDeadlineSeconds = binding.TimeoutSeconds
	template.RetryStrategy = &argoRetryStrategy{Limit: strconv.Itoa(*binding.RetryLimit)}
	return template, nil
}

func lowerMixedNativeTask(index int, pipeline Pipeline, task Task, entry MatrixEntry, taskBinding ArgoTaskBinding,
	native NativeTaskBinding, assignments map[string]string,
	resolutions map[string]DKSWorkerResolution, binding MixedArgoBinding,
) ([]argoTemplate, error) {
	if task.Distribution == DistributionTopologyCoupled && task.Placement.CohortSize != 1 {
		return nil, fmt.Errorf("task %q: multi-host gang semantics are unsupported", task.ID)
	}
	if entry.Platform.Backend != "vk-native" {
		return nil, fmt.Errorf("task %q: native lane requires vk-native backend", task.ID)
	}
	if *taskBinding.RetryLimit != 0 {
		return nil, fmt.Errorf("task %q: native retries must be zero; retrying onto a different attempt is unsupported", task.ID)
	}
	if *taskBinding.TimeoutSeconds != native.ActiveDeadlineSeconds {
		return nil, fmt.Errorf("task %q: native active deadline must equal execution timeout", task.ID)
	}
	resolution := resolutions[assignments[task.ID]]
	if err := validateResolutionForTask(task, entry, resolution); err != nil {
		return nil, err
	}
	if len(entry.Outputs) != 1 || entry.Outputs[0].Kind != ArtifactFile {
		return nil, fmt.Errorf("task %q: native bounded result requires exactly one file output", task.ID)
	}
	inputURLs := map[string]string{}
	for _, input := range native.Inputs {
		inputURLs[input.Name] = input.URL
	}
	if len(inputURLs) != len(entry.Inputs) {
		return nil, fmt.Errorf("task %q: native input URL coverage mismatch", task.ID)
	}
	env := append([]Environment{}, task.Environment...)
	for _, item := range env {
		if strings.HasPrefix(item.Name, "DHNT_") {
			return nil, fmt.Errorf("task %q: environment name %q is reserved by the native runtime contract", task.ID, item.Name)
		}
	}
	for _, input := range entry.Inputs {
		inputURL, ok := inputURLs[input.Name]
		if !ok {
			return nil, fmt.Errorf("task %q: native binding lacks input URL for %q", task.ID, input.Name)
		}
		base := "DHNT_INPUT_" + artifactEnvName(input.Name)
		env = append(env, Environment{Name: base + "_URL", Value: inputURL}, Environment{Name: base + "_SHA256", Value: input.SHA256})
	}
	output := entry.Outputs[0]
	env = append(env,
		Environment{Name: "DHNT_EXECUTOR_NODE", Value: resolution.Node},
		Environment{Name: "DHNT_BACKEND", Value: "vk-native"},
		Environment{Name: "DHNT_TARGET_OS", Value: entry.Platform.OS},
		Environment{Name: "DHNT_TARGET_ARCH", Value: entry.Platform.Arch},
		Environment{Name: "DHNT_EXPECT_OUTPUT_NAME", Value: output.Name},
		Environment{Name: "DHNT_EXPECT_OUTPUT_SHA256", Value: output.SHA256},
	)
	sort.Slice(env, func(i, j int) bool { return env[i].Name < env[j].Name })
	jobName := fmt.Sprintf("{{workflow.name}}-n%02d", index)
	requests, err := placementResourceRequests(task)
	if err != nil {
		return nil, err
	}
	job := nativeJobManifest(jobName, pipeline, task, entry, native, resolution, env, requests)
	wrapperName := task.ID
	createName, collectName := task.ID+"-create", task.ID+"-collect"
	wrapper := argoTemplate{Name: wrapperName, Steps: [][]argoStep{
		{{Name: "create", Template: createName}},
		{{Name: "collect", Template: collectName}},
	}}
	create := argoTemplate{Name: createName, Resource: &argoResource{
		Action: "create", SetOwnerReference: true,
		SuccessCondition: "status.succeeded > 0", FailureCondition: "status.failed > 0", Manifest: job,
	}}
	artifactPath := findArtifactPath(taskBinding, output.Name)
	if artifactPath == "" {
		return nil, fmt.Errorf("task %q: output %q has no runtime path", task.ID, output.Name)
	}
	evidencePath := path.Join(binding.Execution.Workspace.MountPath, taskBinding.EvidenceDirectory, "run.json")
	commitPath := path.Join(binding.Execution.Workspace.MountPath, taskBinding.CommitManifestPath)
	outputPath := path.Join(binding.Execution.Workspace.MountPath, artifactPath)
	script := nativeCollectorScript(jobName, resolution, entry.Platform, output, outputPath, evidencePath, commitPath, taskBinding.RunnerPath)
	collect := argoTemplate{
		Name: collectName,
		NodeSelector: map[string]string{
			"outpost.dhnt.io/backend": "k3s", "kubernetes.io/os": "linux",
		},
		Container: &argoContainer{
			Image: binding.ResultValidatorImage, Command: []string{taskBinding.RunnerPath, "-c"}, Args: []string{script},
			VolumeMounts: []argoVolumeMount{{Name: "workspace", MountPath: binding.Execution.Workspace.MountPath}},
		},
	}
	return []argoTemplate{wrapper, create, collect}, nil
}

func validateResolutionForTask(task Task, entry MatrixEntry, resolution DKSWorkerResolution) error {
	if resolution.Worker == "" {
		return fmt.Errorf("task %q: assigned worker has no runtime resolution", task.ID)
	}
	if resolution.Backend != entry.Platform.Backend || resolution.OS != entry.Platform.OS || resolution.Arch != entry.Platform.Arch {
		return fmt.Errorf("task %q: resolution %q is %s/%s/%s, want %s/%s/%s", task.ID, resolution.Worker,
			resolution.Backend, resolution.OS, resolution.Arch,
			entry.Platform.Backend, entry.Platform.OS, entry.Platform.Arch)
	}
	if task.Distribution == DistributionTopologyCoupled {
		if resolution.TopologyClass != task.Placement.TopologyKey {
			return fmt.Errorf("task %q: resolution topology class does not match placement", task.ID)
		}
	}
	return nil
}

func findArtifactPath(binding ArgoTaskBinding, name string) string {
	for _, artifact := range binding.Artifacts {
		if artifact.Name == name {
			return artifact.Path
		}
	}
	return ""
}

func nativeJobManifest(name string, pipeline Pipeline, task Task, entry MatrixEntry, native NativeTaskBinding,
	resolution DKSWorkerResolution, environment []Environment, requests map[string]string,
) string {
	env := make([]map[string]string, 0, len(environment))
	for _, item := range environment {
		env = append(env, map[string]string{"name": item.Name, "value": item.Value})
	}
	manifest := map[string]any{
		"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{
			"name":   name,
			"labels": map[string]string{"dhnt.io/pipeline": pipeline.Pipeline, "dhnt.io/task": task.ID},
		},
		"spec": map[string]any{
			"backoffLimit": 0, "activeDeadlineSeconds": native.ActiveDeadlineSeconds,
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]string{"dhnt.io/pipeline": pipeline.Pipeline, "dhnt.io/task": task.ID},
					"annotations": map[string]string{
						"outpost.dhnt.io/native-artifact-url":    native.ExecutableURL,
						"outpost.dhnt.io/native-artifact-sha256": native.ExecutableSHA256,
						"outpost.dhnt.io/native-artifact-path":   native.ExecutablePath,
						"outpost.dhnt.io/termination-log-tail":   "true",
					},
				},
				"spec": map[string]any{
					"restartPolicy": "Never", "automountServiceAccountToken": false,
					"nodeSelector": map[string]string{
						"outpost.dhnt.io/backend": "vk-native", "outpost.dhnt.io/host": resolution.OutpostHost,
						"kubernetes.io/hostname": resolution.Node, "kubernetes.io/os": entry.Platform.OS,
						"kubernetes.io/arch": entry.Platform.Arch,
					},
					"tolerations": []map[string]string{{"key": native.TaintKey, "operator": "Exists", "effect": "NoSchedule"}},
					"containers": []any{map[string]any{
						"name": "payload", "image": "dhnt.io/native-process", "imagePullPolicy": "Never",
						"command": task.Argv, "env": env,
						"resources": map[string]any{"requests": requests},
					}},
				},
			},
		},
	}
	data, _ := yaml.Marshal(manifest)
	return string(data)
}

func placementResourceRequests(task Task) (map[string]string, error) {
	if task.Placement == nil {
		return nil, nil
	}
	placement := task.Placement
	requests := map[string]string{}
	if placement.CPU > 0 {
		requests["cpu"] = strconv.Itoa(placement.CPU)
	}
	if placement.MemoryBytes > 0 {
		requests["memory"] = strconv.FormatUint(placement.MemoryBytes, 10)
	}
	for resource, minimum := range placement.MinimumCapacity {
		requests[resource] = strconv.FormatUint(minimum, 10)
	}
	if placement.AcceleratorCount > 0 {
		var resource string
		switch placement.AcceleratorKind {
		case "cuda", "nvidia":
			resource = "nvidia.com/gpu"
		case "rocm", "amd":
			resource = "amd.com/gpu"
		default:
			return nil, fmt.Errorf("task %q: accelerator kind %q has no Kubernetes scalar resource mapping",
				task.ID, placement.AcceleratorKind)
		}
		requests[resource] = strconv.Itoa(placement.AcceleratorCount)
	}
	return requests, nil
}

func nativeCollectorScript(job string, resolution DKSWorkerResolution, platform Platform, artifact Artifact,
	outputPath, evidencePath, commitPath, runner string,
) string {
	return fmt.Sprintf(`set -eu
job=%s
namespace="{{workflow.namespace}}"
job_uid=$(%s kubectl get job "$job" -n "$namespace" -o jsonpath='{.metadata.uid}')
pods=$(%s kubectl get pods -n "$namespace" -l "job-name=$job" -o jsonpath='{range .items[?(@.status.phase=="Succeeded")]}{.metadata.name}{"\n"}{end}')
count=$(printf '%%s\n' "$pods" | %s grep -c . || true)
[ "$count" -eq 1 ]
pod="$pods"
owner_uid=$(%s kubectl get pod "$pod" -n "$namespace" -o jsonpath='{.metadata.ownerReferences[0].uid}')
[ "$owner_uid" = "$job_uid" ]
node=$(%s kubectl get pod "$pod" -n "$namespace" -o jsonpath='{.spec.nodeName}')
[ "$node" = %s ]
message=$(%s kubectl get pod "$pod" -n "$namespace" -o jsonpath='{.status.containerStatuses[0].state.terminated.message}')
%s mkdir -p %s %s %s
printf '%%s' "$message" > /tmp/dhnt-native-message
%s dhnt verify-native-result --expect-name %s --expect-kind file --expect-sha256 %s --expect-node "$node" --expect-backend vk-native --expect-os %s --expect-arch %s --artifact-output %s --commit-output %s /tmp/dhnt-native-message > %s
`,
		shellQuote(job), shellQuote(runner), shellQuote(runner), shellQuote(runner), shellQuote(runner),
		shellQuote(runner), shellQuote(resolution.Node), shellQuote(runner), shellQuote(runner),
		shellQuote(path.Dir(outputPath)), shellQuote(path.Dir(evidencePath)), shellQuote(path.Dir(commitPath)), shellQuote(runner),
		shellQuote(artifact.Name), shellQuote(artifact.SHA256), shellQuote(platform.OS), shellQuote(platform.Arch),
		shellQuote(outputPath), shellQuote(commitPath), shellQuote(evidencePath))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
