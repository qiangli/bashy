package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func rec(index, of int, runID string, names ...string) record {
	vs := make([]verdict, 0, len(names))
	for _, name := range names {
		vs = append(vs, verdict{Name: name, Verdict: "passed", DurationSeconds: 1})
	}
	return record{SchemaVersion: 1, Suite: "bash-5.3", Chunk: chunkRef{Index: index, Of: of}, RunID: runID,
		Context: json.RawMessage(`{"runner":"bash53suite","commit":"abc123","started_at":"2026-07-18T10:00:00Z","finished_at":"2026-07-18T10:01:00Z","host_os":"linux","host_arch":"amd64","bash_path":"/workspace/bin/bash"}`), Infrastructure: infrastructure{Status: "ok"}, Verdicts: vs}
}

func TestValidSet(t *testing.T) {
	got, err := aggregate([]record{rec(2, 2, "run", "b"), rec(1, 2, "run", "a")}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary.Passed != 2 || got.Verdicts[0].Name != "a" || got.Chunk.Index != 0 {
		t.Fatalf("unexpected aggregate: %+v", got)
	}
}

func TestMissingChunk(t *testing.T) {
	if _, err := aggregate([]record{rec(1, 2, "run", "a")}, 2); err == nil {
		t.Fatal("missing chunk accepted")
	}
}

func TestDuplicateChunk(t *testing.T) {
	_, err := aggregate([]record{rec(1, 2, "run", "a"), rec(1, 2, "run", "b")}, 2)
	if err == nil || !strings.Contains(err.Error(), "duplicate chunk") {
		t.Fatalf("got %v, want duplicate error", err)
	}
}

func TestCrossContext(t *testing.T) {
	b := rec(2, 2, "other-run", "b")
	if _, err := aggregate([]record{rec(1, 2, "run", "a"), b}, 2); err == nil {
		t.Fatal("cross-run set accepted")
	}
}

func TestPerInvocationTimestampsAccepted(t *testing.T) {
	b := rec(2, 2, "run", "b")
	b.Context = json.RawMessage(`{"runner":"bash53suite","commit":"abc123","started_at":"2026-07-18T11:00:00Z","finished_at":"2026-07-18T11:01:00Z","host_os":"linux","host_arch":"amd64","bash_path":"/workspace/bin/bash"}`)
	if _, err := aggregate([]record{rec(1, 2, "run", "a"), b}, 2); err != nil {
		t.Fatalf("timestamp-only context difference rejected: %v", err)
	}
}

func TestStableIdentityDifferenceRejected(t *testing.T) {
	b := rec(2, 2, "run", "b")
	b.Context = json.RawMessage(`{"runner":"bash53suite","commit":"different","started_at":"2026-07-18T10:00:00Z","finished_at":"2026-07-18T10:01:00Z","host_os":"linux","host_arch":"amd64","bash_path":"/workspace/bin/bash"}`)
	_, err := aggregate([]record{rec(1, 2, "run", "a"), b}, 2)
	if err == nil || !strings.Contains(err.Error(), "crosses execution context") {
		t.Fatalf("got %v, want cross-context error", err)
	}
}

func TestReferenceMatchAndMismatch(t *testing.T) {
	got, err := aggregate([]record{rec(1, 1, "run", "a")}, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := got
	want.Verdicts = append([]verdict(nil), got.Verdicts...)
	want.Verdicts[0].DurationSeconds = 99
	if err := compareReference(got, want); err != nil {
		t.Fatalf("matching reference rejected: %v", err)
	}
	want.Verdicts[0].Verdict = "failed"
	if err := compareReference(got, want); err == nil {
		t.Fatal("mismatching reference accepted")
	}
}
