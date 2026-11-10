// bash53evidence records the sealed runner's JSON and compares fixture identities.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
)

type summary struct {
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Skipped  int `json:"skipped"`
	TimedOut int `json:"timed_out"`
}
type verdict struct {
	Name    string `json:"name"`
	Verdict string `json:"verdict"`
}
type suite struct {
	SchemaVersion int `json:"schema_version"`
	Context       struct {
		BashPPMode     string `json:"bashpp_mode"`
		BashPPVerified bool   `json:"bashpp_verified"`
	} `json:"context"`
	Infrastructure struct {
		Status string `json:"status"`
	} `json:"infrastructure"`
	Verdicts []verdict `json:"verdicts"`
	Summary  summary   `json:"summary"`
}
type comparison struct {
	New       []string `json:"new"`
	Recovered []string `json:"recovered"`
	Unchanged []string `json:"unchanged"`
}
type evidence struct {
	SchemaVersion int         `json:"schema_version"`
	BashyCommit   string      `json:"bashy_commit"`
	SHPin         string      `json:"sh_pin"`
	ImageID       string      `json:"image_id"`
	ImageDigest   string      `json:"image_digest"`
	BashPPMode    string      `json:"bash53_bashpp"`
	TesteeUID     int         `json:"testee_uid"`
	HostOS        string      `json:"host_os"`
	Timestamp     string      `json:"timestamp"`
	Totals        summary     `json:"totals"`
	Fixtures      []string    `json:"fixtures"`
	Failing       []string    `json:"failing"`
	Timeouts      []string    `json:"timeouts"`
	Skips         []string    `json:"skips"`
	Comparison    *comparison `json:"comparison,omitempty"`
}

func collect(raw []byte, e evidence) (evidence, error) {
	var s suite
	if err := json.Unmarshal(raw, &s); err != nil {
		return e, fmt.Errorf("runner JSON: %w", err)
	}
	if s.SchemaVersion == 0 || s.Infrastructure.Status != "ok" {
		return e, fmt.Errorf("runner infrastructure status %q", s.Infrastructure.Status)
	}
	if e.TesteeUID <= 0 {
		return e, errors.New("testee UID must be non-zero")
	}
	if e.BashPPMode != "0" && e.BashPPMode != "1" {
		return e, errors.New("BASH53_BASHPP must be 0 or 1")
	}
	mode := map[string]string{"0": "off", "1": "on"}[e.BashPPMode]
	if s.Context.BashPPMode != mode || !s.Context.BashPPVerified {
		return e, errors.New("runner did not verify the requested Bash# mode")
	}
	seen := map[string]bool{}
	var totals summary
	for _, v := range s.Verdicts {
		if v.Name == "" || seen[v.Name] {
			return e, fmt.Errorf("empty or duplicate fixture %q", v.Name)
		}
		seen[v.Name] = true
		e.Fixtures = append(e.Fixtures, v.Name)
		switch v.Verdict {
		case "passed":
			totals.Passed++
		case "failed":
			totals.Failed++
			e.Failing = append(e.Failing, v.Name)
		case "timed_out":
			totals.TimedOut++
			e.Failing = append(e.Failing, v.Name)
			e.Timeouts = append(e.Timeouts, v.Name)
		case "skipped":
			totals.Skipped++
			e.Skips = append(e.Skips, v.Name)
		default:
			return e, fmt.Errorf("unknown verdict %q", v.Verdict)
		}
	}
	if !reflect.DeepEqual(totals, s.Summary) {
		return e, fmt.Errorf("runner summary %+v disagrees with verdicts %+v", s.Summary, totals)
	}
	e.Totals = totals
	for _, names := range [][]string{e.Fixtures, e.Failing, e.Timeouts, e.Skips} {
		sort.Strings(names)
	}
	return e, nil
}

func compare(e *evidence, old evidence) error {
	if old.SchemaVersion != 1 || old.BashPPMode != e.BashPPMode || !reflect.DeepEqual(old.Fixtures, e.Fixtures) {
		return errors.New("baseline mode or fixture inventory differs")
	}
	before := map[string]bool{}
	after := map[string]bool{}
	for _, n := range old.Failing {
		before[n] = true
	}
	for _, n := range e.Failing {
		after[n] = true
	}
	c := &comparison{New: []string{}, Recovered: []string{}, Unchanged: []string{}}
	for _, n := range e.Fixtures {
		switch {
		case after[n] && before[n]:
			c.Unchanged = append(c.Unchanged, n)
		case after[n]:
			c.New = append(c.New, n)
		case before[n]:
			c.Recovered = append(c.Recovered, n)
		}
	}
	e.Comparison = c
	return nil
}

func run(args []string) error {
	f := flag.NewFlagSet("bash53evidence", flag.ContinueOnError)
	var input, output, baseline string
	var gate bool
	e := evidence{SchemaVersion: 1}
	f.StringVar(&input, "input", "", "runner JSON path")
	f.StringVar(&output, "output", "", "evidence JSON path")
	f.StringVar(&baseline, "baseline", "", "previous evidence JSON path")
	f.StringVar(&e.BashyCommit, "bashy-commit", "", "bashy commit")
	f.StringVar(&e.SHPin, "sh-pin", "", "sh module pin")
	f.StringVar(&e.ImageID, "image-id", "", "content-addressed image ID")
	f.StringVar(&e.BashPPMode, "mode", "", "0 or 1")
	f.StringVar(&e.HostOS, "host-os", "", "host operating system")
	f.IntVar(&e.TesteeUID, "uid", 0, "container testee UID")
	f.BoolVar(&gate, "gate", false, "require all 86 fixtures to pass")
	if err := f.Parse(args); err != nil {
		return err
	}
	if input == "" || output == "" || e.BashyCommit == "" || e.SHPin == "" || e.ImageID == "" || e.HostOS == "" {
		return errors.New("missing evidence input, output or provenance")
	}
	e.ImageDigest = e.ImageID // OCI image IDs are content-addressed sha256 digests.
	e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	e, err = collect(raw, e)
	if err != nil {
		return err
	}
	if baseline != "" {
		prior, err := os.ReadFile(baseline)
		if err != nil {
			return err
		}
		var old evidence
		if err := json.Unmarshal(prior, &old); err != nil {
			return err
		}
		if err := compare(&e, old); err != nil {
			return err
		}
		fmt.Printf("NEW: %s\nRECOVERED: %s\nUNCHANGED: %s\n", strings.Join(e.Comparison.New, ", "), strings.Join(e.Comparison.Recovered, ", "), strings.Join(e.Comparison.Unchanged, ", "))
	}
	encoded, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(output, append(encoded, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("evidence: %s (%d passed, %d failed, %d timed out, %d skipped)\n", output, e.Totals.Passed, e.Totals.Failed, e.Totals.TimedOut, e.Totals.Skipped)
	if e.Comparison != nil && len(e.Comparison.New) != 0 {
		return fmt.Errorf("NEW failures: %s", strings.Join(e.Comparison.New, ", "))
	}
	if gate && (e.Totals.Passed != 86 || e.Totals.Failed != 0 || e.Totals.TimedOut != 0 || e.Totals.Skipped != 0) {
		return errors.New("sealed gate requires 86/86 with no failures, timeouts or skips")
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bash53evidence:", err)
		os.Exit(1)
	}
}
