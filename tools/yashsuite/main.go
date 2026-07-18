package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type fixture struct {
	Name string
	Path string
}

type manifest struct {
	SchemaVersion int                 `json:"schema_version"`
	Suite         string              `json:"suite"`
	ChunkCount    int                 `json:"chunk_count"`
	Measurement   manifestMeasurement `json:"measurement"`
	Chunks        []manifestChunk     `json:"chunks"`
}

type manifestMeasurement struct {
	MeasuredAt     string `json:"measured_at"`
	Runner         string `json:"runner"`
	Command        string `json:"command"`
	Result         string `json:"result"`
	DurationSource string `json:"duration_source"`
}

type manifestChunk struct {
	ID       int               `json:"id"`
	Seconds  float64           `json:"duration_seconds"`
	Fixtures []manifestFixture `json:"fixtures"`
}

type manifestFixture struct {
	Name    string  `json:"name"`
	Seconds float64 `json:"duration_seconds"`
}

type runRecord struct {
	SchemaVersion  int            `json:"schema_version"`
	Suite          string         `json:"suite"`
	Chunk          recordChunk    `json:"chunk"`
	RunID          string         `json:"run_id"`
	Context        map[string]any `json:"context"`
	Infrastructure recordInfra    `json:"infrastructure"`
	Verdicts       []verdict      `json:"verdicts"`
	Summary        summary        `json:"summary"`
}

type recordChunk struct {
	Index int `json:"index"`
	Of    int `json:"of"`
}

type recordInfra struct {
	Status          string   `json:"status"`
	PreflightErrors []string `json:"preflight_errors"`
}

type verdict struct {
	Name            string  `json:"name"`
	Verdict         string  `json:"verdict"`
	DurationSeconds float64 `json:"duration_seconds"`
}

type summary struct {
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Skipped  int `json:"skipped"`
	TimedOut int `json:"timed_out"`
}

func main() {
	var testsDir, shell, chunk, manifestPath, runID string
	var of, shard int
	var jsonOutput, listOnly, chunkCountOnly bool
	var timeout time.Duration
	flag.StringVar(&testsDir, "tests-dir", ".yash-tests/tests", "directory containing yash POSIX fixtures")
	flag.StringVar(&shell, "shell", "bin/bash", "shell under test")
	flag.StringVar(&chunk, "chunk", "", "stable manifest chunk as I/N")
	flag.StringVar(&manifestPath, "chunks-manifest", "yash-chunks.json", "stable yash chunk manifest")
	flag.StringVar(&runID, "run-id", "", "run identifier shared by all chunks")
	flag.IntVar(&of, "of", 0, "number of ad-hoc modulo shards")
	flag.IntVar(&shard, "shard", 0, "one-based ad-hoc shard index")
	flag.BoolVar(&jsonOutput, "json", false, "emit a JSON chunk record")
	flag.BoolVar(&listOnly, "list", false, "list discovered fixture names")
	flag.BoolVar(&chunkCountOnly, "chunk-count", false, "print manifest chunk_count")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "per-fixture timeout")
	flag.Parse()
	if flag.NArg() != 0 {
		die(fmt.Errorf("unexpected arguments: %s", strings.Join(flag.Args(), " ")))
	}
	if chunkCountOnly {
		m, err := loadManifest(manifestPath)
		dieIf(err)
		dieIf(validateManifestHeader(m))
		fmt.Println(m.ChunkCount)
		return
	}

	fixtures, err := discoverFixtures(testsDir)
	dieIf(err)
	if listOnly {
		for _, fixture := range fixtures {
			fmt.Println(fixture.Name)
		}
		return
	}
	if len(fixtures) == 0 {
		die(fmt.Errorf("no *.p.tst or *-p.tst fixtures found in %s", testsDir))
	}

	selected := fixtures
	chunkIndex, chunkTotal := 1, 1
	switch {
	case chunk != "" && (of != 0 || shard != 0):
		die(errors.New("--chunk cannot be combined with --of/--shard"))
	case chunk != "":
		chunkIndex, chunkTotal, err = parseChunk(chunk)
		dieIf(err)
		m, loadErr := loadManifest(manifestPath)
		dieIf(loadErr)
		dieIf(validateManifest(m, fixtures))
		if chunkTotal != m.ChunkCount {
			die(fmt.Errorf("chunk %s does not match manifest chunk_count %d", chunk, m.ChunkCount))
		}
		selected = selectManifest(fixtures, m, chunkIndex)
	case of != 0 || shard != 0:
		if of < 1 || shard < 1 || shard > of {
			die(fmt.Errorf("--of and --shard require 1 <= shard <= of"))
		}
		chunkIndex, chunkTotal = shard, of
		selected = selectShard(fixtures, of, shard)
	}
	if len(selected) == 0 {
		die(errors.New("selected chunk has no fixtures"))
	}
	if runID == "" {
		runID = os.Getenv("YASH_RUN_ID")
	}
	if runID == "" {
		runID = fmt.Sprintf("yash-%d", time.Now().UTC().Unix())
	}

	shellPath, err := filepath.Abs(shell)
	dieIf(err)
	if _, err := os.Stat(shellPath); err != nil {
		if found, lookErr := exec.LookPath(shell); lookErr == nil {
			shellPath = found
		} else {
			die(fmt.Errorf("shell under test not found: %s", shell))
		}
	}
	testsDir, err = filepath.Abs(testsDir)
	dieIf(err)
	if _, err := os.Stat(filepath.Join(testsDir, "run-test.sh")); err != nil {
		die(fmt.Errorf("yash test harness not found: %w", err))
	}

	record := runRecord{
		SchemaVersion: 1, Suite: "yash", Chunk: recordChunk{Index: chunkIndex, Of: chunkTotal}, RunID: runID,
		Context:        map[string]any{"shell": shellPath, "tests_dir": testsDir},
		Infrastructure: recordInfra{Status: "ok", PreflightErrors: []string{}}, Verdicts: []verdict{},
	}
	for _, fixture := range selected {
		start := time.Now()
		status, runErr := runFixture(testsDir, shellPath, fixture, timeout)
		elapsed := time.Since(start).Seconds()
		record.Verdicts = append(record.Verdicts, verdict{Name: fixture.Name, Verdict: status, DurationSeconds: elapsed})
		switch status {
		case "passed":
			record.Summary.Passed++
		case "failed":
			record.Summary.Failed++
		case "timed_out":
			record.Summary.TimedOut++
		}
		if !jsonOutput {
			fmt.Printf("%-9s %s (%.3fs)\n", strings.ToUpper(status), fixture.Name, elapsed)
			if runErr != nil && status != "timed_out" {
				fmt.Printf("          %v\n", runErr)
			}
		}
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		dieIf(enc.Encode(record))
	}
	if record.Summary.Failed != 0 || record.Summary.TimedOut != 0 {
		os.Exit(1)
	}
}

