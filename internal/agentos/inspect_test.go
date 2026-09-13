package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/binmgr"
	"github.com/qiangli/coreutils/pkg/execlog"
	"github.com/qiangli/coreutils/pkg/fleet"
	coreskills "github.com/qiangli/coreutils/pkg/skills"
	"github.com/qiangli/coreutils/pkg/weavecli"
)

// TestInspectIndexNamesOnlyAtlasVerbs: the index is a navigation aid, and a
// navigation aid that points at a verb that does not exist is worse than none.
func TestInspectIndexNamesOnlyAtlasVerbs(t *testing.T) {
	known := inspectAtlasVerbs()
	for _, r := range inspectIndex {
		if !known[r.Verb] {
			t.Errorf("index row %q names verb %q, which is not in the atlas", r.Question, r.Verb)
		}
		if !strings.HasPrefix(r.Command, "bashy "+r.Verb) {
			t.Errorf("index row %q: command %q does not start with its verb %q", r.Question, r.Command, r.Verb)
		}
	}
}

// TestInspectNoPathLiterals enforces the "never a second path computation"
// contract on inspect.go itself: every row must come from the owning
// package's accessor, so the file may not spell a store root.
func TestInspectNoPathLiterals(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller path")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(self), "inspect.go"))
	if err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile(`"[^"\n]*(\.bashy|\.config/bashy|\.agents/)[^"\n]*"`)
	for i, line := range bytes.Split(src, []byte("\n")) {
		if strings.HasPrefix(strings.TrimSpace(string(line)), "//") {
			continue
		}
		if literal.Match(line) {
			t.Errorf("inspect.go:%d spells a store root: %s", i+1, strings.TrimSpace(string(line)))
		}
	}
}

// clearAgentSignals removes every input the gate deciders read so a test
// starts from "an interactive human at the keyboard" — including the harness
// markers of whatever agent is running THIS test.
func clearAgentSignals(t *testing.T) {
	t.Helper()
	for _, env := range append(fleet.MarkerEnvs(), "BASHY_AGENTIC", "BASHY_ADVISOR", "BASHY_HINTS", "BASHY_EXECHIST", "BASHY_LEARN", "BASHY_AUDIT") {
		t.Setenv(env, "")
		os.Unsetenv(env)
	}
}

func modeRow(t *testing.T, rows []inspectModeRow, gate string) inspectModeRow {
	t.Helper()
	for _, r := range rows {
		if r.Gate == gate {
			return r
		}
	}
	t.Fatalf("no %q row in inspect mode", gate)
	return inspectModeRow{}
}

// TestInspectModeReportsDecisions is table-driven over the deciding signals:
// each case sets one signal and asserts BOTH the decision (which must equal
// what the real decider says) AND that the explanation names that signal.
func TestInspectModeReportsDecisions(t *testing.T) {
	marker := ""
	for _, env := range fleet.MarkerEnvs() {
		if env != "AGENT" && env != "AI_AGENT" {
			marker = env
			break
		}
	}
	if marker == "" {
		t.Skip("no harness marker env registered")
	}
	cases := []struct {
		name    string
		env     map[string]string
		gate    string
		on      bool
		explain string // substring the decided_by column must carry
	}{
		{"human at the keyboard", nil, "advisor", false, "interactive human"},
		{"BASHY_AGENTIC forces the affordances on", map[string]string{"BASHY_AGENTIC": "1"}, "advisor", true, "BASHY_AGENTIC=1"},
		{"a harness marker alone drives the shell", map[string]string{marker: "1"}, "agent-driven", true, marker},
		{"the marker turns the advisor on by default", map[string]string{marker: "1"}, "advisor", true, "agent-driven"},
		{"explicit BASHY_ADVISOR=off wins over the marker", map[string]string{marker: "1", "BASHY_ADVISOR": "off"}, "advisor", false, "BASHY_ADVISOR=off"},
		{"BASHY_AGENTIC=0 is the master kill", map[string]string{"BASHY_AGENTIC": "0", marker: "1"}, "hints", false, "master kill"},
		{"BASHY_LEARN=off flips learn only", map[string]string{"BASHY_AGENTIC": "1", "BASHY_LEARN": "off"}, "learn", false, "BASHY_LEARN=off"},
		{"learn follows the advisor otherwise", map[string]string{"BASHY_AGENTIC": "1"}, "learn", true, "advisor || hints"},
		{"BASHY_AUDIT=1 turns auditing on", map[string]string{"BASHY_AUDIT": "1"}, "audit", true, "BASHY_AUDIT=1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAgentSignals(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			row := modeRow(t, collectInspectMode(), tc.gate)
			if row.On != tc.on {
				t.Errorf("%s: on=%v, want %v (decided_by=%q)", tc.gate, row.On, tc.on, row.DecidedBy)
			}
			if !strings.Contains(row.DecidedBy, tc.explain) {
				t.Errorf("%s: decided_by=%q does not name %q", tc.gate, row.DecidedBy, tc.explain)
			}
			// The row must agree with the REAL decider, whatever the explanation says.
			var real bool
			switch tc.gate {
			case "advisor":
				real = advisorEnabled()
			case "hints":
				real = hintsEnabled()
			case "learn":
				real = learnEnabled()
			case "audit":
				real = auditEnabled()
			case "agent-driven":
				real = fleet.MarkerEnvs() != nil && (os.Getenv("BASHY_AGENTIC") == "1" || row.On)
			}
			if tc.gate != "agent-driven" && real != row.On {
				t.Errorf("%s: inspect says %v but the decider says %v", tc.gate, row.On, real)
			}
		})
	}
}

