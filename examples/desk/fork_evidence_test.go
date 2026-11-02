package desk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Retain only check outcomes from this test's isolated home, not the sealed
// record or candidate environment. The CLI summary omits the nested command
// output that distinguishes provisioning, compilation and assertion failures.
func logForkCheckEvidence(t *testing.T, home string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(home, "src", ".evidence", "*.json"))
	if err != nil {
		t.Logf("fork check evidence: %v", err)
		return
	}
	for _, path := range files {
		checks, err := forkCheckEvidence(path)
		if err != nil {
			t.Logf("fork check evidence %s: %v", filepath.Base(path), err)
			continue
		}
		t.Logf("fork check evidence %s: %s", filepath.Base(path), checks)
	}
}

func forkCheckEvidence(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail,omitempty"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return json.Marshal(record.Checks)
}
