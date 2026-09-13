// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	coreatlas "github.com/qiangli/coreutils/pkg/atlas"
)

func TestContextManifestTeachesKnowledgeStages(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASHY_KB_DIR", filepath.Join(root, "kb"))
	t.Setenv("BASHY_HOME", filepath.Join(root, "home"))
	t.Setenv("BASHY_SKILLS_DIR", filepath.Join(root, "skills"))
	t.Setenv("YCODE_DATA_DIR", filepath.Join(root, "agent-data"))

	report := collectContext()
	var commands []string
	for _, item := range report.RecommendedCommands {
		commands = append(commands, item.Command)
	}
	joined := strings.Join(commands, "\n")
	for _, want := range []string{
		"kb context --for TASK --rings repo,host --forms note,page --budget 700 --json",
		"kb search QUERY",
		"kb observe --ring agent --episode EPISODE --kind KIND --ref REF",
		"kb validate SLUG --ring RING --from-gate EVENT_ID",
		"kb note add --candidate --ring agent --episode EPISODE --title TITLE --body TEXT",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("context manifest does not teach %q\n%s", want, joined)
		}
	}
}

func TestCommandAtlasTeachesKnowledgeSurface(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASHY_KB_DIR", filepath.Join(root, "kb"))
	t.Setenv("BASHY_HOME", filepath.Join(root, "home"))
	t.Setenv("BASHY_SKILLS_DIR", filepath.Join(root, "skills"))
	t.Setenv("YCODE_DATA_DIR", filepath.Join(root, "agent-data"))

	for _, record := range liveAtlas(false) {
		if record.Name != "kb" {
			continue
		}
		if record.Group != coreatlas.GroupKnowledge || !slices.Contains(record.Caps, coreatlas.CapJSON) {
			t.Fatalf("kb atlas classification = group %q caps %v", record.Group, record.Caps)
		}
		for _, want := range []string{"context/search", "observe", "validate", "note add"} {
			if !strings.Contains(record.Synopsis, want) {
				t.Errorf("kb atlas synopsis does not teach %q: %q", want, record.Synopsis)
			}
		}
		return
	}
	t.Fatal("command atlas omitted kb")
}
