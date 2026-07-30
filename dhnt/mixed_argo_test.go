package dhnt

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	dag "github.com/qiangli/coreutils/pkg/dag"
	"gopkg.in/yaml.v3"
)

func mixedArgoFixture(t *testing.T) (Pipeline, DKSPlacementPlan, MixedArgoBinding, DKSWorkerResolverBinding) {
	t.Helper()
	pipeline := placementPipeline()
	pipeline.Tasks[0].Lane, pipeline.Tasks[1].Lane = LaneCluster, LaneCluster
	pipeline.Matrix[0].Platform.Backend, pipeline.Matrix[1].Platform.Backend = "k3s", "k3s"
	pipeline.Tasks[3].Placement.CohortSize = 1
	now := time.Date(2026, 7, 30, 18, 0, 0, 0, time.UTC)
	facts := []dag.HostFacts{
		{SchemaVersion: 1, Worker: "cluster-a", OS: "linux", Arch: "amd64", CPU: 8, MemBytes: 16 << 30,
			Venues: []string{dag.VenueCluster}, Capacities: map[string]uint64{"dhnt.io/vram": 8 << 30},
			Topology: dag.TopologyFacts{Class: "kubernetes.io/hostname", Domain: "cluster-domain-a"}, ObservedAt: now},
		{SchemaVersion: 1, Worker: "cluster-b", OS: "linux", Arch: "amd64", CPU: 8, MemBytes: 16 << 30,
			Venues: []string{dag.VenueCluster}, Capacities: map[string]uint64{"dhnt.io/vram": 8 << 30},
			Topology: dag.TopologyFacts{Class: "kubernetes.io/hostname", Domain: "cluster-domain-b"}, ObservedAt: now},
		{SchemaVersion: 1, Worker: "native-a", OS: "linux", Arch: "amd64", CPU: 16, MemBytes: 32 << 30,
			Venues:       []string{dag.VenueNative},
			Accelerators: []dag.AcceleratorFacts{{Kind: "cuda", Family: "rtx-3070", Count: 1, MemoryBytes: 8 << 30}},
			Topology:     dag.TopologyFacts{Class: "kubernetes.io/hostname", Domain: "native-domain-a"}, ObservedAt: now},
	}
	plan, err := PlanDKSPlacement(pipeline, facts, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	timeout, retries := 300, 0
	binding := MixedArgoBinding{
		Schema: MixedArgoBindingSchema,
		Execution: ArgoBinding{
			Schema:    ArgoBindingSchemaV2,
			Workspace: ArgoWorkspace{ClaimName: "mixed-workspace", MountPath: "/workspace"},
		},
		ServiceAccountName:   "argo-workflow",
		ResultValidatorImage: "registry.example/bashy@sha256:" + strings.Repeat("d", 64),
	}
	matrix := map[string]MatrixEntry{}
	for _, entry := range pipeline.Matrix {
		matrix[entry.Task] = entry
	}
	for _, task := range pipeline.Tasks {
		taskBinding := ArgoTaskBinding{
			ID: task.ID, Image: "registry.example/workload@sha256:" + strings.Repeat("e", 64),
			RunnerPath: "/usr/local/bin/bashy", EvidenceDirectory: "evidence/" + task.ID,
			CommitManifestPath: "commits/" + task.ID + ".json", NonzeroClass: ResultInfraFail,
			TimeoutSeconds: &timeout, RetryLimit: &retries,
		}
		seen := map[string]bool{}
		for _, artifact := range append(append([]Artifact{}, matrix[task.ID].Inputs...), matrix[task.ID].Outputs...) {
			if seen[artifact.Name] {
				continue
			}
			seen[artifact.Name] = true
			taskBinding.Artifacts = append(taskBinding.Artifacts, ArgoArtifactBinding{
				Name: artifact.Name, Path: "artifacts/" + artifact.Name + "/" + artifact.SHA256,
			})
		}
		binding.Execution.Tasks = append(binding.Execution.Tasks, taskBinding)
		if task.Lane == LaneNative {
			native := NativeTaskBinding{
				Task: task.ID, ExecutableURL: "https://artifacts.example/bashy.tar.gz",
				ExecutableSHA256: strings.Repeat("f", 64), ExecutablePath: "bin/bashy",
				TaintKey: "virtual-kubelet.io/provider", ActiveDeadlineSeconds: timeout,
			}
			for _, input := range matrix[task.ID].Inputs {
				native.Inputs = append(native.Inputs, NativeInputBinding{
					Name: input.Name, URL: "https://artifacts.example/" + input.SHA256,
				})
			}
			binding.Native = append(binding.Native, native)
		}
	}
	resolver := DKSWorkerResolverBinding{
		Schema: DKSWorkerResolverSchema,
		Workers: []DKSWorkerResolution{
			{Worker: "cluster-a", Node: "private-cluster-a", Backend: "k3s", OS: "linux", Arch: "amd64",
				TopologyClass: "kubernetes.io/hostname", TopologyDomain: "cluster-domain-a"},
			{Worker: "cluster-b", Node: "private-cluster-b", Backend: "k3s", OS: "linux", Arch: "amd64",
				TopologyClass: "kubernetes.io/hostname", TopologyDomain: "cluster-domain-b"},
			{Worker: "native-a", Node: "private-native-a", Backend: "vk-native", OutpostHost: "private-host-a",
				OS: "linux", Arch: "amd64", TopologyClass: "kubernetes.io/hostname", TopologyDomain: "native-domain-a"},
		},
	}
	return pipeline, plan, binding, resolver
}

func TestLowerMixedDKSGeneratesDirectNativeAndReduceFanIn(t *testing.T) {
	pipeline, plan, binding, resolver := mixedArgoFixture(t)
	first, err := LowerMixedDKS(pipeline, plan, binding, resolver)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LowerMixedDKS(pipeline, plan, binding, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("mixed lowering is not deterministic")
	}
	var workflow argoWorkflow
	if err := yaml.Unmarshal(first, &workflow); err != nil {
		t.Fatal(err)
	}
	if workflow.Spec.ServiceAccountName != binding.ServiceAccountName || len(workflow.Spec.Templates) != 7 {
		t.Fatalf("unexpected workflow: %+v", workflow.Spec)
	}
	templates := map[string]argoTemplate{}
	for _, template := range workflow.Spec.Templates {
		templates[template.Name] = template
	}
	if templates["chunk-1"].NodeSelector["kubernetes.io/hostname"] != "private-cluster-a" ||
		templates["chunk-2"].NodeSelector["kubernetes.io/hostname"] != "private-cluster-b" {
		t.Fatalf("shard placement was not lowered exactly: %+v %+v",
			templates["chunk-1"].NodeSelector, templates["chunk-2"].NodeSelector)
	}
	if templates["chunk-1"].Container.Resources == nil ||
		templates["chunk-1"].Container.Resources.Requests["cpu"] != "2" {
		t.Fatalf("cluster capacity request was not lowered: %+v", templates["chunk-1"].Container.Resources)
	}
	if templates["train-create"].Resource == nil ||
		!strings.Contains(templates["train-create"].Resource.Manifest, "private-native-a") ||
		!strings.Contains(templates["train-create"].Resource.Manifest, "private-host-a") ||
		!strings.Contains(templates["train-create"].Resource.Manifest, "nvidia.com/gpu") {
		t.Fatalf("native Job indirection is incomplete: %+v", templates["train-create"])
	}
	collector := templates["train-collect"].Container
	if collector == nil || len(collector.Args) != 1 ||
		!strings.Contains(collector.Args[0], "verify-native-result") ||
		!strings.Contains(collector.Args[0], "--commit-output") {
		t.Fatalf("v2 native collector is incomplete: %+v", collector)
	}
	var reduce argoDAGTask
	for _, task := range templates["pipeline"].DAG.Tasks {
		if task.Name == "reduce" {
			reduce = task
		}
	}
	if !reflect.DeepEqual(reduce.Dependencies, []string{"chunk-1", "chunk-2"}) {
		t.Fatalf("reducer does not fan in every chunk: %+v", reduce)
	}
}

func TestLowerMixedDKSFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Pipeline, *DKSPlacementPlan, *MixedArgoBinding, *DKSWorkerResolverBinding)
		want string
	}{
		{
			name: "multi-host gang",
			edit: func(_ *Pipeline, plan *DKSPlacementPlan, _ *MixedArgoBinding, _ *DKSWorkerResolverBinding) {
				plan.Cohorts[0].Workers = []string{"native-a", "cluster-a"}
			},
			want: "gang scheduling",
		},
		{
			name: "resolver platform mismatch",
			edit: func(_ *Pipeline, _ *DKSPlacementPlan, _ *MixedArgoBinding, resolver *DKSWorkerResolverBinding) {
				resolver.Workers[2].OS = "darwin"
			},
			want: "want vk-native/linux/amd64",
		},
		{
			name: "missing native input",
			edit: func(_ *Pipeline, _ *DKSPlacementPlan, binding *MixedArgoBinding, _ *DKSWorkerResolverBinding) {
				binding.Native[0].Inputs = nil
			},
			want: "input URL coverage",
		},
		{
			name: "incomplete shard placement",
			edit: func(_ *Pipeline, plan *DKSPlacementPlan, _ *MixedArgoBinding, _ *DKSWorkerResolverBinding) {
				plan.Reductions[0].Chunks = plan.Reductions[0].Chunks[:1]
			},
			want: "required chunk indexes",
		},
		{
			name: "native retry",
			edit: func(_ *Pipeline, _ *DKSPlacementPlan, binding *MixedArgoBinding, _ *DKSWorkerResolverBinding) {
				retry := 1
				for i := range binding.Execution.Tasks {
					if binding.Execution.Tasks[i].ID == "train" {
						binding.Execution.Tasks[i].RetryLimit = &retry
					}
				}
			},
			want: "retries must be zero",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pipeline, plan, binding, resolver := mixedArgoFixture(t)
			test.edit(&pipeline, &plan, &binding, &resolver)
			if _, err := LowerMixedDKS(pipeline, plan, binding, resolver); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

func TestLowerMixedDKSUmbrellaNanochatSmoke(t *testing.T) {
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
	now := time.Date(2026, 7, 30, 18, 0, 0, 0, time.UTC)
	facts := []dag.HostFacts{
		{SchemaVersion: 1, Worker: "cluster-arm", OS: "linux", Arch: "arm64", CPU: 8, MemBytes: 16 << 30,
			Venues: []string{dag.VenueCluster}, ObservedAt: now},
		{SchemaVersion: 1, Worker: "cluster-amd", OS: "linux", Arch: "amd64", CPU: 8, MemBytes: 16 << 30,
			Venues: []string{dag.VenueCluster}, ObservedAt: now},
		{SchemaVersion: 1, Worker: "native-mac", OS: "darwin", Arch: "arm64", CPU: 16, MemBytes: 64 << 30,
			Venues:   []string{dag.VenueNative},
			Topology: dag.TopologyFacts{Class: "kubernetes.io/hostname", Domain: "native-domain"}, ObservedAt: now},
	}
	plan, err := PlanDKSPlacement(pipeline, facts, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	timeout, retries := 300, 0
	binding := MixedArgoBinding{
		Schema: MixedArgoBindingSchema,
		Execution: ArgoBinding{
			Schema:    ArgoBindingSchemaV2,
			Workspace: ArgoWorkspace{ClaimName: "nanochat-workspace", MountPath: "/workspace"},
		},
		ServiceAccountName:   "argo-workflow",
		ResultValidatorImage: "registry.example/bashy@sha256:" + strings.Repeat("d", 64),
	}
	matrix := map[string]MatrixEntry{}
	for _, entry := range pipeline.Matrix {
		matrix[entry.Task] = entry
	}
	for _, task := range pipeline.Tasks {
		taskBinding := ArgoTaskBinding{
			ID: task.ID, Image: "registry.example/nanochat@sha256:" + strings.Repeat("e", 64),
			RunnerPath: "/usr/local/bin/bashy", EvidenceDirectory: "evidence/" + task.ID,
			CommitManifestPath: "commits/" + task.ID + ".json", NonzeroClass: ResultInfraFail,
			TimeoutSeconds: &timeout, RetryLimit: &retries,
		}
		seen := map[string]bool{}
		for _, artifact := range append(append([]Artifact{}, matrix[task.ID].Inputs...), matrix[task.ID].Outputs...) {
			if seen[artifact.Name] {
				continue
			}
			seen[artifact.Name] = true
			taskBinding.Artifacts = append(taskBinding.Artifacts, ArgoArtifactBinding{
				Name: artifact.Name, Path: "artifacts/" + artifact.Name + "/" + artifact.SHA256,
			})
		}
		binding.Execution.Tasks = append(binding.Execution.Tasks, taskBinding)
		if task.Lane == LaneNative {
			native := NativeTaskBinding{
				Task: task.ID, ExecutableURL: "https://artifacts.example/nanochat-stage-runner",
				ExecutableSHA256: strings.Repeat("f", 64), ExecutablePath: "bin/nanochat-stage-runner",
				TaintKey: "virtual-kubelet.io/provider", ActiveDeadlineSeconds: timeout,
			}
			for _, input := range matrix[task.ID].Inputs {
				native.Inputs = append(native.Inputs, NativeInputBinding{
					Name: input.Name, URL: "https://artifacts.example/" + input.SHA256,
				})
			}
			binding.Native = append(binding.Native, native)
		}
	}
	resolver := DKSWorkerResolverBinding{Schema: DKSWorkerResolverSchema, Workers: []DKSWorkerResolution{
		{Worker: "cluster-arm", Node: "private-cluster-arm", Backend: "k3s", OS: "linux", Arch: "arm64"},
		{Worker: "cluster-amd", Node: "private-cluster-amd", Backend: "k3s", OS: "linux", Arch: "amd64"},
		{Worker: "native-mac", Node: "private-native-mac", Backend: "vk-native", OutpostHost: "private-mac",
			OS: "darwin", Arch: "arm64", TopologyClass: "kubernetes.io/hostname", TopologyDomain: "native-domain"},
	}}
	output, err := LowerMixedDKS(pipeline, plan, binding, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(output), "action: create") != 5 ||
		!strings.Contains(string(output), "name: dataset-reduce") ||
		!strings.Contains(string(output), "name: base-eval-reduce") {
		t.Fatalf("nanochat workflow is incomplete:\n%s", output)
	}
}
