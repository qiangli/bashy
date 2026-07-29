// Package dhnt defines the portable, Argo-independent dhnt.pipeline/v1 and
// dhnt.run/v1 data contracts used by Bashy workers and evidence gates.
package dhnt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	PipelineSchema = "dhnt.pipeline/v1"
	RunSchema      = "dhnt.run/v1"
)

type Lane string

const (
	LaneNative    Lane = "native"
	LaneContainer Lane = "container"
	LaneCluster   Lane = "cluster"
	LaneCloud     Lane = "cloud"
)

type ResultClass string

const (
	ResultPass       ResultClass = "pass"
	ResultTestFail   ResultClass = "test-fail"
	ResultInfraFail  ResultClass = "infra-fail"
	ResultIncomplete ResultClass = "incomplete"
	ResultCanceled   ResultClass = "canceled"
)

type Source struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	SHA256     string `json:"sha256"`
}

type Artifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type Environment struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Platform struct {
	Backend string `json:"backend"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

type Task struct {
	ID               string        `json:"id"`
	Lane             Lane          `json:"lane"`
	Needs            []string      `json:"needs"`
	Argv             []string      `json:"argv"`
	WorkingDirectory string        `json:"workingDirectory"`
	Environment      []Environment `json:"environment"`
}

// MatrixEntry declares one required task/platform result and the exact
// content-addressed inputs and outputs that result must attest.
type MatrixEntry struct {
	Task     string     `json:"task"`
	Platform Platform   `json:"platform"`
	Inputs   []Artifact `json:"inputs"`
	Outputs  []Artifact `json:"outputs"`
}

type Pipeline struct {
	Schema   string        `json:"schema"`
	Pipeline string        `json:"pipeline"`
	Source   Source        `json:"source"`
	Tasks    []Task        `json:"tasks"`
	Matrix   []MatrixEntry `json:"matrix"`
}

type Executor struct {
	Node    string `json:"node"`
	Backend string `json:"backend"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

type Result struct {
	Class    ResultClass `json:"class"`
	ExitCode int         `json:"exitCode"`
}

type Run struct {
	Schema     string     `json:"schema"`
	Pipeline   string     `json:"pipeline"`
	Task       string     `json:"task"`
	Run        string     `json:"run"`
	Source     Source     `json:"source"`
	Inputs     []Artifact `json:"inputs"`
	Executor   Executor   `json:"executor"`
	Result     Result     `json:"result"`
	Outputs    []Artifact `json:"outputs"`
	StartedAt  string     `json:"startedAt"`
	FinishedAt string     `json:"finishedAt"`
	TraceID    string     `json:"traceId"`
}

var (
	identifierRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
	envNameRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	sha256RE     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	traceIDRE    = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

func (p Pipeline) Validate() error {
	if p.Schema != PipelineSchema {
		return fmt.Errorf("schema: got %q, want %q", p.Schema, PipelineSchema)
	}
	if err := validateID("pipeline", p.Pipeline); err != nil {
		return err
	}
	if err := validateSource(p.Source); err != nil {
		return err
	}
	if len(p.Tasks) == 0 {
		return errors.New("tasks: must not be empty")
	}
	tasks := make(map[string]Task, len(p.Tasks))
	for i, task := range p.Tasks {
		if err := validateTask(task); err != nil {
			return fmt.Errorf("tasks[%d]: %w", i, err)
		}
		if _, exists := tasks[task.ID]; exists {
			return fmt.Errorf("tasks[%d]: duplicate task %q", i, task.ID)
		}
		tasks[task.ID] = task
	}
	for _, task := range p.Tasks {
		for _, need := range task.Needs {
			if _, exists := tasks[need]; !exists {
				return fmt.Errorf("task %q: unknown dependency %q", task.ID, need)
			}
			if need == task.ID {
				return fmt.Errorf("task %q: depends on itself", task.ID)
			}
		}
	}
	if err := validateAcyclic(tasks); err != nil {
		return err
	}
	if len(p.Matrix) == 0 {
		return errors.New("matrix: must declare at least one platform")
	}
	covered := make(map[string]bool, len(tasks))
	matrixKeys := make(map[string]bool, len(p.Matrix))
	for i, entry := range p.Matrix {
		task, exists := tasks[entry.Task]
		if !exists {
			return fmt.Errorf("matrix[%d]: unknown task %q", i, entry.Task)
		}
		if err := validatePlatform(entry.Platform); err != nil {
			return fmt.Errorf("matrix[%d]: %w", i, err)
		}
		if task.Lane == LaneNative && entry.Platform.Backend != "vk-native" {
			return fmt.Errorf("matrix[%d]: native task %q requires backend vk-native", i, entry.Task)
		}
		if err := validateArtifacts("inputs", entry.Inputs, true); err != nil {
			return fmt.Errorf("matrix[%d]: %w", i, err)
		}
		if err := validateArtifacts("outputs", entry.Outputs, true); err != nil {
			return fmt.Errorf("matrix[%d]: %w", i, err)
		}
		key := matrixKey(entry.Task, entry.Platform)
		if matrixKeys[key] {
			return fmt.Errorf("matrix[%d]: duplicate task/platform %q", i, key)
		}
		matrixKeys[key] = true
		covered[entry.Task] = true
	}
	for id := range tasks {
		if !covered[id] {
			return fmt.Errorf("matrix: task %q has no declared platform", id)
		}
	}
	return nil
}

func (r Run) Validate() error {
	if r.Schema != RunSchema {
		return fmt.Errorf("schema: got %q, want %q", r.Schema, RunSchema)
	}
	if err := validateID("pipeline", r.Pipeline); err != nil {
		return err
	}
	if err := validateID("task", r.Task); err != nil {
		return err
	}
	if err := validateID("run", r.Run); err != nil {
		return err
	}
	if err := validateSource(r.Source); err != nil {
		return err
	}
	if err := validateArtifacts("inputs", r.Inputs, true); err != nil {
		return err
	}
	if err := validateArtifacts("outputs", r.Outputs, true); err != nil {
		return err
	}
	if strings.TrimSpace(r.Executor.Node) == "" {
		return errors.New("executor.node: must not be empty")
	}
	if err := validatePlatform(Platform{
		Backend: r.Executor.Backend,
		OS:      r.Executor.OS,
		Arch:    r.Executor.Arch,
	}); err != nil {
		return fmt.Errorf("executor: %w", err)
	}
	switch r.Result.Class {
	case ResultPass, ResultTestFail, ResultInfraFail, ResultIncomplete, ResultCanceled:
	default:
		return fmt.Errorf("result.class: unknown value %q", r.Result.Class)
	}
	if r.Result.Class == ResultPass && r.Result.ExitCode != 0 {
		return errors.New("result.exitCode: pass requires exit code 0")
	}
	if r.Result.Class == ResultTestFail && r.Result.ExitCode == 0 {
		return errors.New("result.exitCode: test-fail requires a non-zero exit code")
	}
	start, err := validateTimestamp("startedAt", r.StartedAt)
	if err != nil {
		return err
	}
	finish, err := validateTimestamp("finishedAt", r.FinishedAt)
	if err != nil {
		return err
	}
	if finish.Before(start) {
		return errors.New("finishedAt: must not be before startedAt")
	}
	if !traceIDRE.MatchString(r.TraceID) || r.TraceID == strings.Repeat("0", 32) {
		return errors.New("traceId: must be 32 lowercase hex characters and not all zero")
	}
	return nil
}

func DecodePipeline(data []byte) (Pipeline, error) {
	var p Pipeline
	if err := decodeStrict(data, &p); err != nil {
		return p, err
	}
	if err := p.Validate(); err != nil {
		return p, err
	}
	return p, nil
}

func DecodeRun(data []byte) (Run, error) {
	var r Run
	if err := decodeStrict(data, &r); err != nil {
		return r, err
	}
	if err := r.Validate(); err != nil {
		return r, err
	}
	return r, nil
}

func MarshalPipeline(p Pipeline) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	p = canonicalPipeline(p)
	return marshalLine(p)
}

func MarshalRun(r Run) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	r = canonicalRun(r)
	return marshalLine(r)
}