// TestInspectPathsDeterministic: two collections agree byte-for-byte once the
// volatile stat columns are masked — the property that makes `inspect paths
// --json` a drift detector across hosts and versions.
func TestInspectPathsDeterministic(t *testing.T) {
	mask := func(rows []inspectPathRow) []byte {
		for i := range rows {
			rows[i].Size, rows[i].Mtime = 0, ""
		}
		b, _ := json.Marshal(rows)
		return b
	}
	a, b := mask(collectInspectPaths()), mask(collectInspectPaths())
	if !bytes.Equal(a, b) {
		t.Fatalf("inspect paths is not deterministic:\n%s\n%s", a, b)
	}
}

// TestInspectPathsContract: every row names its owner, secrets never carry a
// size, and an absent home is reported as unknown rather than as a path.
func TestInspectPathsContract(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range collectInspectPaths() {
		if r.Owner == "" {
			t.Errorf("row %q has no owner accessor", r.Name)
		}
		if seen[r.Name] {
			t.Errorf("duplicate row name %q", r.Name)
		}
		seen[r.Name] = true
		if r.Secret && (r.Size != 0 || r.Mtime != "") {
			t.Errorf("secret row %q leaks size/mtime", r.Name)
		}
		if r.Path == "" && r.Unknown == "" {
			t.Errorf("row %q has neither a path nor a reason", r.Name)
		}
	}
	for _, want := range []string{"skills", "kb", "mb", "sprint", "exec", "audit", "otel-spool", "bin-cache", "todo", "secrets-token"} {
		if !seen[want] {
			t.Errorf("resource map is missing the %q row", want)
		}
	}
}

// TestInspectFoldedAspectsDispatch: the folded verbs keep their own argument
// handling — `--help` still exits 0 through the aspect switch — and an
// unknown aspect is refused with the usage hint.
func TestInspectFoldedAspectsDispatch(t *testing.T) {
	for _, aspect := range []string{"doctor", "context", "audit"} {
		if code := dispatchInspect([]string{aspect, "--help"}); code != 0 {
			t.Errorf("inspect %s --help exited %d", aspect, code)
		}
	}
	if code := dispatchInspect([]string{"nonsense"}); code != 2 {
		t.Errorf("unknown aspect exited %d, want 2", code)
	}
}

