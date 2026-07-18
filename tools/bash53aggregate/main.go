package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
)

type chunkRef struct {
	Index int `json:"index"`
	Of    int `json:"of"`
}

type infrastructure struct {
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

type executionIdentity struct {
	Runner   string `json:"runner"`
	Commit   string `json:"commit"`
	HostOS   string `json:"host_os"`
	HostArch string `json:"host_arch"`
	BashPath string `json:"bash_path"`
}

type record struct {
	SchemaVersion  int             `json:"schema_version"`
	Suite          string          `json:"suite"`
	Chunk          chunkRef        `json:"chunk"`
	RunID          string          `json:"run_id"`
	Context        json.RawMessage `json:"context"`
	Infrastructure infrastructure  `json:"infrastructure"`
	Verdicts       []verdict       `json:"verdicts"`
	Summary        summary         `json:"summary"`
}

func main() {
	expected := flag.Int("expected", 0, "expected number of chunks (defaults to chunk.of)")
	reference := flag.String("reference", "", "unified JSON record whose verdicts must match")
	flag.Parse()
	if len(flag.Args()) == 0 {
		die(errors.New("at least one chunk record is required"))
	}

	records := make([]record, 0, len(flag.Args()))
	for _, path := range flag.Args() {
		r, err := readRecord(path)
		if err != nil {
			die(err)
		}
		records = append(records, r)
	}
	got, err := aggregate(records, *expected)
	if err != nil {
		die(err)
	}
	if *reference != "" {
		want, err := readRecord(*reference)
		if err != nil {
			die(fmt.Errorf("reference: %w", err))
		}
		if err := compareReference(got, want); err != nil {
			die(err)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(got); err != nil {
		die(err)
	}
}

func readRecord(path string) (record, error) {
	f, err := os.Open(path)
	if err != nil {
		return record{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var r record
	if err := dec.Decode(&r); err != nil {
		return record{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := ensureEOF(dec); err != nil {
		return record{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return r, nil
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func aggregate(records []record, expected int) (record, error) {
	if len(records) == 0 {
		return record{}, errors.New("no records")
	}
	first := records[0]
	if first.SchemaVersion != 1 || first.Suite == "" || first.RunID == "" {
		return record{}, errors.New("invalid schema_version, suite, or run_id")
	}
	if first.Chunk.Of < 1 {
		return record{}, errors.New("chunk.of must be positive")
	}
	if expected == 0 {
		expected = first.Chunk.Of
	}
	if expected < 1 || len(records) != expected {
		return record{}, fmt.Errorf("got %d chunk records, want %d", len(records), expected)
	}

	contextKey, err := contextIdentity(first.Context)
	if err != nil {
		return record{}, fmt.Errorf("invalid context: %w", err)
	}
	seenChunks := make(map[int]bool, expected)
	seenVerdicts := make(map[string]bool)
	out := first
	out.Chunk = chunkRef{Index: 0, Of: expected}
	out.Verdicts = nil
	out.Summary = summary{}
	out.Infrastructure = infrastructure{Status: "ok", PreflightErrors: []string{}}

	for _, r := range records {
		if r.SchemaVersion != 1 {
			return record{}, fmt.Errorf("chunk %d has schema_version %d", r.Chunk.Index, r.SchemaVersion)
		}
		if r.Suite != first.Suite || r.RunID != first.RunID {
			return record{}, fmt.Errorf("chunk %d crosses suite/run context", r.Chunk.Index)
		}
		key, err := contextIdentity(r.Context)
		if err != nil || key != contextKey {
			return record{}, fmt.Errorf("chunk %d crosses execution context", r.Chunk.Index)
		}
		if r.Chunk.Of != expected || r.Chunk.Index < 1 || r.Chunk.Index > expected {
			return record{}, fmt.Errorf("invalid chunk %d/%d; expected */%d", r.Chunk.Index, r.Chunk.Of, expected)
		}
		if seenChunks[r.Chunk.Index] {
			return record{}, fmt.Errorf("duplicate chunk %d", r.Chunk.Index)
		}
		seenChunks[r.Chunk.Index] = true
		if r.Infrastructure.Status != "ok" {
			out.Infrastructure.Status = "failed"
		}
		out.Infrastructure.PreflightErrors = append(out.Infrastructure.PreflightErrors, r.Infrastructure.PreflightErrors...)
		for _, v := range r.Verdicts {
			if v.Name == "" || v.Verdict == "" {
				return record{}, fmt.Errorf("chunk %d has incomplete verdict", r.Chunk.Index)
			}
			if seenVerdicts[v.Name] {
				return record{}, fmt.Errorf("duplicate verdict %q", v.Name)
			}
			seenVerdicts[v.Name] = true
			out.Verdicts = append(out.Verdicts, v)
		}
	}
	for i := 1; i <= expected; i++ {
		if !seenChunks[i] {
			return record{}, fmt.Errorf("missing chunk %d", i)
		}
	}
	sort.Slice(out.Verdicts, func(i, j int) bool { return out.Verdicts[i].Name < out.Verdicts[j].Name })
	for _, v := range out.Verdicts {
		switch v.Verdict {
		case "passed":
			out.Summary.Passed++
		case "failed":
			out.Summary.Failed++
		case "skipped":
			out.Summary.Skipped++
		case "timed_out":
			out.Summary.TimedOut++
		default:
			return record{}, fmt.Errorf("verdict %q has invalid value %q", v.Name, v.Verdict)
		}
	}
	return out, nil
}

func contextIdentity(raw json.RawMessage) (executionIdentity, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var identity executionIdentity
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&identity); err != nil {
		return executionIdentity{}, err
	}
	if err := ensureEOF(dec); err != nil {
		return executionIdentity{}, err
	}
	return identity, nil
}

func compareReference(got, want record) error {
	if got.Suite != want.Suite {
		return fmt.Errorf("reference suite = %q, got %q", want.Suite, got.Suite)
	}
	gv, wv := verdictMap(got.Verdicts), verdictMap(want.Verdicts)
	if len(gv) != len(wv) {
		return fmt.Errorf("reference has %d verdicts, aggregate has %d", len(wv), len(gv))
	}
	for name, verdict := range wv {
		if actual, ok := gv[name]; !ok || actual != verdict {
			return fmt.Errorf("reference mismatch for %q: want %q, got %q", name, verdict, actual)
		}
	}
	return nil
}

func verdictMap(verdicts []verdict) map[string]string {
	out := make(map[string]string, len(verdicts))
	for _, v := range verdicts {
		out[v.Name] = v.Verdict
	}
	return out
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "bash53aggregate:", err)
	os.Exit(2)
}
