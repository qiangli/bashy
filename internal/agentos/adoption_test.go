package agentos

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/atlas"
)

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func TestVocabularyMatchesAuditedMatrixScope(t *testing.T) {
	var got []string
	for _, o := range CommonOptionsVocabulary {
		got = append(got, strings.TrimPrefix(o.Option, "--"))
	}
	slices.Sort(got)
	want := slices.Clone(commonOptionNames)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("vocabulary %v diverges from audited matrix scope %v", got, want)
	}
}

func TestTriggerRowsAreGeneratedFromDeclarations(t *testing.T) {
	rows := TriggerRows()
	want := len(lensTriggerDecls) + len(adaptTriggerDecls)
	for _, o := range CommonOptionsVocabulary {
		if o.Intent != "" {
			want++
		}
	}
	if len(rows) != want {
		t.Fatalf("rows = %d, want %d (vocabulary intents + lens + adapt)", len(rows), want)
	}
	for _, o := range CommonOptionsVocabulary {
		if o.Intent == "" {
			continue
		}
		if !slices.ContainsFunc(rows, func(r TriggerRow) bool { return r.Trigger == o.Option && r.Example == o.Example }) {
			t.Errorf("no trigger row for vocabulary option %s", o.Option)
		}
	}
	// Mutating a declaration changes the output: nothing is hand-copied.
	saved := lensTriggerDecls
	defer func() { lensTriggerDecls = saved }()
	lensTriggerDecls = append(slices.Clone(saved), TriggerRow{Intent: "x", Trigger: "lens -t zzz", Word: "lens -t zzz", Gloss: "zzz"})
	if !strings.Contains(SkillDescription(), "lens -t zzz (zzz)") {
		t.Error("SkillDescription does not follow lens declarations")
	}
}

// The static projections must equal what the source generates.
func TestAdoptionProjectionsMatchSource(t *testing.T) {
	skill := readRepoFile(t, "skills/bashy/SKILL.md")
	var desc string
	for _, line := range strings.Split(skill, "\n") {
		if d, ok := strings.CutPrefix(line, "description: "); ok {
			desc = d
			break
		}
	}
	if desc != SkillDescription() {
		t.Errorf("skills/bashy/SKILL.md description diverges from SkillDescription():\n got: %s\nwant: %s", desc, SkillDescription())
	}

	opts := readRepoFile(t, "skills/bashy/commands/common-options.md")
	for _, o := range CommonOptionsVocabulary {
		if !strings.Contains(opts, "`"+o.Option) {
			t.Errorf("common-options.md table lacks %s", o.Option)
		}
	}
	for _, line := range strings.Split(opts, "\n") {
		if strings.HasPrefix(line, "| `--") {
			name := strings.TrimPrefix(strings.FieldsFunc(line, func(r rune) bool { return r == '`' || r == '=' })[1], "--")
			if !slices.Contains(commonOptionNames, name) && !slices.Contains([]string{"help", "verbose/--quiet/--debug"}, name) && !strings.HasPrefix(name, "verbose") && name != "help" {
				t.Errorf("common-options.md documents --%s which is not in the vocabulary", name)
			}
		}
	}

	for doc, decls := range map[string][]TriggerRow{
		"skills/bashy/commands/lens.md":  lensTriggerDecls,
		"skills/bashy/commands/adapt.md": adaptTriggerDecls,
	} {
		body := readRepoFile(t, doc)
		for _, r := range decls {
			if !strings.Contains(body, r.Word) {
				t.Errorf("%s does not mention trigger %q", doc, r.Word)
			}
		}
	}
}

func TestHelpCommonOptionsFooterReportsPerOptionStatus(t *testing.T) {
	f := HelpCommonOptionsFooter("sprint show", map[string]bool{"json": true, "zero": true})
	for _, want := range []string{"sprint show", "--json=supported", "--null=mapped", "--explain=mapped", "--no-input=mapped", "--dry-run=missing", "--since=missing", "bashy adapt --OPTION -- CMD"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer lacks %q: %s", want, f)
		}
	}
	all := map[string]bool{}
	for _, o := range CommonOptionsVocabulary {
		all[strings.TrimPrefix(o.Option, "--")] = true
	}
	if f := HelpCommonOptionsFooter("x", all); strings.Contains(f, "missing") || strings.Contains(f, "bashy adapt") {
		t.Errorf("fully supported command should not point at adapt: %s", f)
	}
}

