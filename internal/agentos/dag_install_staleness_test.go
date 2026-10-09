package agentos

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// bashy dag's own content-fingerprint cache (pkg/dag in the sibling yoke
// module) only hashes files named in a target's Sources:/Generates: lines,
// resolved inside this document's own tree. It has no notion of the Go
// module/import graph, so it cannot see a go.work sibling (../ycode, ../yoke,
// ../sh) change underneath an unrelated Sources: list. Story b24f3f95: a
// `build` target declaring Sources:/Generates: let `bashy dag install`
// (Requires: build) reinstall a stale bin/bashy after a sibling-only fix,
// because the cache reported `build` up to date. The fix is to never let
// `build` use that skip at all — every `bashy dag build`/`install` runs `go
// build`, whose own cache keeps a true no-op cheap. This guards the
// regression: the live dag.md must keep `build` free of Sources:/Generates:.
func TestDagMdBuildTargetNeverSkipsOnStaleFingerprint(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "dag.md"))
	if err != nil {
		t.Fatal(err)
	}
	// A Windows checkout carries CRLF line ends; the heading regexp anchors on \n.
	section := buildTargetSection(t, strings.ReplaceAll(string(data), "\r\n", "\n"))
	for _, key := range []string{"Sources:", "Generates:"} {
		if strings.Contains(section, "\n"+key) || strings.HasPrefix(section, key) {
			t.Fatalf("dag.md's `build` target declares %s, which lets the content-fingerprint cache "+
				"report it up to date while a go.work sibling (../ycode, ../yoke, ../sh) changed "+
				"outside that list, reinstalling a stale bin/bashy via `bashy dag install`:\n%s", key, section)
		}
	}
}

// buildTargetSection extracts the body of the `### build` heading up to the
// next `###` heading (or end of file).
func buildTargetSection(t *testing.T, doc string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^### build\n`)
	loc := re.FindStringIndex(doc)
	if loc == nil {
		t.Fatal("dag.md has no `### build` target")
	}
	rest := doc[loc[1]:]
	if next := regexp.MustCompile(`(?m)^### `).FindStringIndex(rest); next != nil {
		rest = rest[:next[0]]
	}
	return rest
}
