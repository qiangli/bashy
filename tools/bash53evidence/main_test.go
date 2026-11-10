package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fakeSuite(t *testing.T, verdicts []verdict) []byte {
	t.Helper()
	s := suite{SchemaVersion: 1, Verdicts: verdicts}
	s.Infrastructure.Status = "ok"
	s.Context.BashPPMode = "off"
	s.Context.BashPPVerified = true
	for _, v := range verdicts {
		switch v.Verdict {
		case "passed":
			s.Summary.Passed++
		case "failed":
			s.Summary.Failed++
		case "timed_out":
			s.Summary.TimedOut++
		case "skipped":
			s.Summary.Skipped++
		}
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEvidenceWriterAndComparison(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "suite.json")
	output := filepath.Join(dir, "evidence.json")
	args := []string{"-input", input, "-output", output, "-bashy-commit", "abc", "-sh-pin", "def", "-image-id", "sha256:123", "-host-os", "Darwin", "-uid", "1000", "-mode", "0"}
	old := fakeSuite(t, []verdict{{"a", "failed"}, {"b", "passed"}, {"c", "timed_out"}, {"d", "skipped"}})
	if err := os.WriteFile(input, old, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	var prior evidence
	b, _ := os.ReadFile(output)
	if err := json.Unmarshal(b, &prior); err != nil {
		t.Fatal(err)
	}
	if prior.BashyCommit != "abc" || prior.SHPin != "def" || prior.ImageDigest != "sha256:123" || prior.TesteeUID != 1000 || prior.Totals.TimedOut != 1 || !reflect.DeepEqual(prior.Failing, []string{"a", "c"}) || !reflect.DeepEqual(prior.Skips, []string{"d"}) {
		t.Fatalf("bad evidence: %+v", prior)
	}
	baseline := filepath.Join(dir, "baseline.json")
	if err := os.Rename(output, baseline); err != nil {
		t.Fatal(err)
	}
	next := fakeSuite(t, []verdict{{"a", "passed"}, {"b", "failed"}, {"c", "failed"}, {"d", "skipped"}})
	if err := os.WriteFile(input, next, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(append(args, "-baseline", baseline)); err == nil {
		t.Fatal("NEW failure should fail")
	}
	b, _ = os.ReadFile(output)
	var got evidence
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Comparison, &comparison{New: []string{"b"}, Recovered: []string{"a"}, Unchanged: []string{"c"}}) {
		t.Fatalf("bad comparison: %+v", got.Comparison)
	}
	if err := os.WriteFile(input, old, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(append(args, "-baseline", baseline)); err != nil {
		t.Fatalf("unchanged baseline: %v", err)
	}
}

func TestEvidenceRejectsMalformedAndIncomplete(t *testing.T) {
	e := evidence{BashPPMode: "0", TesteeUID: 1000}
	for _, tc := range []struct {
		name     string
		verdicts []verdict
		bad      bool
	}{
		{"duplicate", []verdict{{"a", "passed"}, {"a", "failed"}}, true},
		{"unknown", []verdict{{"a", "mystery"}}, true},
		{"good", []verdict{{"a", "passed"}}, false},
	} {
		_, err := collect(fakeSuite(t, tc.verdicts), e)
		if (err != nil) != tc.bad {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "in.json")
	output := filepath.Join(dir, "out.json")
	var on suite
	json.Unmarshal(fakeSuite(t, []verdict{{"a", "passed"}}), &on)
	on.Context.BashPPMode = "on"
	onRaw, _ := json.Marshal(on)
	os.WriteFile(input, onRaw, 0600)
	if err := run([]string{"-input", input, "-output", output, "-bashy-commit", "abc", "-sh-pin", "def", "-image-id", "sha256:123", "-host-os", "Darwin", "-uid", "1000", "-mode", "1", "-gate"}); err == nil {
		t.Fatal("incomplete gate passed")
	}
}