// TestContextModeReportsDecisions: `context.mode` must agree with the
// deciders and name the signal, so it can never again print advisor:false
// under a detected harness while the advisor is on.
func TestContextModeReportsDecisions(t *testing.T) {
	marker := ""
	for _, env := range fleet.MarkerEnvs() {
		if env != "AGENT" && env != "AI_AGENT" {
			marker = env
			break
		}
	}
	if marker == "" {
		t.Skip("no harness marker env registered")
	}
	clearAgentSignals(t)
	t.Setenv(marker, "1")
	mode := contextMode{
		Agentic:     envTruthy("BASHY_AGENTIC"),
		AgentDriven: weavecli.IsAgentDriven(),
		Advisor:     advisorEnabled(),
		DecidedBy:   agentDrivenSignal(),
	}
	if mode.Agentic {
		t.Fatal("agentic should be false with BASHY_AGENTIC unset")
	}
	if !mode.AgentDriven || !mode.Advisor {
		t.Fatalf("under marker %s: agent_driven=%v advisor=%v, want both true", marker, mode.AgentDriven, mode.Advisor)
	}
	if !strings.Contains(mode.DecidedBy, marker) {
		t.Fatalf("decided_by=%q does not name the marker %s", mode.DecidedBy, marker)
	}
	t.Setenv("BASHY_ADVISOR", "off")
	if advisorEnabled() {
		t.Fatal("BASHY_ADVISOR=off must win")
	}
}

// TestStorePathsResolveThroughOwners pins bashy's three path names to the
// coreutils accessors that own them — the S2 collapse of the byte-identical
// duplicates that let the space store diverge.
func TestStorePathsResolveThroughOwners(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("BASHY_SKILLS_DIR", "")
	t.Setenv("BASHY_EXECHIST", "")
	if bashySkillsDir() != coreskills.DefaultStoreDir() {
		t.Errorf("bashySkillsDir=%q != skills.DefaultStoreDir=%q", bashySkillsDir(), coreskills.DefaultStoreDir())
	}
	if execHistDir() != execlog.DefaultRoot() {
		t.Errorf("execHistDir=%q != execlog.DefaultRoot=%q", execHistDir(), execlog.DefaultRoot())
	}
	if d, err := binmgr.CacheDir(); err == nil && engineCacheDir() != d {
		t.Errorf("engineCacheDir=%q != binmgr.CacheDir=%q", engineCacheDir(), d)
	}
}

// scratchStores points every store the action rows read at t.TempDir(), so
// the test neither reads nor writes the operator's fleet or skills ring.
func scratchStores(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"BASHY_HOME", "BASHY_FLEET_DIR", "BASHY_SKILLS_DIR", "BASHY_TOOLS_DIR",
		"BASHY_MODELS_DIR", "BASHY_AGENTS_DIR",
	} {
		t.Setenv(key, t.TempDir())
	}
	t.Setenv("BASHY_SKILLS_PATH", "")
}

func actionRow(t *testing.T, rows []inspectActionRow, kind, name string) inspectActionRow {
	t.Helper()
	for _, r := range rows {
		if r.Kind == kind && r.Name == name {
			return r
		}
	}
	t.Fatalf("no %s row named %q in inspect actions", kind, name)
	return inspectActionRow{}
}

