package agentos

import (
	"sort"
	"testing"

	"github.com/qiangli/coreutils/pkg/resources"
)

func TestTopSprintProcessesPreservesStableFullSort(t *testing.T) {
	rows := make([]resources.ProcessObservation, 400)
	for i := range rows {
		rows[i].Identity.PID = i
		v := float64((i * 17) % 53)
		if i%7 != 0 {
			rows[i].CPU.Value = &v
		}
	}
	expected := append([]resources.ProcessObservation(nil), rows...)
	sort.SliceStable(expected, func(i, j int) bool {
		a, b := expected[i].CPU.Value, expected[j].CPU.Value
		return a != nil && (b == nil || *a > *b)
	})
	for _, limit := range []int{1, 32, 400, 500} {
		got := topSprintProcesses(rows, limit)
		want := min(limit, len(rows))
		if len(got) != want || cap(got) != want {
			t.Fatalf("retained excessive backing array: len=%d cap=%d want=%d", len(got), cap(got), want)
		}
		for i := range got {
			if got[i].Identity != expected[i].Identity {
				t.Fatalf("stable top-%d row%d differs", limit, i)
			}
		}
	}
	for i := range rows {
		if rows[i].Identity.PID != i {
			t.Fatal("input mutated")
		}
	}
}
