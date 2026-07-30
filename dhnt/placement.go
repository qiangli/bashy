package dhnt

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	dag "github.com/qiangli/coreutils/pkg/dag"
)

const (
	DKSPlacementPlanSchema = "dhnt.dks-placement/v1"
	DKSHostFactsSchema     = "dhnt.host-facts/v1"
)

// DKSHostFactsInventory is the versioned, observed input to placement
// planning. It contains capability-only facts: transport/reach details remain
// outside the compiler contract.
type DKSHostFactsInventory struct {
	Schema string          `json:"schema"`
	Facts  []dag.HostFacts `json:"facts"`
}

// DKSReductionPlan keeps the pipeline's pinned chunk-manifest digest distinct
// from the planner's derived membership digest.
type DKSReductionPlan struct {
	dag.ChunkReducePlan
	ManifestSHA256 string `json:"manifest_sha256"`
}

type DKSPlacementPlan struct {
	Schema     string             `json:"schema"`
	Pipeline   string             `json:"pipeline"`
	Cohorts    []dag.CohortPlan   `json:"cohorts"`
	Reductions []DKSReductionPlan `json:"reductions"`
}

// PlanDKSPlacement is the generic compiler seam between the portable v2
// pipeline and a concrete DKS/Argo lowerer. It resolves only observed
// capability/topology and stable chunk placement; native result collection is
// deliberately a separate contract.
func PlanDKSPlacement(p Pipeline, facts []dag.HostFacts, now time.Time, maxAge time.Duration) (DKSPlacementPlan, error) {
	if err := p.Validate(); err != nil {
		return DKSPlacementPlan{}, fmt.Errorf("pipeline: %w", err)
	}
	if p.Schema != PipelineSchemaV2 {
		return DKSPlacementPlan{}, fmt.Errorf("placement planning requires %s", PipelineSchemaV2)
	}
	p = canonicalPipeline(p)
	if now.IsZero() || maxAge <= 0 {
		return DKSPlacementPlan{}, fmt.Errorf("placement planning requires a current time and positive maximum fact age")
	}
	workers := make(map[string]bool, len(facts))
	for i := range facts {
		if err := facts[i].Validate(); err != nil {
			return DKSPlacementPlan{}, fmt.Errorf("facts[%d]: %w", i, err)
		}
		if workers[facts[i].Worker] {
			return DKSPlacementPlan{}, fmt.Errorf("facts[%d]: duplicate worker %q", i, facts[i].Worker)
		}
		workers[facts[i].Worker] = true
	}
	matrix := make(map[string]MatrixEntry, len(p.Matrix))
	tasks := make(map[string]Task, len(p.Tasks))
	for _, task := range p.Tasks {
		tasks[task.ID] = task
	}
	for _, entry := range p.Matrix {
		if _, exists := matrix[entry.Task]; exists {
			if tasks[entry.Task].Placement != nil {
				return DKSPlacementPlan{}, fmt.Errorf(
					"task %q has multiple matrix rows; placement requires one already-expanded row", entry.Task)
			}
		}
		matrix[entry.Task] = entry
	}
	plan := DKSPlacementPlan{
		Schema:     DKSPlacementPlanSchema,
		Pipeline:   p.Pipeline,
		Cohorts:    []dag.CohortPlan{},
		Reductions: []DKSReductionPlan{},
	}
	plannedReducers := map[string]bool{}
	for _, task := range p.Tasks {
		if task.Placement == nil {
			continue
		}
		entry := matrix[task.ID]
		spec := placementTaskSpec(task, entry)
		switch task.Distribution {
		case DistributionShardable:
			if plannedReducers[task.Reducer] {
				continue
			}
			fullManifest := &dag.ChunkManifest{SchemaVersion: 1, Suite: task.Reducer}
			type shardGroup struct {
				spec   dag.TaskSpec
				chunks []dag.Chunk
			}
			groups := map[string]*shardGroup{}
			var groupKeys []string
			for _, sibling := range p.Tasks {
				if sibling.Reducer != task.Reducer {
					continue
				}
				chunk := matrix[sibling.ID].Chunk
				member := dag.Chunk{
					ID: chunk.Index, Fixtures: []dag.Fixture{{Name: sibling.ID}},
				}
				fullManifest.ChunkCount = chunk.Count
				fullManifest.Chunks = append(fullManifest.Chunks, member)
				siblingSpec := placementTaskSpec(sibling, matrix[sibling.ID])
				identitySpec := siblingSpec
				identitySpec.Task = ""
				identitySpec.Reducer = ""
				keyData, err := json.Marshal(identitySpec)
				if err != nil {
					return DKSPlacementPlan{}, fmt.Errorf("task %q placement identity: %w", sibling.ID, err)
				}
				key := string(keyData)
				group := groups[key]
				if group == nil {
					group = &shardGroup{spec: siblingSpec}
					groups[key] = group
					groupKeys = append(groupKeys, key)
				}
				group.chunks = append(group.chunks, member)
			}
			sort.Strings(groupKeys)
			reduction := dag.ChunkReducePlan{
				Task: task.ID, Reducer: task.Reducer,
				MembershipSHA256: strings.TrimPrefix(fullManifest.MembershipHash(), "m"),
			}
			for _, key := range groupKeys {
				group := groups[key]
				manifest := &dag.ChunkManifest{
					SchemaVersion: 1, Suite: task.Reducer,
					ChunkCount: len(group.chunks), Chunks: group.chunks,
				}
				partial, err := dag.PlanChunkReduce(group.spec, manifest, facts, now, maxAge)
				if err != nil {
					return DKSPlacementPlan{}, fmt.Errorf("task %q: %w", group.spec.Task, err)
				}
				reduction.Chunks = append(reduction.Chunks, partial.Chunks...)
				reduction.RequiredChunkIndex = append(reduction.RequiredChunkIndex, partial.RequiredChunkIndex...)
			}
			sort.Slice(reduction.Chunks, func(i, j int) bool {
				return reduction.Chunks[i].Index < reduction.Chunks[j].Index
			})
			sort.Ints(reduction.RequiredChunkIndex)
			plan.Reductions = append(plan.Reductions, DKSReductionPlan{
				ChunkReducePlan: reduction,
				ManifestSHA256:  matrix[task.ID].Chunk.ManifestSHA256,
			})
			plannedReducers[task.Reducer] = true
		case DistributionTopologyCoupled, DistributionSingle, DistributionReplicated:
			cohort, err := dag.PlanCohort(spec, facts, now, maxAge)
			if err != nil {
				return DKSPlacementPlan{}, fmt.Errorf("task %q: %w", task.ID, err)
			}
			plan.Cohorts = append(plan.Cohorts, cohort)
		}
	}
	sort.Slice(plan.Cohorts, func(i, j int) bool { return plan.Cohorts[i].Task < plan.Cohorts[j].Task })
	sort.Slice(plan.Reductions, func(i, j int) bool { return plan.Reductions[i].Reducer < plan.Reductions[j].Reducer })
	return plan, nil
}

