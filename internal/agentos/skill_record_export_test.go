package agentos

import (
	"strings"
	"testing"

	"github.com/qiangli/bashy/skills"
	"github.com/qiangli/yoke/pkg/redact"
	coreskills "github.com/qiangli/yoke/pkg/skills"
)

func TestEmbeddedBashySkillRecordExportsWithHostIdentity(t *testing.T) {
	source := coreskills.EmbedSource(skills.FS, coreskills.RingEmbedded)
	skill := coreskills.Skill{Name: "bashy"}
	record, err := coreskills.RecordFrom(&skill, source, redact.New(redact.WithHost("qiangli")))
	if err != nil {
		t.Fatalf("export embedded bashy skill record: %v", err)
	}
	if record.Kind != "skill" || record.Name != "bashy" {
		t.Fatalf("unexpected record kind/name: %q/%q", record.Kind, record.Name)
	}
	if !strings.Contains(record.Files["commands/conductor.md"], "conductor-run-deltas.md") {
		t.Fatal("exported record lost conductor guidance")
	}
	if _, err := coreskills.MarshalRecord(record); err != nil {
		t.Fatalf("marshal embedded skill record: %v", err)
	}
}
