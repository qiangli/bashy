package dhnt

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	dag "github.com/qiangli/coreutils/pkg/dag"
)

func placementPipeline() Pipeline {
	digest := strings.Repeat("a", 64)
	artifact := func(name string) Artifact {
		return Artifact{Name: name, Kind: ArtifactFile, DigestAlgorithm: DigestSHA256FileV1, SHA256: digest}
	}
	placement := &PlacementRequirement{CPU: 2, MinimumCapacity: map[string]uint64{"dhnt.io/vram": 8 << 30}}
	return Pipeline{
		Schema: PipelineSchemaV2, Pipeline: "placement-smoke",
		Source: Source{Repository: "https://example.invalid/repo.git", Commit: "abc", SHA256: digest},
		Tasks: []Task{
			{ID: "chunk-1", Lane: LaneNative, Distribution: DistributionShardable, Needs: []string{}, Argv: []string{"work", "1"}, WorkingDirectory: ".", Environment: []Environment{}, Placement: placement, Reducer: "reduce"},
			{ID: "chunk-2", Lane: LaneNative, Distribution: DistributionShardable, Needs: []string{}, Argv: []string{"work", "2"}, WorkingDirectory: ".", Environment: []Environment{}, Placement: placement, Reducer: "reduce"},
			{ID: "reduce", Lane: LaneCluster, Distribution: DistributionSingle, Needs: []string{"chunk-1", "chunk-2"}, Argv: []string{"reduce"}, WorkingDirectory: ".", Environment: []Environment{}},
			{ID: "train", Lane: LaneNative, Distribution: DistributionTopologyCoupled, Needs: []string{"reduce"}, Argv: []string{"train"}, WorkingDirectory: ".", Environment: []Environment{},
				Placement: &PlacementRequirement{AcceleratorKind: "cuda", AcceleratorCount: 1, AcceleratorMemoryBytes: 8 << 30, TopologyKey: "kubernetes.io/hostname", CohortSize: 2}},
		},
		Matrix: []MatrixEntry{
			{Task: "chunk-1", Platform: Platform{Backend: "vk-native", OS: "linux", Arch: "amd64"}, Chunk: &Chunk{Index: 1, Count: 2, ManifestSHA256: digest}, Inputs: []Artifact{artifact("input")}, Outputs: []Artifact{artifact("one")}},
			{Task: "chunk-2", Platform: Platform{Backend: "vk-native", OS: "linux", Arch: "amd64"}, Chunk: &Chunk{Index: 2, Count: 2, ManifestSHA256: digest}, Inputs: []Artifact{artifact("input")}, Outputs: []Artifact{artifact("two")}},
			{Task: "reduce", Platform: Platform{Backend: "k3s", OS: "linux", Arch: "amd64"}, Inputs: []Artifact{artifact("one"), artifact("two")}, Outputs: []Artifact{artifact("reduced")}},
			{Task: "train", Platform: Platform{Backend: "vk-native", OS: "linux", Arch: "amd64"}, Inputs: []Artifact{artifact("reduced")}, Outputs: []Artifact{artifact("model")}},
		},
	}
}

func placementFacts(now time.Time, mixed bool) []dag.HostFacts {
	family := "rtx-3070"
	secondFamily := family
	if mixed {
		secondFamily = "rtx-4090"
	}
	makeFact := func(worker, family string) dag.HostFacts {
		return dag.HostFacts{
			SchemaVersion: 1, Worker: worker, OS: "linux", Arch: "amd64", CPU: 12,
			MemBytes: 32 << 30, Venues: []string{dag.VenueNative},
			Accelerators: []dag.AcceleratorFacts{{Kind: "cuda", Family: family, Count: 1, MemoryBytes: 8 << 30}},
			Capacities:   map[string]uint64{"dhnt.io/vram": 8 << 30},
			Topology:     dag.TopologyFacts{Class: "kubernetes.io/hostname", Domain: "owned-fabric"},
			ObservedAt:   now,
		}
	}
	return []dag.HostFacts{makeFact("worker-b", secondFamily), makeFact("worker-a", family)}
}

func TestV2ReduceCoverageFailsClosed(t *testing.T) {
	pipeline := placementPipeline()
	pipeline.Tasks[2].Needs = []string{"chunk-1"}
	if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), "does not depend") {
		t.Fatalf("incomplete reducer accepted: %v", err)
	}
	pipeline = placementPipeline()
	pipeline.Matrix[1].Chunk.Index = 1
	if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate chunk") {
		t.Fatalf("duplicate/incomplete chunk set accepted: %v", err)
	}
}

