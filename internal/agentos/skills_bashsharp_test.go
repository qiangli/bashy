package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	coreskills "github.com/qiangli/yoke/pkg/skills"
)

func TestSkillRunBashSharpContract(t *testing.T) {
	for _, pass := range []bool{true, false} {
		t.Run(fmt.Sprint(pass), func(t *testing.T) {
			store := t.TempDir()
			t.Setenv("BASHY_SKILLS_DIR", store)
			t.Setenv("SKILL_CALLER_VALUE", "caller")
			check := "true"
			if !pass {
				check = "false"
			}
			doc := "---\nname: sharp-fixture\ndescription: tiny runtime fixture\n---\n```bsh\n@guard(effects: \"read\")\n@effects(\"read\")\n@require('test \"$SKILL_CALLER_VALUE\" = caller')\n@ensure('" + check + "')\nwork() { echo body-output; }\nwork\n```\n"
			opts := append(skillsOptions(), coreskills.WithSource(coreskills.EmbedSource(fstest.MapFS{"sharp-fixture/SKILL.md": {Data: []byte(doc)}}, coreskills.RingEmbedded)))
			cmd := coreskills.NewSkillsCmd(opts...)
			var out, log bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&log)
			cmd.SetArgs([]string{"run", "sharp-fixture", "--json"})
			err := cmd.Execute()
			if (err == nil) != pass {
				t.Fatalf("pass=%v err=%v out=%s log=%s", pass, err, &out, &log)
			}
			var rec coreskills.AttestRecord
			if err := json.Unmarshal(out.Bytes(), &rec); err != nil {
				t.Fatalf("receipt: %s: %v", &out, err)
			}
			want := 0
			if !pass {
				want = 3
			}
			if rec.Valid != pass || rec.Status == nil || *rec.Status != want || rec.Identity == "" {
				t.Fatalf("receipt: %+v", rec)
			}
			if !strings.Contains(log.String(), "body-output") {
				t.Fatalf("output not streamed to stderr: %s", &log)
			}
			if !pass && !strings.Contains(log.String(), "postcondition failed") {
				t.Fatalf("missing contract diagnostic: %s", &log)
			}
		})
	}
}
