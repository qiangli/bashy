package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// seedSprintBoard writes the lease table this test selects against. The
// records are queue.json's `stories` — the same store `bashy sprint` owns —
// written directly so the test pins the READER side of the seam (what
// LiveSprintManagers accepts) without coupling to any sprint CLI behavior.
func seedSprintBoard(t *testing.T, stories []map[string]any) {
	t.Helper()
	dir := os.Getenv("BASHY_SPRINT_DIR")
	if dir == "" {
		dir = t.TempDir()
		t.Setenv("BASHY_SPRINT_DIR", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]any{"next_id": 1, "next_story_id": len(stories) + 1, "stories": stories})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "queue.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func seededStory(id int, holder string, at time.Time) map[string]any {
	s := map[string]any{"id": id, "title": "t", "column": "doing", "created": at}
	if holder != "" {
		s["lease"] = map[string]any{"holder": holder, "at": at}
	}
	return s
}
