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
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
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

// Distribution declares how a task may be expanded by an executor. It is a
// planning contract only: declaring a mode does not imply that a particular
// executor implements it.
type Distribution string

const (
	DistributionSingle          Distribution = "single"
	DistributionShardable       Distribution = "shardable"
	DistributionReplicated      Distribution = "replicated"
	DistributionTopologyCoupled Distribution = "topology-coupled"
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
	Distribution     Distribution  `json:"distribution"`
	Needs            []string      `json:"needs"`
	Argv             []string      `json:"argv"`
	WorkingDirectory string        `json:"workingDirectory"`
	Environment      []Environment `json:"environment"`
}

// Chunk pins one stable, one-based shard identity. ManifestSHA256 identifies
// the immutable manifest that defines membership; online fleet capacity may
// change concurrency but never Index, Count, or membership.
type Chunk struct {
	Index          int    `json:"index"`
	Count          int    `json:"count"`
	ManifestSHA256 string `json:"manifestSha256"`
}

// MatrixEntry declares one required task/platform result and the exact
// content-addressed inputs and outputs that result must attest.
type MatrixEntry struct {
	Task     string     `json:"task"`
	Platform Platform   `json:"platform"`
	Chunk    *Chunk     `json:"chunk,omitempty"`
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
	ExitCode *int        `json:"exitCode"`
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
	chunks := make(map[string]Chunk, len(tasks))
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
		switch task.Distribution {
		case DistributionShardable:
			if entry.Chunk == nil {
				return fmt.Errorf("matrix[%d]: shardable task %q requires chunk identity", i, entry.Task)
			}
			if err := validateChunk(*entry.Chunk); err != nil {
				return fmt.Errorf("matrix[%d]: chunk: %w", i, err)
			}
			if prior, ok := chunks[entry.Task]; ok && prior != *entry.Chunk {
				return fmt.Errorf("matrix[%d]: shardable task %q has inconsistent chunk identity", i, entry.Task)
			}
			chunks[entry.Task] = *entry.Chunk
		default:
			if entry.Chunk != nil {
				return fmt.Errorf("matrix[%d]: non-shardable task %q must not declare chunk identity", i, entry.Task)
			}
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
	if strings.ContainsRune(r.Executor.Node, 0) {
		return errors.New("executor.node: must not contain NUL byte")
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
	if r.Result.ExitCode == nil {
		return errors.New("result.exitCode: must be present")
	}
	if *r.Result.ExitCode < 0 {
		return errors.New("result.exitCode: must be non-negative (cannot be an observed process exit status)")
	}
	if r.Result.Class == ResultPass && *r.Result.ExitCode != 0 {
		return errors.New("result.exitCode: pass requires exit code 0")
	}
	if r.Result.Class == ResultTestFail && *r.Result.ExitCode == 0 {
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

func intPtr(i int) *int { return &i }

func decodeStrict(data []byte, dst any) error {
	if !utf8.Valid(data) {
		return errors.New("malformed JSON: invalid UTF-8")
	}
	if err := validateJSONSurrogates(data); err != nil {
		return err
	}
	if err := validateJSONKeys(data, dst); err != nil {
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

func validateJSONKeys(data []byte, dst any) error {
	t := reflect.TypeOf(dst)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := validateToken(dec, t); err != nil {
		return err
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

func validateJSONSurrogates(data []byte) error {
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		if i+1 >= len(data) {
			continue
		}
		switch data[i+1] {
		case '\\', '"', '/', 'b', 'f', 'n', 'r', 't':
			i++
			continue
		case 'u':
		default:
			continue
		}
		if i+5 >= len(data) {
			continue
		}
		r := parseHex4(data[i+2:])
		if r < 0xD800 || r > 0xDFFF {
			continue
		}
		if r <= 0xDBFF {
			if i+12 > len(data) || data[i+6] != '\\' || data[i+7] != 'u' {
				return fmt.Errorf("malformed JSON: unpaired UTF-16 surrogate \\u%04X", r)
			}
			next := parseHex4(data[i+8:])
			if next < 0xDC00 || next > 0xDFFF {
				return fmt.Errorf("malformed JSON: unpaired UTF-16 surrogate \\u%04X", r)
			}
			i += 11
			continue
		}
		if i >= 6 && data[i-6] == '\\' && data[i-5] == 'u' {
			slashCount := 0
			for j := i - 7; j >= 0 && data[j] == '\\'; j-- {
				slashCount++
			}
			if slashCount%2 == 0 {
				prev := parseHex4(data[i-4:])
				if prev >= 0xD800 && prev <= 0xDBFF {
					continue
				}
			}
		}
		return fmt.Errorf("malformed JSON: unpaired UTF-16 surrogate \\u%04X", r)
	}
	return nil
}

func parseHex4(b []byte) rune {
	var r rune
	for i := 0; i < 4; i++ {
		r <<= 4
		switch {
		case b[i] >= '0' && b[i] <= '9':
			r |= rune(b[i] - '0')
		case b[i] >= 'a' && b[i] <= 'f':
			r |= rune(b[i] - 'a' + 10)
		case b[i] >= 'A' && b[i] <= 'F':
			r |= rune(b[i] - 'A' + 10)
		}
	}
	return r
}

func validateToken(dec *json.Decoder, t reflect.Type) error {
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("malformed JSON: %w", err)
	}
	if token == nil {
		if t.Kind() != reflect.Ptr {
			return errors.New("malformed JSON: unexpected null value")
		}
		return nil
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return nil
	}
	switch delim {
	case '{':
		if t.Kind() == reflect.Struct {
			return validateObject(dec, t)
		}
		return skipDelim(dec, '{', '}')
	case '[':
		if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			return validateArrayElements(dec, t.Elem())
		}
		return skipDelim(dec, '[', ']')
	default:
		return fmt.Errorf("malformed JSON: unexpected delimiter %c", delim)
	}
}

func validateObject(dec *json.Decoder, t reflect.Type) error {
	fields := jsonFieldTypes(t)
	seen := map[string]bool{}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return fmt.Errorf("malformed JSON: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("malformed JSON: object key is not a string")
		}
		ft, known := fields[key]
		if !known {
			return fmt.Errorf("malformed JSON: unknown field %q", key)
		}
		if seen[key] {
			return fmt.Errorf("malformed JSON: duplicate field %q", key)
		}
		seen[key] = true
		if err := validateToken(dec, ft); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("malformed JSON: %w", err)
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := f.Name
		omitempty := false
		if tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				name = parts[0]
			}
			for _, part := range parts[1:] {
				if strings.TrimSpace(part) == "omitempty" {
					omitempty = true
					break
				}
			}
		}
		if omitempty {
			continue
		}
		if !seen[name] {
			return fmt.Errorf("malformed JSON: missing required field %q", name)
		}
	}
	return nil
}

func validateArrayElements(dec *json.Decoder, elemType reflect.Type) error {
	for dec.More() {
		if err := validateToken(dec, elemType); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("malformed JSON: %w", err)
	}
	return nil
}

func skipDelim(dec *json.Decoder, open, close json.Delim) error {
	depth := 1
	for depth > 0 {
		token, err := dec.Token()
		if err != nil {
			return fmt.Errorf("malformed JSON: %w", err)
		}
		if delim, ok := token.(json.Delim); ok {
			if delim == open {
				depth++
			} else if delim == close {
				depth--
			}
		}
	}
	return nil
}

func jsonFieldTypes(t reflect.Type) map[string]reflect.Type {
	m := make(map[string]reflect.Type)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := f.Name
		if tag != "" {
			if idx := strings.IndexByte(tag, ','); idx != -1 {
				if idx > 0 {
					name = tag[:idx]
				}
			} else {
				name = tag
			}
		}
		m[name] = f.Type
	}
	return m
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
	if strings.ContainsRune(s.Repository, 0) {
		return errors.New("source.repository: must not contain NUL byte")
	}
	if strings.TrimSpace(s.Commit) == "" {
		return errors.New("source.commit: must not be empty")
	}
	if strings.ContainsRune(s.Commit, 0) {
		return errors.New("source.commit: must not contain NUL byte")
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
	switch task.Distribution {
	case DistributionSingle, DistributionShardable, DistributionReplicated, DistributionTopologyCoupled:
	default:
		return fmt.Errorf("distribution: unknown value %q", task.Distribution)
	}
	if len(task.Argv) == 0 || task.Argv[0] == "" {
		return errors.New("argv: must contain a non-empty command")
	}
	for i, arg := range task.Argv {
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("argv[%d]: must not contain NUL byte", i)
		}
	}
	if task.WorkingDirectory == "" {
		return errors.New("workingDirectory: must not be empty")
	}
	if strings.ContainsRune(task.WorkingDirectory, 0) {
		return errors.New("workingDirectory: must not contain NUL byte")
	}
	if strings.Contains(task.WorkingDirectory, `\`) {
		return errors.New("workingDirectory: must be a clean repository-relative path")
	}
	wd := task.WorkingDirectory
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
		if strings.ContainsRune(item.Name, 0) {
			return fmt.Errorf("environment[%d].name: must not contain NUL byte", i)
		}
		if !envNameRE.MatchString(item.Name) {
			return fmt.Errorf("environment[%d].name: invalid name %q", i, item.Name)
		}
		if strings.ContainsRune(item.Value, 0) {
			return fmt.Errorf("environment[%d].value: must not contain NUL byte", i)
		}
		if seen[item.Name] {
			return fmt.Errorf("environment[%d].name: duplicate name %q", i, item.Name)
		}
		seen[item.Name] = true
	}
	return nil
}

func validateChunk(chunk Chunk) error {
	if chunk.Count < 1 {
		return errors.New("count: must be positive")
	}
	if chunk.Index < 1 || chunk.Index > chunk.Count {
		return fmt.Errorf("index: must be between 1 and count (%d)", chunk.Count)
	}
	return validateDigest("manifestSha256", chunk.ManifestSHA256)
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
