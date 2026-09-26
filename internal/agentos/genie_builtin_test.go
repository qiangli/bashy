package agentos

import (
	"os"
	"path/filepath"
	"testing"
)

// The builtin source materializes into a genie source tree (the package
// target's inputs), once per digest; a builtin bundle is current only while
// its digest matches, and a developer's build is always kept.
func TestBuiltinGenieSource(t *testing.T) {
	home := t.TempDir()
	digest, err := builtinGenieDigest()
	if err != nil || len(digest) != 64 {
		t.Fatalf("digest %q: %v", digest, err)
	}
	if again, _ := builtinGenieDigest(); again != digest {
		t.Fatal("the digest is not deterministic")
	}
	dir, err := materializeBuiltinGenie(home, digest)
	if err != nil {
		t.Fatal(err)
	}
	if !isGenieSource(dir) {
		t.Fatalf("%s is not a genie source tree", dir)
	}
	for _, rel := range []string{"agent.yaml", "cmd/genie/main.go", "lib/model-server.bsh", "prompts/system.md", "fixture/task.json", "go.mod", "models.json"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	if again, err := materializeBuiltinGenie(home, digest); err != nil || again != dir {
		t.Fatalf("second materialize: %q %v", again, err)
	}

	if !builtinGenieCurrent(home) {
		t.Fatal("no provenance: an existing bundle must be kept")
	}
	if err := writeGenieProvenance(home, genieProvenance{Source: "builtin", Digest: digest}); err != nil {
		t.Fatal(err)
	}
	if !builtinGenieCurrent(home) {
		t.Fatal("a builtin bundle with this digest is current")
	}
	if err := writeGenieProvenance(home, genieProvenance{Source: "builtin", Digest: "0000"}); err != nil {
		t.Fatal(err)
	}
	if builtinGenieCurrent(home) {
		t.Fatal("a builtin bundle from another bashy is stale")
	}
	if err := writeGenieProvenance(home, genieProvenance{Source: "dir", Path: "/src/genie"}); err != nil {
		t.Fatal(err)
	}
	if !builtinGenieCurrent(home) {
		t.Fatal("a developer's build is kept")
	}
}
