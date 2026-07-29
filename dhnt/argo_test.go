package dhnt

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLowerArgoCheckedInAllClusterFixture(t *testing.T) {
	pipelineData, err := os.ReadFile("testdata/argo.pipeline.json")
	if err != nil {
		t.Fatal(err)
	}
	bindingData, err := os.ReadFile("testdata/argo.binding.json")
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := DecodePipeline(pipelineData)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := DecodeArgoBinding(bindingData)
	if err != nil {
		t.Fatal(err)
	}
	output, err := LowerArgo(pipeline, binding)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "kind: Workflow") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestLowerArgoRejectsUmbrellaNanochatFixtureWithoutFiltering(t *testing.T) {
	data, err := os.ReadFile("../../integrations/nanochat/examples/smoke/pipeline.json")
	if os.IsNotExist(err) {
		t.Skip("umbrella nanochat fixture is not present in a standalone bashy clone")
	}
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := DecodePipeline(data)
	if err != nil {
		t.Fatal(err)
	}
	binding := ArgoBinding{
		Schema: ArgoBindingSchema,
		Workspace: ArgoWorkspace{
			ClaimName: "nanochat-workspace",
			MountPath: "/workspace",
		},
	}
	matrix := make(map[string]MatrixEntry, len(pipeline.Matrix))
	for _, entry := range pipeline.Matrix {
		matrix[entry.Task] = entry
	}
	for _, task := range pipeline.Tasks {
		taskBinding := ArgoTaskBinding{
			ID:    task.ID,
			Image: "registry.example/nanochat@sha256:" + strings.Repeat("e", 64),
		}
		names := map[string]bool{}
		for _, artifact := range append(append([]Artifact{}, matrix[task.ID].Inputs...), matrix[task.ID].Outputs...) {
			if !names[artifact.Name] {
				taskBinding.Artifacts = append(taskBinding.Artifacts, ArgoArtifactBinding{
					Name: artifact.Name,
					Path: "artifacts/" + artifact.Name,
				})
				names[artifact.Name] = true
			}
		}
		binding.Tasks = append(binding.Tasks, taskBinding)
	}
	_, err = LowerArgo(pipeline, binding)
	if err == nil || (!strings.Contains(err.Error(), `lane "native"`) &&
		!strings.Contains(err.Error(), `distribution "topology-coupled"`)) {
		t.Fatalf("got %v, want whole-pipeline rejection of native/topology-coupled nanochat stages", err)
	}
}

func argoFixture() (Pipeline, ArgoBinding) {
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)
	digestC := strings.Repeat("c", 64)
	chunkDigest := strings.Repeat("d", 64)
	timeout, retries := 300, 2
	pipeline := Pipeline{
		Schema: PipelineSchema, Pipeline: "nanochat-smoke",
		Source: Source{
			Repository: "https://github.com/karpathy/nanochat.git",
			Commit:     "abc123",
			SHA256:     digestA,
		},
		Tasks: []Task{
			{
				ID: "prepare-01", Lane: LaneCluster, Distribution: DistributionShardable,
				Needs: []string{}, Argv: []string{"./run-stage.sh", "prepare", "--chunk", "1/1"},
				WorkingDirectory: ".", Environment: []Environment{{Name: "PROFILE", Value: "smoke"}},
			},
			{
				ID: "train", Lane: LaneContainer, Distribution: DistributionSingle,
				Needs: []string{"prepare-01"}, Argv: []string{"python", "-m", "nanochat.train"},
				WorkingDirectory: "src", Environment: []Environment{},
			},
		},
		Matrix: []MatrixEntry{
			{
				Task: "prepare-01", Platform: Platform{Backend: "k3s", OS: "linux", Arch: "arm64"},
				Chunk:   &Chunk{Index: 1, Count: 1, ManifestSHA256: chunkDigest},
				Inputs:  []Artifact{{Name: "source", SHA256: digestA}},
				Outputs: []Artifact{{Name: "dataset", SHA256: digestB}},
			},
			{
				Task: "train", Platform: Platform{Backend: "k3s", OS: "linux", Arch: "amd64"},
				Inputs:  []Artifact{{Name: "dataset", SHA256: digestB}},
				Outputs: []Artifact{{Name: "model", SHA256: digestC}},
			},
		},
	}
	binding := ArgoBinding{
		Schema: ArgoBindingSchema,
		Workspace: ArgoWorkspace{
			ClaimName: "nanochat-workspace",
			MountPath: "/workspace",
		},
		Tasks: []ArgoTaskBinding{
			{
				ID: "train", Image: "registry.example/nanochat@sha256:" + digestC,
				Artifacts:      []ArgoArtifactBinding{{Name: "model", Path: "artifacts/model"}, {Name: "dataset", Path: "artifacts/dataset"}},
				TimeoutSeconds: &timeout, RetryLimit: &retries,
			},
			{
				ID: "prepare-01", Image: "registry.example/nanochat@sha256:" + digestC,
				Artifacts: []ArgoArtifactBinding{{Name: "dataset", Path: "artifacts/dataset"}, {Name: "source", Path: "source"}},
			},
		},
	}
	return pipeline, binding
}