func discoverFixtures(testsDir string) ([]fixture, error) {
	entries, err := os.ReadDir(testsDir)
	if err != nil {
		return nil, fmt.Errorf("read tests directory %s: %w", testsDir, err)
	}
	var fixtures []fixture
	for _, entry := range entries {
		if entry.IsDir() || !(strings.HasSuffix(entry.Name(), ".p.tst") || strings.HasSuffix(entry.Name(), "-p.tst")) {
			continue
		}
		fixtures = append(fixtures, fixture{Name: strings.TrimSuffix(entry.Name(), ".tst"), Path: filepath.Join(testsDir, entry.Name())})
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].Name < fixtures[j].Name })
	return fixtures, nil
}

func loadManifest(path string) (*manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open chunk manifest %s: %w", path, err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var m manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode chunk manifest %s: %w", path, err)
	}
	return &m, nil
}

func validateManifestHeader(m *manifest) error {
	if m == nil || m.SchemaVersion != 1 || m.Suite != "yash" || m.ChunkCount < 1 {
		return errors.New("manifest requires schema_version 1, suite yash, and positive chunk_count")
	}
	return nil
}

func validateManifest(m *manifest, fixtures []fixture) error {
	if err := validateManifestHeader(m); err != nil {
		return err
	}
	if len(m.Chunks) != m.ChunkCount {
		return fmt.Errorf("manifest has %d chunks, want %d", len(m.Chunks), m.ChunkCount)
	}
	seenChunks := map[int]bool{}
	seenFixtures := map[string]int{}
	for _, chunk := range m.Chunks {
		if chunk.ID < 1 || chunk.ID > m.ChunkCount || seenChunks[chunk.ID] {
			return fmt.Errorf("invalid or duplicate chunk id %d", chunk.ID)
		}
		seenChunks[chunk.ID] = true
		if len(chunk.Fixtures) == 0 {
			return fmt.Errorf("chunk %d has no fixtures", chunk.ID)
		}
		for _, fixture := range chunk.Fixtures {
			seenFixtures[fixture.Name]++
			if fixture.Name == "" || seenFixtures[fixture.Name] != 1 {
				return fmt.Errorf("invalid or duplicate fixture %q", fixture.Name)
			}
		}
	}
	known := map[string]bool{}
	for _, fixture := range fixtures {
		known[fixture.Name] = true
		if seenFixtures[fixture.Name] != 1 {
			return fmt.Errorf("fixture %q appears %d times, want once", fixture.Name, seenFixtures[fixture.Name])
		}
	}
	for name := range seenFixtures {
		if !known[name] {
			return fmt.Errorf("manifest includes unknown fixture %q", name)
		}
	}
	return nil
}

func selectManifest(fixtures []fixture, m *manifest, id int) []fixture {
	wanted := map[string]bool{}
	for _, chunk := range m.Chunks {
		if chunk.ID == id {
			for _, fixture := range chunk.Fixtures {
				wanted[fixture.Name] = true
			}
		}
	}
	var selected []fixture
	for _, fixture := range fixtures {
		if wanted[fixture.Name] {
			selected = append(selected, fixture)
		}
	}
	return selected
}

func selectShard(fixtures []fixture, of, shard int) []fixture {
	var selected []fixture
	for i, fixture := range fixtures {
		if i%of == shard-1 {
			selected = append(selected, fixture)
		}
	}
	return selected
}

func parseChunk(value string) (int, int, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("chunk must be I/N, got %q", value)
	}
	i, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || i < 1 || n < 1 || i > n {
		return 0, 0, fmt.Errorf("invalid chunk %q", value)
	}
	return i, n, nil
}

func runFixture(testsDir, shell string, fixture fixture, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "run-test.sh", shell, filepath.Base(fixture.Path))
	cmd.Dir = testsDir
	output, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timed_out", ctx.Err()
	}
	if err != nil {
		return "failed", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return "passed", nil
}

func dieIf(err error) {
	if err != nil {
		die(err)
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "yashsuite:", err)
	os.Exit(2)
}