func TestPlanDKSPlacementRejectsHeterogeneousGangAndIsDeterministic(t *testing.T) {
	now := time.Now().UTC()
	pipeline := placementPipeline()
	if _, err := PlanDKSPlacement(pipeline, placementFacts(now, true), now, time.Minute); err == nil {
		t.Fatal("heterogeneous accelerator gang was accepted")
	}
	first, err := PlanDKSPlacement(pipeline, placementFacts(now, false), now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	facts := placementFacts(now, false)
	facts[0], facts[1] = facts[1], facts[0]
	pipeline.Tasks[0], pipeline.Tasks[3] = pipeline.Tasks[3], pipeline.Tasks[0]
	pipeline.Matrix[0], pipeline.Matrix[3] = pipeline.Matrix[3], pipeline.Matrix[0]
	second, err := PlanDKSPlacement(pipeline, facts, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("placement depended on inventory order:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if len(first.Reductions) != 1 || len(first.Reductions[0].Chunks) != 2 ||
		!reflect.DeepEqual(first.Cohorts[0].Workers, []string{"worker-a", "worker-b"}) {
		t.Fatalf("incomplete placement: %+v", first)
	}
	if !reflect.DeepEqual(
		[]string{first.Reductions[0].Chunks[0].Worker, first.Reductions[0].Chunks[1].Worker},
		[]string{"worker-a", "worker-b"},
	) {
		t.Fatalf("homogeneous chunks were not spread across eligible workers: %+v", first.Reductions[0].Chunks)
	}
	if first.Reductions[0].ManifestSHA256 != strings.Repeat("a", 64) ||
		first.Reductions[0].MembershipSHA256 == first.Reductions[0].ManifestSHA256 {
		t.Fatalf("pinned manifest and derived membership identities were conflated: %+v", first.Reductions[0])
	}
}

func TestPlanDKSPlacementMarshalsEmptyCollectionsForTopologyOnlyPlan(t *testing.T) {
	now := time.Now().UTC()
	pipeline := placementPipeline()
	pipeline.Tasks = []Task{pipeline.Tasks[3]}
	pipeline.Tasks[0].Needs = []string{}
	pipeline.Matrix = []MatrixEntry{pipeline.Matrix[3]}
	plan, err := PlanDKSPlacement(
		pipeline, placementFacts(now, false), now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalDKSPlacementPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"reductions":null`) ||
		!strings.Contains(string(encoded), `"reductions":[]`) {
		t.Fatalf("topology-only placement emitted a null collection: %s", encoded)
	}
	if _, err := DecodeDKSPlacementPlan(encoded); err != nil {
		t.Fatalf("planner output was rejected by its strict decoder: %v", err)
	}
}

func TestPlanDKSPlacementRoundTripsEmbeddedReductionFields(t *testing.T) {
	now := time.Now().UTC()
	plan, err := PlanDKSPlacement(
		placementPipeline(), placementFacts(now, false), now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Reductions) == 0 {
		t.Fatal("fixture did not produce a reduction")
	}
	encoded, err := MarshalDKSPlacementPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeDKSPlacementPlan(encoded)
	if err != nil {
		t.Fatalf("strict decoder rejected flattened embedded reduction fields: %v", err)
	}
	if !reflect.DeepEqual(decoded, plan) {
		t.Fatalf("placement round trip changed the plan:\nwant: %#v\ngot:  %#v", plan, decoded)
	}
}

func TestPlanDKSPlacementRejectsAmbiguousMatrixAndDuplicateWorkers(t *testing.T) {
	now := time.Now().UTC()
	pipeline := placementPipeline()
	duplicate := pipeline.Matrix[3]
	duplicate.Platform.Arch = "arm64"
	pipeline.Matrix = append(pipeline.Matrix, duplicate)
	if _, err := PlanDKSPlacement(pipeline, placementFacts(now, false), now, time.Minute); err == nil ||
		!strings.Contains(err.Error(), "multiple matrix rows") {
		t.Fatalf("ambiguous placed matrix accepted: %v", err)
	}

	pipeline = placementPipeline()
	facts := placementFacts(now, false)
	facts[1].Worker = facts[0].Worker
	if _, err := PlanDKSPlacement(pipeline, facts, now, time.Minute); err == nil ||
		!strings.Contains(err.Error(), "duplicate worker") {
		t.Fatalf("duplicate logical worker accepted: %v", err)
	}
}

func TestPlanDKSPlacementHonorsPerShardPlatform(t *testing.T) {
	now := time.Now().UTC()
	pipeline := placementPipeline()
	pipeline.Matrix[1].Platform.Arch = "arm64"
	pipeline.Tasks[3].Placement.CohortSize = 1
	facts := placementFacts(now, false)
	facts[0].Arch = "arm64"
	plan, err := PlanDKSPlacement(pipeline, facts, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Reductions[0].Chunks
	if len(got) != 2 || got[0].Worker != "worker-a" || got[1].Worker != "worker-b" {
		t.Fatalf("per-shard os/arch constraints were not preserved: %+v", got)
	}
}

func TestDecodeDKSHostFactsIsVersionedAndStrict(t *testing.T) {
	now := time.Now().UTC()
	inventory := DKSHostFactsInventory{Schema: DKSHostFactsSchema, Facts: placementFacts(now, false)}
	data, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeDKSHostFacts(data); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["reach"] = "must-not-pass"
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeDKSHostFacts(data); err == nil {
		t.Fatal("unknown inventory field accepted")
	}
}