func TestLowerArgoDeterministicAndPreservesExecutionContract(t *testing.T) {
	pipeline, binding := argoFixture()
	first, err := LowerArgo(pipeline, binding)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LowerArgo(pipeline, binding)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("Argo output is not deterministic")
	}
	var workflow argoWorkflow
	if err := yaml.Unmarshal(first, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.Spec.Templates) != 3 {
		t.Fatalf("got %d templates, want DAG plus two tasks", len(workflow.Spec.Templates))
	}
	dag, prepare, train := workflow.Spec.Templates[0], workflow.Spec.Templates[1], workflow.Spec.Templates[2]
	if len(dag.DAG.Tasks) != 2 || strings.Join(dag.DAG.Tasks[1].Dependencies, ",") != "prepare-01" {
		t.Fatalf("dependencies not preserved: %+v", dag.DAG.Tasks)
	}
	if prepare.NodeSelector["outpost.dhnt.io/backend"] != "k3s" ||
		train.NodeSelector["kubernetes.io/os"] != "linux" ||
		train.NodeSelector["kubernetes.io/arch"] != "amd64" {
		t.Fatalf("hard DKS selectors missing: %+v %+v", prepare.NodeSelector, train.NodeSelector)
	}
	if train.Container == nil || strings.Join(train.Container.Command, ",") != "python" ||
		strings.Join(train.Container.Args, ",") != "-m,nanochat.train" ||
		train.Container.WorkingDir != "/workspace/src" {
		t.Fatalf("process contract not preserved: %+v", train.Container)
	}
	env := map[string]string{}
	for _, item := range train.Container.Env {
		env[item.Name] = item.Value
	}
	if env["DHNT_INPUT_DATASET_PATH"] != "/workspace/artifacts/dataset" ||
		env["DHNT_OUTPUT_MODEL_SHA256"] != strings.Repeat("c", 64) {
		t.Fatalf("artifact contract missing: %+v", env)
	}
	output := string(first)
	for _, want := range []string{
		"apiVersion: argoproj.io/v1alpha1",
		"generateName: nanochat-smoke-",
		"outpost.dhnt.io/backend: k3s",
		"kubernetes.io/os: linux",
		"kubernetes.io/arch: amd64",
		"workingDir: /workspace/src",
		"activeDeadlineSeconds: 300",
		"limit: \"2\"",
		"claimName: nanochat-workspace",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output lacks %q:\n%s", want, output)
		}
	}
}

func TestLowerArgoFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Pipeline, *ArgoBinding)
		want string
	}{
		{
			name: "native lane",
			edit: func(p *Pipeline, _ *ArgoBinding) {
				p.Tasks[0].Lane = LaneNative
				p.Matrix[0].Platform.Backend = "vk-native"
			},
			want: "lane \"native\"",
		},
		{
			name: "topology coupled",
			edit: func(p *Pipeline, _ *ArgoBinding) {
				p.Tasks[0].Distribution = DistributionTopologyCoupled
				p.Matrix[0].Chunk = nil
			},
			want: "no fake replication, DDP",
		},
		{
			name: "cloud backend",
			edit: func(p *Pipeline, _ *ArgoBinding) {
				p.Matrix[0].Platform.Backend = "cloud"
			},
			want: "requires platform backend k3s",
		},
		{
			name: "missing image",
			edit: func(_ *Pipeline, b *ArgoBinding) {
				b.Tasks[0].Image = ""
			},
			want: "pinned by lowercase sha256",
		},
		{
			name: "unpinned image",
			edit: func(_ *Pipeline, b *ArgoBinding) {
				b.Tasks[0].Image = "registry.example/nanochat:latest"
			},
			want: "pinned by lowercase sha256",
		},
		{
			name: "missing artifact path",
			edit: func(_ *Pipeline, b *ArgoBinding) {
				b.Tasks[0].Artifacts = b.Tasks[0].Artifacts[:1]
			},
			want: "binding lacks input artifact",
		},
		{
			name: "undeclared artifact path",
			edit: func(_ *Pipeline, b *ArgoBinding) {
				b.Tasks[0].Artifacts = append(b.Tasks[0].Artifacts, ArgoArtifactBinding{Name: "secret", Path: "secret"})
			},
			want: "undeclared artifact",
		},
		{
			name: "multiple matrix rows",
			edit: func(p *Pipeline, _ *ArgoBinding) {
				extra := p.Matrix[0]
				extra.Platform.Arch = "amd64"
				p.Matrix = append(p.Matrix, extra)
			},
			want: "multiple matrix rows",
		},
		{
			name: "reserved runtime environment",
			edit: func(p *Pipeline, _ *ArgoBinding) {
				p.Tasks[0].Environment = append(p.Tasks[0].Environment, Environment{Name: "DHNT_TASK", Value: "spoof"})
			},
			want: "reserved",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, binding := argoFixture()
			tt.edit(&pipeline, &binding)
			_, err := LowerArgo(pipeline, binding)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestDecodeArgoBindingRejectsSecretOrUnknownFields(t *testing.T) {
	data := `{
	  "schema":"dhnt.argo-binding/v1",
	  "workspace":{"claimName":"workspace","mountPath":"/workspace"},
	  "tasks":[{
	    "id":"task",
	    "image":"example/image@sha256:` + strings.Repeat("a", 64) + `",
	    "artifacts":[{"name":"input","path":"input"}],
	    "secretRef":"not-supported"
	  }]
	}`
	if _, err := DecodeArgoBinding([]byte(data)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("got %v, want unknown secret field rejection", err)
	}
}
