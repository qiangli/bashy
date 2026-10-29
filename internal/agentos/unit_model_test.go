// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/yoke/pkg/lexicon"
	"github.com/qiangli/yoke/pkg/shape/verify"
	coreskills "github.com/qiangli/yoke/pkg/skills"
	"github.com/qiangli/yoke/pkg/unit"
)

// One promise in two faces: a skill.dhnt (cap {read write}, contract
// {gereeni}) and a SKILL.md Bash# body whose decorators state the same.
const (
	s525Dhnt = "sokilili demo efefecato reada wurite fini sotepo wana reada fini enisure gereeni fini fini\n"
	s525Body = "gereeni() { true; }\n\n@effects(\"read,write\")\n@ensure(\"gereeni\")\nreada() { true; }\n\nreada\n"
)

func s525Doc(name, body string) string {
	doc := "---\nname: " + name + "\ndescription: one-unit fixture\n---\n# " + name + "\n"
	if body != "" {
		doc += "\n```bsh\n" + body + "```\n"
	}
	return doc
}

// TestS525A1PairedFacesShareCapability: bashy's catalog rows read the skill's
// one unit from yoke pkg/unit, so the dhnt face and the Bash# face of one
// promise project the same capability key — and it is exactly the key
// pkg/unit's own fence extractor + contract reader give for the body.
func TestS525A1PairedFacesShareCapability(t *testing.T) {
	fs := fstest.MapFS{
		"by-dhnt/SKILL.md":   {Data: []byte(s525Doc("by-dhnt", ""))},
		"by-dhnt/skill.dhnt": {Data: []byte(s525Dhnt)},
		"by-bsh/SKILL.md":    {Data: []byte(s525Doc("by-bsh", s525Body))},
		"prose/SKILL.md":     {Data: []byte(s525Doc("prose", ""))},
	}
	cat := &coreskills.Catalog{Sources: []coreskills.Source{coreskills.EmbedSource(fs, coreskills.RingEmbedded)}}
	listed, err := cat.List(nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]lexicon.SkillRow{}
	for _, l := range listed {
		rows[l.Skill.Name] = lexiconSkillRow(l.Skill)
	}
	d, b, p := rows["by-dhnt"], rows["by-bsh"], rows["prose"]

	c, err := unit.ReadContract(unit.BashSharpBody(s525Doc("by-bsh", s525Body)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := unit.CapabilityKey(c)
	if err != nil {
		t.Fatal(err)
	}
	if d.Face != coreskills.FaceDhnt || b.Face != coreskills.FaceBashSharp {
		t.Fatalf("faces: dhnt=%q bsh=%q", d.Face, b.Face)
	}
	if d.Capability != want || b.Capability != want {
		t.Fatalf("capability keys: dhnt=%q bsh=%q, pkg/unit=%q", d.Capability, b.Capability, want)
	}
	if !b.FaceValid || b.Identity != unit.BodyIdentity([]byte(s525Body)) {
		t.Fatalf("Bash# row identity %q valid=%v, want the body's content address", b.Identity, b.FaceValid)
	}
	if strings.Join(b.EffectCap, ",") != "read,write" {
		t.Fatalf("Bash# row effect cap = %v", b.EffectCap)
	}
	// Prose-only stays discovery-only: no face, no identity, no capability.
	if p.Face != "" || p.FaceValid || p.Identity != "" || p.Capability != "" {
		t.Fatalf("prose-only row claims a unit: %+v", p)
	}
}

// TestContractCallLog: with BASHY_SHAPE_CALLLOG set, the contract decorator
// path records each contracted function it reaches — before the body runs, so
// a failing precondition still counts — and never one that was not called.
func TestContractCallLog(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv(verify.CallLogEnv, log)
	script := `@ensure('true')
function called() { :; }
@ensure('true')
function uncalled() { :; }
@require('false')
function refused() { echo body-ran; }
called
refused || true
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"BASHY_AUDIT": "0"})
	if err != nil {
		t.Fatalf("script failed: %v (stderr %q)", err, errOut.String())
	}
	if strings.Contains(out.String(), "body-ran") {
		t.Fatal("refused body ran")
	}
	b, rerr := os.ReadFile(log)
	if rerr != nil {
		t.Fatal(rerr)
	}
	got := strings.Fields(string(b))
	if strings.Join(got, ",") != "called,refused" {
		t.Fatalf("call log = %q, want called then refused (uncalled absent)", got)
	}
}