func DecodeDKSHostFacts(data []byte) (DKSHostFactsInventory, error) {
	var inventory DKSHostFactsInventory
	if err := decodeStrict(data, &inventory); err != nil {
		return inventory, err
	}
	if inventory.Schema != DKSHostFactsSchema {
		return inventory, fmt.Errorf("schema: got %q, want %q", inventory.Schema, DKSHostFactsSchema)
	}
	if len(inventory.Facts) == 0 {
		return inventory, fmt.Errorf("facts: must not be empty")
	}
	for i := range inventory.Facts {
		if err := inventory.Facts[i].Validate(); err != nil {
			return inventory, fmt.Errorf("facts[%d]: %w", i, err)
		}
	}
	return inventory, nil
}

func DecodeDKSPlacementPlan(data []byte) (DKSPlacementPlan, error) {
	var plan DKSPlacementPlan
	if err := decodeStrict(data, &plan); err != nil {
		return plan, err
	}
	if plan.Schema != DKSPlacementPlanSchema {
		return plan, fmt.Errorf("schema: got %q, want %q", plan.Schema, DKSPlacementPlanSchema)
	}
	if err := validateID("pipeline", plan.Pipeline); err != nil {
		return plan, err
	}
	return plan, nil
}

func MarshalDKSPlacementPlan(plan DKSPlacementPlan) ([]byte, error) {
	if plan.Schema != DKSPlacementPlanSchema {
		return nil, fmt.Errorf("schema: got %q, want %q", plan.Schema, DKSPlacementPlanSchema)
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func placementTaskSpec(task Task, entry MatrixEntry) dag.TaskSpec {
	p := task.Placement
	spec := dag.TaskSpec{
		SchemaVersion: dag.TaskSpecSchemaVersion,
		Task:          task.ID, Venue: string(task.Lane), Distribution: dag.Distribution(task.Distribution),
		Reducer: task.Reducer,
		Match:   map[string]string{"os": entry.Platform.OS, "arch": entry.Platform.Arch},
	}
	if p != nil {
		spec.CPUPerTask = p.CPU
		spec.MemPerTask = p.MemoryBytes
		spec.Accelerator = dag.AcceleratorRequest{
			Kind: p.AcceleratorKind, Family: p.AcceleratorFamily,
			Count: p.AcceleratorCount, MemoryBytes: p.AcceleratorMemoryBytes,
		}
		spec.MinimumCapacity = p.MinimumCapacity
		spec.TopologyClass = p.TopologyKey
		spec.CohortSize = p.CohortSize
	}
	return spec
}