func decodeStrict(data []byte, dst any) error {
	if err := rejectDuplicateJSONFields(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("malformed JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("malformed JSON: multiple values")
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	return nil
}

func rejectDuplicateJSONFields(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var checkValue func() error
	checkValue = func() error {
		token, err := dec.Token()
		if err != nil {
			return fmt.Errorf("malformed JSON: %w", err)
		}
		delim, composite := token.(json.Delim)
		if !composite {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				token, err := dec.Token()
				if err != nil {
					return fmt.Errorf("malformed JSON: %w", err)
				}
				key, ok := token.(string)
				if !ok {
					return errors.New("malformed JSON: object key is not a string")
				}
				if seen[key] {
					return fmt.Errorf("malformed JSON: duplicate field %q", key)
				}
				seen[key] = true
				if err := checkValue(); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil {
				return fmt.Errorf("malformed JSON: %w", err)
			}
		case '[':
			for dec.More() {
				if err := checkValue(); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil {
				return fmt.Errorf("malformed JSON: %w", err)
			}
		default:
			return errors.New("malformed JSON: unexpected delimiter")
		}
		return nil
	}
	if err := checkValue(); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("malformed JSON: multiple values")
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	return nil
}

func marshalLine(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func canonicalPipeline(p Pipeline) Pipeline {
	p.Tasks = append([]Task(nil), p.Tasks...)
	for i := range p.Tasks {
		p.Tasks[i].Needs = sortedStrings(p.Tasks[i].Needs)
		p.Tasks[i].Argv = append([]string(nil), p.Tasks[i].Argv...)
		p.Tasks[i].Environment = append([]Environment{}, p.Tasks[i].Environment...)
		sort.Slice(p.Tasks[i].Environment, func(a, b int) bool {
			return p.Tasks[i].Environment[a].Name < p.Tasks[i].Environment[b].Name
		})
	}
	sort.Slice(p.Tasks, func(i, j int) bool { return p.Tasks[i].ID < p.Tasks[j].ID })
	p.Matrix = append([]MatrixEntry(nil), p.Matrix...)
	for i := range p.Matrix {
		p.Matrix[i].Inputs = sortedArtifacts(p.Matrix[i].Inputs)
		p.Matrix[i].Outputs = sortedArtifacts(p.Matrix[i].Outputs)
	}
	sort.Slice(p.Matrix, func(i, j int) bool {
		return matrixKey(p.Matrix[i].Task, p.Matrix[i].Platform) <
			matrixKey(p.Matrix[j].Task, p.Matrix[j].Platform)
	})
	return p
}

func canonicalRun(r Run) Run {
	r.Inputs = sortedArtifacts(r.Inputs)
	r.Outputs = sortedArtifacts(r.Outputs)
	return r
}

func sortedStrings(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func sortedArtifacts(in []Artifact) []Artifact {
	out := append([]Artifact(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func validateSource(s Source) error {
	if strings.TrimSpace(s.Repository) == "" {
		return errors.New("source.repository: must not be empty")
	}
	if strings.TrimSpace(s.Commit) == "" {
		return errors.New("source.commit: must not be empty")
	}
	return validateDigest("source.sha256", s.SHA256)
}

func validateTask(task Task) error {
	if err := validateID("id", task.ID); err != nil {
		return err
	}
	switch task.Lane {
	case LaneNative, LaneContainer, LaneCluster, LaneCloud:
	default:
		return fmt.Errorf("lane: unknown value %q", task.Lane)
	}
	if len(task.Argv) == 0 || task.Argv[0] == "" {
		return errors.New("argv: must contain a non-empty command")
	}
	if task.WorkingDirectory == "" {
		return errors.New("workingDirectory: must not be empty")
	}
	wd := strings.ReplaceAll(task.WorkingDirectory, "\\", "/")
	if strings.HasPrefix(wd, "/") {
		return errors.New("workingDirectory: must be a clean repository-relative path")
	}
	if len(wd) >= 2 && wd[1] == ':' {
		c := wd[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return errors.New("workingDirectory: must be a clean repository-relative path")
		}
	}
	if strings.HasPrefix(wd, "//") {
		return errors.New("workingDirectory: must be a clean repository-relative path")
	}
	cleaned := path.Clean(wd)
	if cleaned != wd || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return errors.New("workingDirectory: must be a clean repository-relative path")
	}
	if err := validateUniqueStrings("needs", task.Needs); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, item := range task.Environment {
		if !envNameRE.MatchString(item.Name) {
			return fmt.Errorf("environment[%d].name: invalid name %q", i, item.Name)
		}
		if seen[item.Name] {
			return fmt.Errorf("environment[%d].name: duplicate name %q", i, item.Name)
		}
		seen[item.Name] = true
	}
	return nil
}

func validatePlatform(p Platform) error {
	switch p.Backend {
	case "local", "vk-native", "vk-podman", "k3s", "cloud":
	default:
		return fmt.Errorf("platform.backend: unknown value %q", p.Backend)
	}
	switch p.OS {
	case "linux", "darwin", "windows":
	default:
		return fmt.Errorf("platform.os: unknown value %q", p.OS)
	}
	switch p.Arch {
	case "amd64", "arm64":
	default:
		return fmt.Errorf("platform.arch: unknown value %q", p.Arch)
	}
	return nil
}

func validateArtifacts(field string, artifacts []Artifact, required bool) error {
	if required && len(artifacts) == 0 {
		return fmt.Errorf("%s: must not be empty", field)
	}
	seen := map[string]bool{}
	for i, artifact := range artifacts {
		if err := validateID(fmt.Sprintf("%s[%d].name", field, i), artifact.Name); err != nil {
			return err
		}
		if seen[artifact.Name] {
			return fmt.Errorf("%s[%d].name: duplicate name %q", field, i, artifact.Name)
		}
		seen[artifact.Name] = true
		if err := validateDigest(fmt.Sprintf("%s[%d].sha256", field, i), artifact.SHA256); err != nil {
			return err
		}
	}
	return nil
}

func validateID(field, value string) error {
	if !identifierRE.MatchString(value) {
		return fmt.Errorf("%s: invalid identifier %q", field, value)
	}
	return nil
}

func validateDigest(field, value string) error {
	if !sha256RE.MatchString(value) {
		return fmt.Errorf("%s: must be lowercase 64-hex sha256", field)
	}
	return nil
}

func validateTimestamp(field, value string) (time.Time, error) {
	if !strings.HasSuffix(value, "Z") {
		return time.Time{}, fmt.Errorf("%s: must be UTC RFC3339/RFC3339Nano", field)
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: must be UTC RFC3339/RFC3339Nano: %w", field, err)
	}
	return t, nil
}

func validateUniqueStrings(field string, values []string) error {
	seen := map[string]bool{}
	for i, value := range values {
		if err := validateID(fmt.Sprintf("%s[%d]", field, i), value); err != nil {
			return err
		}
		if seen[value] {
			return fmt.Errorf("%s[%d]: duplicate value %q", field, i, value)
		}
		seen[value] = true
	}
	return nil
}

func validateAcyclic(tasks map[string]Task) error {
	state := map[string]uint8{}
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("tasks: dependency cycle at %q", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, need := range tasks[id].Needs {
			if err := visit(need); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range tasks {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func matrixKey(task string, p Platform) string {
	return task + "\x00" + p.Backend + "\x00" + p.OS + "\x00" + p.Arch
}