// TestInspectActionsFacetRows: one row per action, in the facet's vocabulary,
// for each of the three families something fills today — a verb, a binding
// seeded into the scratch fleet store, and an embedded skill with a valid face.
func TestInspectActionsFacetRows(t *testing.T) {
	scratchStores(t)
	if err := (fleet.New()).SaveAgent(fleet.Agent{Name: "b4-probe", Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	rows := collectInspectActions("")

	// command: the atlas record projected — exact, deterministic, executed by the front door.
	run := actionRow(t, rows, "command", "bashy run")
	if run.Identity != "verb:run" || run.Contract != "none" || run.Latitude != "exact" || run.Authority != "deterministic" || run.Executor != "verb" {
		t.Errorf("command row = %+v", run)
	}
	// agent: judge/agentic by definition; identity is the matrix key, executor the launcher.
	probe := actionRow(t, rows, "agent", "b4-probe")
	if probe.Identity != "codex:gpt5.6-sol" || probe.Latitude != "judge" || probe.Authority != "agentic" || probe.Executor != "agentlaunch:codex" {
		t.Errorf("agent row = %+v", probe)
	}
	// skill: the embedded conductor carries a valid face, so the contract is dhnt
	// and the identity is its content address; it has a judge step, so agentic.
	cond := actionRow(t, rows, "skill", "conductor")
	if cond.Contract != "dhnt" || !strings.HasPrefix(cond.Identity, "h") || cond.Latitude != "judge" || cond.Authority != "agentic" || cond.Executor != "dhnt" || len(cond.EffectsDeclared) == 0 {
		t.Errorf("skill row = %+v", cond)
	}

	// --kind filters to exactly that family; the rows come back kind-sorted.
	for _, kind := range inspectActionKinds {
		for _, r := range collectInspectActions(kind) {
			if r.Kind != kind {
				t.Errorf("--kind %s returned a %s row (%s)", kind, r.Kind, r.Name)
			}
		}
	}
	for i := 1; i < len(rows); i++ {
		a, b := rows[i-1], rows[i]
		if a.Kind > b.Kind || (a.Kind == b.Kind && a.Identity > b.Identity) {
			t.Fatalf("rows not sorted by kind then identity at %d: %s/%s before %s/%s", i, a.Kind, a.Identity, b.Kind, b.Identity)
		}
	}
	if !validInspectActionKind("skill") || validInspectActionKind("dag-target") || validInspectActionKind("") {
		t.Error("the --kind vocabulary must be exactly command script agent skill")
	}
	// Unsupported input fails loudly: an unknown kind, and --kind on an aspect
	// that has no families, both exit 2 rather than silently listing everything.
	if code := dispatchInspect([]string{"actions", "--kind", "bogus"}); code != 2 {
		t.Errorf("inspect actions --kind bogus exited %d, want 2", code)
	}
	if code := dispatchInspect([]string{"paths", "--kind", "skill"}); code != 2 {
		t.Errorf("inspect paths --kind exited %d, want 2", code)
	}
}

// TestInspectActionsGenericOnly: a facet row is the shareable half — no path,
// no host, no home directory ever reaches the JSON.
func TestInspectActionsGenericOnly(t *testing.T) {
	scratchStores(t)
	b, err := json.Marshal(collectInspectActions(""))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{os.Getenv("HOME"), os.Getenv("BASHY_SKILLS_DIR"), os.Getenv("BASHY_FLEET_DIR"), `"location":`, `"host":`, `"path":`} {
		if leak != "" && strings.Contains(string(b), leak) {
			t.Errorf("inspect actions --json carries %q", leak)
		}
	}
}

// TestContextCarriesActionCounts: the first-hop record states every family,
// including the one nothing fills yet — a 0 is an answer, an absent key is not.
func TestContextCarriesActionCounts(t *testing.T) {
	scratchStores(t)
	counts := collectInspectActionCounts()
	if counts.Command == 0 || counts.Skill == 0 {
		t.Fatalf("counts = %+v: the atlas verbs and the embedded skills always project", counts)
	}
	b, err := json.Marshal(contextReport{Actions: counts})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Actions map[string]int `json:"actions"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range inspectActionKinds {
		if _, ok := got.Actions[k]; !ok {
			t.Errorf("context.actions lacks %q (a zero must be stated, not omitted)", k)
		}
	}
	if got.Actions["script"] != 0 {
		t.Errorf("script = %d; nothing projects that family yet, so this test's premise changed", got.Actions["script"])
	}
	if n := len(collectInspectActions("skill")); n != counts.Skill {
		t.Errorf("skill count %d != %d skill rows", counts.Skill, n)
	}
}

// TestDefinePrintsActionFacet ratchets the MOUNT half of the facet on `bashy
// define`: the command bashy wires prints the `runs:` line for a concept that
// has one, and its --json carries the nested action object — never a top-level
// or string-valued one.
func TestDefinePrintsActionFacet(t *testing.T) {
	scratchStores(t)
	out := runDefine(t, "run")
	if !strings.Contains(out, "runs: command contract=none latitude=exact authority=deterministic") {
		t.Errorf("define run lacks the facet line:\n%s", out)
	}
	var d struct {
		Concepts []struct {
			ID     string          `json:"id"`
			Action json.RawMessage `json:"action"`
		} `json:"concepts"`
		Action json.RawMessage `json:"action"`
	}
	if err := json.Unmarshal([]byte(runDefine(t, "run", "--json")), &d); err != nil {
		t.Fatal(err)
	}
	if d.Action != nil {
		t.Error("define --json carries a top-level action key")
	}
	var facet map[string]any
	for _, c := range d.Concepts {
		if c.ID == "verb:run" {
			if err := json.Unmarshal(c.Action, &facet); err != nil {
				t.Fatalf("verb:run action is not an object: %s", c.Action)
			}
		}
	}
	if facet["kind"] != "command" || facet["executor"] != "verb" {
		t.Errorf("verb:run facet = %v", facet)
	}
}

func runDefine(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newDefineCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("define %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}