func TestSkillHelpFooterFollowsCommandSchema(t *testing.T) {
	declared := declaredLongFlags("adapt")
	got := skillHelpProcess(t, "classic", "help", "--format", "skill", "adapt")
	for _, o := range CommonOptionsVocabulary {
		want := o.Option + "=" + helpOptionStatus(o, declared)
		if !strings.Contains(string(got), want) {
			t.Errorf("adapt skill help lacks %q", want)
		}
	}
}

func TestMCPCommonOptionsNoteIsWiredIntoServerAndHelp(t *testing.T) {
	note := MCPCommonOptionsNote(cobraSchemas("sprint")["sprint"])
	for _, o := range CommonOptionsVocabulary {
		if !strings.Contains(note, o.Option+"=") {
			t.Errorf("note lacks %s", o.Option)
		}
	}
	if !strings.Contains(cobraSchemaSummary("sprint", cobraSchemas("sprint")), note) {
		t.Error("server discovery summary lacks the common-options note")
	}
	if got := mcpVerbCommand("sprint", mustAtlas(t, "sprint")).Usage; !strings.Contains(got, note) || len(got) > 2048 {
		t.Errorf("verb usage missing note or over limit (%d)", len(got))
	}
	if out := string(skillHelpProcess(t, "classic", "help", "--format", "mcp", "sprint")); !strings.Contains(out, "Common options (argv") {
		t.Error("mcp help lacks the common-options note")
	}
}

func mustAtlas(t *testing.T, name string) atlas.Entry {
	t.Helper()
	e, ok := atlas.Lookup(name)
	if !ok {
		t.Fatalf("no atlas entry %s", name)
	}
	return e
}

func TestLexiconTermsCap(t *testing.T) {
	terms := LexiconTerms()
	if len(terms) > 18 {
		t.Errorf("lexicon terms exceed 18-term cap: %d", len(terms))
	}
	for _, o := range CommonOptionsVocabulary {
		if !slices.Contains(terms, o.Option) {
			t.Errorf("lexicon lacks %s", o.Option)
		}
	}
	for _, w := range []string{"lens", "adapt"} {
		if !slices.Contains(terms, w) {
			t.Errorf("lexicon lacks %s", w)
		}
	}
}

func TestDeclaredNudgeRows(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"unzip", "-l", "file.zip"}, "lens -t listing"},
		{[]string{"tar", "-t", "-f", "a.tgz"}, "lens -t listing"},
		{[]string{"python", "-c", "import yaml"}, "lens"},
		{[]string{"yq", ".a"}, "lens"},
		{[]string{"git", "status", "--json"}, "bashy adapt --json"},
		{[]string{"git", "clean", "--dry-run"}, "bashy adapt --dry-run"},
		{[]string{"rm", "-r", "/tmp/foo"}, ""},
		{[]string{"rm", "-r", "/opt/data"}, "--dry-run"},
	}
	for _, tc := range cases {
		got := SuggestDeclared(tc.args, false)
		if tc.want == "" && got != "" {
			t.Errorf("%v: expected no hint, got %q", tc.args, got)
		}
		if tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("%v: hint %q does not contain %q", tc.args, got, tc.want)
		}
	}
}

func TestShimAgentEnvMarkers(t *testing.T) {
	for agent, want := range map[string]string{"agy": "AGENT='agy'", "muse": "AGENT='muse'", "gemini": ""} {
		dir := t.TempDir()
		t.Setenv("BASHY_SHIM_DIR", dir)
		msg, err := shimInstaller(agent).install(filepath.Join(t.TempDir(), "bashy"), false)
		if err != nil {
			t.Fatalf("install %s: %v", agent, err)
		}
		body, err := os.ReadFile(filepath.Join(dir, "bash"))
		if err != nil {
			t.Fatal(err)
		}
		if want != "" {
			if !strings.Contains(string(body), want) || !strings.Contains(msg, "marker set") {
				t.Errorf("%s shim lacks %s: %q / %q", agent, want, body, msg)
			}
		} else if strings.Contains(string(body), "AGENT=") || strings.Contains(msg, "marker set") {
			t.Errorf("%s shim must not set AGENT: %q", agent, body)
		}
	}
	if !slices.Contains(installAgentOrder, "muse") {
		t.Error("installAgentOrder lacks muse")
	}
	if _, ok := agentInstallers(false)["muse"]; !ok {
		t.Error("agentInstallers lacks muse")
	}
}
