package desk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForkCheckEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.json")
	input := `{"seal":"do-not-report","binding":{"principal":"do-not-report"},"checks":[{"name":"checks/go.bsh","status":"timeout","detail":"compiler still running"}]}`
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := forkCheckEvidence(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"name":"checks/go.bsh","status":"timeout","detail":"compiler still running"}]`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if strings.Contains(string(got), "do-not-report") {
		t.Fatal("reported non-check metadata")
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := forkCheckEvidence(path); err == nil {
		t.Fatal("malformed record accepted")
	}
	if _, err := forkCheckEvidence(path + ".missing"); err == nil {
		t.Fatal("missing record accepted")
	}
}
