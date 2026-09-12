// inspect.go — `bashy inspect`: bashy's self-inspection verb.
//
// The subject is bashy ITSELF. `doctor` answers about the host, `why` about the
// process tree, `otel` about a trace, `audit` about one store; inspect is the
// resource map (where every store bashy owns lives, scope-resolved for THIS
// cwd), the gate decisions (each middleware gate with the SIGNAL that decided
// it), and the index that names which verb answers every other question about
// bashy. It exists so an agent — or a third-party harness driving bashy — can
// navigate bashy without grepping its source.
//
// Contract (docs in the umbrella; the public statement is `--help`):
//   - read-only, offline, model-free: no mutation, no provisioning, no network,
//     no agent spawn;
//   - NEVER a second path computation: every row calls the accessor the owning
//     package already uses. TestInspectNoPathLiterals fails this file on any
//     `.bashy` / `.config/bashy` / `.agents/` literal;
//   - secrets: path and existence only, never a value or a size;
//   - an undeterminable row says "unknown" and why — never a default;
//   - deterministic for a given host state, so two hosts' `inspect paths --json`
//     diff into a drift report.
//
// `doctor`, `context` and `audit` fold behind it as aspects (subject = bashy AND
// effects = {read}); their bodies are unchanged and their old names remain as
// hidden aliases. A new read-only self-view is a new ASPECT here, never a new
// verb.
package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/binmgr"
	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/capability"
	"github.com/qiangli/coreutils/pkg/chat"
	"github.com/qiangli/coreutils/pkg/craft"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/kb"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/resources"
	"github.com/qiangli/coreutils/pkg/room"
	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/pkg/secrets"
	"github.com/qiangli/coreutils/pkg/telemetry"
	"github.com/qiangli/coreutils/pkg/todo"
	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/qiangli/coreutils/pkg/weavecli"

	"github.com/qiangli/bashy/internal/agentos/activity"
	"github.com/qiangli/coreutils/external/loom"
)

const inspectSchemaVersion = "bashy-inspect-v1"

// inspectIndexRow is one question an agent asks about bashy and the command
// that answers it today. Verb is the atlas name the command starts with;
// TestInspectIndexNamesOnlyAtlasVerbs keeps the table honest.
type inspectIndexRow struct {
	Question string `json:"question"`
	Command  string `json:"command"`
	Verb     string `json:"verb"`
}

// inspectIndex is a static table on purpose: it is the navigation aid, and a
// navigation aid that lists something unavailable is worse than none.
var inspectIndex = []inspectIndexRow{
	{"where does bashy keep X (every store, scope-resolved for this cwd)", "bashy inspect paths", "inspect"},
	{"which gates are on, and which signal decided each", "bashy inspect mode", "inspect"},
	{"is this host healthy for bashy (PATH/sh, engines, bin cache)", "bashy inspect doctor", "inspect"},
	{"the first-hop record an agent reads before anything else", "bashy inspect context --json", "inspect"},
	{"what ran here, tamper-evident (status, tail, verify, export)", "bashy inspect audit status", "inspect"},
	{"what commands exist and how each one resolves", "bashy commands X --features", "commands"},
	{"the full command atlas (group, tier, stage, caps, effects)", "bashy commands --atlas", "commands"},
	{"which commands compose naturally", "bashy commands --idioms", "commands"},
	{"what did I run (episodes, with coverage)", "bashy graph history", "graph"},
	{"why is a trace slow, what did LLM calls cost", "bashy otel why-slow TRACE_ID; bashy otel cost", "otel"},
	{"what the learn middleware recorded from past invocations", "bashy craft facts; bashy craft history", "craft"},
	{"what a name means on this system, how to reach it", "bashy define NAME; bashy whois NAME", "define"},
	{"the complete output behind an elision marker", "bashy out HANDLE", "out"},
	{"why this process or port exists", "bashy why NAME", "why"},
	{"who an event reached, and why it reached them", "bashy activity routes ID", "activity"},
}

// inspectPathRow is one store, directory or file bashy owns.
type inspectPathRow struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
	Path    string `json:"path"`
	Scope   string `json:"scope"` // user | repo | cache
	Exists  bool   `json:"exists"`
	Kind    string `json:"kind,omitempty"` // dir | file | "" (absent)
	Size    int64  `json:"size,omitempty"`
	Mtime   string `json:"mtime,omitempty"`
	Env     string `json:"env,omitempty"` // the override variable, when one exists
	Owner   string `json:"owner"`         // the accessor that computes the path
	ReadBy  string `json:"read_by,omitempty"`
	Secret  bool   `json:"secret,omitempty"`  // existence only; size/mtime withheld
	Unknown string `json:"unknown,omitempty"` // why the path could not be determined
}

// inspectModeRow is one middleware gate and the decision it actually made.
type inspectModeRow struct {
	Gate      string `json:"gate"`
	On        bool   `json:"on"`
	DecidedBy string `json:"decided_by"`
	Evidence  string `json:"evidence,omitempty"`
}

// dispatchInspect implements `bashy inspect [ASPECT] [args…]`.
//
// The folded aspects (doctor, context, audit) receive their arguments verbatim
// so their behaviour — flags, output bytes, exit codes — is unchanged. The
// old top-level names route here too (see the dispatch switch), which is what
// makes them aliases rather than copies.
func dispatchInspect(args []string) int {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		aspect, rest := args[0], args[1:]
		switch aspect {
		case "doctor":
			return dispatchDoctor(rest)
		case "context":
			return dispatchContext(rest)
		case "audit":
			return dispatchAudit(rest)
		case "paths", "mode":
			return dispatchInspectAspect(aspect, rest)
		case "help":
			inspectUsage(os.Stdout)
			return 0
		default:
			fmt.Fprintf(os.Stderr, "bashy inspect: unknown aspect %q (try: paths mode doctor context audit)\n", aspect)
			return 2
		}
	}
	return dispatchInspectAspect("index", args)
}

func inspectUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: bashy inspect [ASPECT] [--json|--plain]")
	fmt.Fprintln(w, "Self-inspection: what bashy is, where it keeps things, which of its gates are on.")
	fmt.Fprintln(w, "  (none)    index — every question, and the verb that answers it")
	fmt.Fprintln(w, "  paths     the resource map: every store bashy owns, scope-resolved for this cwd")
	fmt.Fprintln(w, "  mode      effective gate decisions, each with the signal that decided it")
	fmt.Fprintln(w, "  doctor    diagnose the host environment            (was: bashy doctor)")
	fmt.Fprintln(w, "  context   the first-hop agent record               (was: bashy context)")
	fmt.Fprintln(w, "  audit     the tamper-evident command trail          (was: bashy audit)")
	fmt.Fprintln(w, "Read-only, offline, model-free. Secrets are reported by path and existence only.")
}

func dispatchInspectAspect(aspect string, args []string) int {
	asJSON := weavecli.IsAgent()
	for _, a := range args {
		switch a {
		case "--json", "--json=true":
			asJSON = true
		case "--json=false", "--plain":
			asJSON = false
		case "-h", "--help":
			inspectUsage(os.Stdout)
			return 0
		default:
			fmt.Fprintf(os.Stderr, "bashy inspect %s: unknown option %q\n", aspect, a)
			return 2
		}
	}
	switch aspect {
	case "index":
		if asJSON {
			return inspectEmitJSON(aspect, inspectIndex)
		}
		fmt.Println("bashy inspect — what can be asked about bashy, and what answers it")
		w := 0
		for _, r := range inspectIndex {
			if len(r.Command) > w {
				w = len(r.Command)
			}
		}
		for _, r := range inspectIndex {
			fmt.Printf("  %-*s  %s\n", w, r.Command, r.Question)
		}
		return 0
	case "paths":
		rows := collectInspectPaths()
		if asJSON {
			return inspectEmitJSON(aspect, rows)
		}
		fmt.Printf("bashy inspect paths (%d):\n", len(rows))
		for _, r := range rows {
			state := "absent"
			switch {
			case r.Unknown != "":
				state = "unknown: " + r.Unknown
			case r.Exists && r.Secret:
				state = "present (secret; size withheld)"
			case r.Exists && r.Kind == "dir":
				state = "dir"
			case r.Exists:
				state = fmt.Sprintf("%s %s", r.Kind, inspectHumanSize(r.Size))
			}
			env := ""
			if r.Env != "" {
				env = "  $" + r.Env
			}
			fmt.Printf("  %-14s %-5s %s\n      %s%s\n", r.Name, r.Scope, r.Path, state, env)
		}
		return 0
	case "mode":
		rows := collectInspectMode()
		if asJSON {
			return inspectEmitJSON(aspect, rows)
		}
		fmt.Println("bashy inspect mode — each gate, and the signal that decided it:")
		for _, r := range rows {
			ev := ""
			if r.Evidence != "" {
				ev = "  → " + r.Evidence
			}
			fmt.Printf("  %-13s %-3s %s%s\n", r.Gate, onOff(r.On), r.DecidedBy, ev)
		}
		return 0
	}
	return 2
}

func inspectEmitJSON(aspect string, rows any) int {
	b, err := json.Marshal(map[string]any{
		"schema_version": inspectSchemaVersion,
		"aspect":         aspect,
		"rows":           rows,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy inspect:", err)
		return 1
	}
	fmt.Println(string(b))
	return 0
}

func inspectHumanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// inspectStat fills the existence/size/mtime columns from ONE stat. Directories
// report the size of their immediate entries' metadata as 0 — a `du` walk is
// deliberately not done (bounded cost; see the design's --deep reservation).
func inspectStat(r *inspectPathRow) {
	if r.Path == "" {
		if r.Unknown == "" {
			r.Unknown = "no home directory could be determined"
		}
		return
	}
	fi, err := os.Stat(r.Path)
	if err != nil {
		return // absent: Exists stays false, nothing else to say
	}
	r.Exists = true
	if fi.IsDir() {
		r.Kind = "dir"
	} else {
		r.Kind = "file"
	}
	if r.Secret {
		return
	}
	if !fi.IsDir() {
		r.Size = fi.Size()
	}
	r.Mtime = fi.ModTime().UTC().Format("2006-01-02T15:04:05Z")
}

// collectInspectPaths is the resource map. Every Path comes from the accessor
// the owning package uses; the Owner column names it so a reader can verify.
func collectInspectPaths() []inspectPathRow {
	var rows []inspectPathRow
	add := func(r inspectPathRow) {
		inspectStat(&r)
		rows = append(rows, r)
	}
	errPath := func(p string, err error) (string, string) {
		if err != nil {
			return "", err.Error()
		}
		return p, ""
	}

	// ── config ring (definitions an operator authors) ──
	fleetRoot := fleet.DefaultRoot()
	add(inspectPathRow{Name: "fleet", Purpose: "tool/model/agent definitions, one YAML per entry", Path: fleetRoot, Scope: "user", Env: "BASHY_FLEET_DIR", Owner: "fleet.DefaultRoot", ReadBy: "tools/models/agents list"})
	for _, noun := range []string{"tools", "models", "agents"} {
		add(inspectPathRow{Name: noun, Purpose: "the " + noun + " noun store", Path: fleet.NounDir(fleetRoot, noun), Scope: "user", Env: "BASHY_" + strings.ToUpper(noun) + "_DIR", Owner: "fleet.NounDir", ReadBy: "bashy " + noun})
	}
	skills := bashySkillsDir()
	add(inspectPathRow{Name: "skills", Purpose: "the local skills ring; also the craft store", Path: skills, Scope: "user", Env: "BASHY_SKILLS_DIR", Owner: "bashySkillsDir", ReadBy: "skills list; craft"})
	add(inspectPathRow{Name: "facts", Purpose: "craft facts (host-local, never exported)", Path: craft.OpenFacts(craftStoreDir()).Path(), Scope: "user", Owner: "craft.OpenFacts().Path", ReadBy: "craft facts; the learn middleware"})
	add(inspectPathRow{Name: "folds", Purpose: "craft folds (generalisable, shareable)", Path: craft.OpenFolds(craftStoreDir(), nil).Path(), Scope: "user", Owner: "craft.OpenFolds().Path", ReadBy: "craft folds"})
	add(inspectPathRow{Name: "attest", Purpose: "skills run receipts", Path: inspectSubdir(craft.AttestDir, craftStoreDir()), Scope: "user", Owner: "craft.AttestDir", ReadBy: "craft history; skills"})
	add(inspectPathRow{Name: "hints", Purpose: "once-per-repo skill-hint markers", Path: hintsDir(), Scope: "user", Owner: "hintsDir", ReadBy: "the advertisement ladder"})
	add(inspectPathRow{Name: "activity", Purpose: "activity-event journal and interests", Path: activity.StateDir(), Scope: "user", Env: "BASHY_ACTIVITY_DIR", Owner: "activity.StateDir", ReadBy: "activity; inbox"})
	add(inspectPathRow{Name: "schedule", Purpose: "the schedule store", Path: schedule.StatePathFor(inspectCwd(), os.Environ()), Scope: "user", Owner: "schedule.StatePathFor", ReadBy: "bashy schedule"})
	add(inspectPathRow{Name: "secrets-token", Purpose: "the scoped cloudbox token for bashy secrets", Path: secrets.TokenFilePath(), Scope: "user", Env: "BASHY_SECRETS_TOKEN", Owner: "secrets.TokenFilePath", ReadBy: "bashy secrets", Secret: true})

	// ── state (what bashy and its agents write while working) ──
	add(inspectPathRow{Name: "kb", Purpose: "host knowledge base", Path: kb.DefaultDir(), Scope: "user", Env: "BASHY_KB_DIR", Owner: "kb.DefaultDir", ReadBy: "kb; recall"})
	add(inspectPathRow{Name: "mb", Purpose: "the public message board", Path: bus.BoardDir(), Scope: "user", Env: "BASHY_MB_DIR", Owner: "bus.BoardDir", ReadBy: "mb; inbox"})
	p, u := errPath(meet.BaseDir())
	add(inspectPathRow{Name: "meet", Purpose: "meet rooms, rosters, minutes", Path: p, Unknown: u, Scope: "user", Env: "BASHY_MEET_DIR", Owner: "meet.BaseDir", ReadBy: "meet; inbox"})
	add(inspectPathRow{Name: "room", Purpose: "room mesh state", Path: room.Dir(), Scope: "user", Env: "BASHY_ROOM_DIR", Owner: "room.Dir", ReadBy: "room; meet"})
	p, u = errPath(todo.Root())
	add(inspectPathRow{Name: "todo-host", Purpose: "the per-host personal task lists", Path: p, Unknown: u, Scope: "user", Env: "BASHY_TODO_DIR", Owner: "todo.Root", ReadBy: "todo --user"})
	p, u = errPath(weave.SprintStoreDir())
	add(inspectPathRow{Name: "sprint", Purpose: "the sprint board (queue, leases, continuity)", Path: p, Unknown: u, Scope: "user", Env: "BASHY_SPRINT_DIR", Owner: "weave.SprintStoreDir", ReadBy: "sprint"})
	add(inspectPathRow{Name: "weave", Purpose: "weave runs and workspace registry", Path: weave.StateRoot(), Scope: "user", Owner: "weave.StateRoot", ReadBy: "weave"})
	add(inspectPathRow{Name: "exec", Purpose: "exec history episodes (execlog)", Path: execHistDir(), Scope: "user", Env: "BASHY_EXECHIST", Owner: "execHistDir", ReadBy: "graph history; graph reached"})
	if st, err := shellOutputStoreForEnv(os.Environ()); err == nil {
		add(inspectPathRow{Name: "out", Purpose: "complete command output behind elision markers", Path: st.Root(), Scope: "user", Env: "BASHY_HOME", Owner: "shellOutputStoreForEnv().Root", ReadBy: "bashy out HANDLE"})
	} else {
		add(inspectPathRow{Name: "out", Purpose: "complete command output behind elision markers", Unknown: err.Error(), Scope: "user", Owner: "shellOutputStoreForEnv"})
	}
	add(inspectPathRow{Name: "audit", Purpose: "the hash-chained command audit log", Path: auditPath(), Scope: "user", Env: "BASHY_AUDIT", Owner: "auditPath", ReadBy: "inspect audit"})
	p, u = errPath(chat.SessionsRoot())
	add(inspectPathRow{Name: "sessions", Purpose: "chat sessions and capture logs", Path: p, Unknown: u, Scope: "user", Owner: "chat.SessionsRoot", ReadBy: "chat"})
	add(inspectPathRow{Name: "capability", Purpose: "the agent × capability ledger", Path: capability.LedgerPath(), Scope: "user", Owner: "capability.LedgerPath", ReadBy: "capability"})
	add(inspectPathRow{Name: "resources", Purpose: "host resource observations", Path: resources.ResourcesStateDir(), Scope: "user", Env: "BASHY_HOME", Owner: "resources.ResourcesStateDir", ReadBy: "resources; sprint monitor"})

	// ── telemetry and managed externals ──
	add(inspectPathRow{Name: "otel-spool", Purpose: "the OTel span spool (file exporter)", Path: telemetry.SpoolPath(), Scope: "user", Env: "BASHY_OTEL_SPOOL", Owner: "telemetry.SpoolPath", ReadBy: "otel; graph history"})
	add(inspectPathRow{Name: "loom", Purpose: "managed loom (forge) data", Path: loom.DefaultDataDir(), Scope: "user", Owner: "loom.DefaultDataDir", ReadBy: "bashy loom"})

	// ── caches (safe to delete; re-provisioned on use) ──
	p, u = errPath(binmgr.CacheDir())
	add(inspectPathRow{Name: "bin-cache", Purpose: "downloaded, checksummed external binaries", Path: p, Unknown: u, Scope: "cache", Env: "BASHY_BIN_CACHE", Owner: "binmgr.CacheDir", ReadBy: "every managed external"})
	if mp := defaultMemoryPath(); mp != "" {
		add(inspectPathRow{Name: "advisor", Purpose: "the space-time advisor's host memory", Path: mp, Scope: "cache", Env: "BASHY_ADVISOR_STATE", Owner: "defaultMemoryPath", ReadBy: "the advisor"})
	} else {
		add(inspectPathRow{Name: "advisor", Purpose: "the space-time advisor's host memory", Unknown: "persistence off (BASHY_ADVISOR_NOMEM) or no cache dir", Scope: "cache", Owner: "defaultMemoryPath"})
	}

	// ── scope-resolved for THIS cwd: what a harness must know before it files anything ──
	if st, label, err := todo.ResolveStore("", false, false, ""); err == nil {
		add(inspectPathRow{Name: "todo", Purpose: "the task list that applies here (" + label + ")", Path: st.Dir(), Scope: inspectScopeKind(st.Sub == ""), Owner: "todo.ResolveStore().Dir", ReadBy: "todo"})
	} else {
		add(inspectPathRow{Name: "todo", Purpose: "the task list that applies here", Unknown: err.Error(), Scope: "repo", Owner: "todo.ResolveStore"})
	}
	if root, ok := todo.FindGitRoot(); ok {
		add(inspectPathRow{Name: "repo-graph", Purpose: "this repo's shared contribution ring", Path: kb.RepoContribPath(root), Scope: "repo", Owner: "kb.RepoContribPath", ReadBy: "graph recall; kb sources"})
	} else {
		add(inspectPathRow{Name: "repo-graph", Purpose: "this repo's shared contribution ring", Unknown: "not inside a git repo", Scope: "repo", Owner: "kb.RepoContribPath"})
	}
	return rows
}

// inspectSubdir applies a subdir accessor only when the store itself resolved;
// an empty store must stay empty ("unknown"), not become a relative path.
func inspectSubdir(f func(string) string, store string) string {
	if store == "" {
		return ""
	}
	return f(store)
}

func inspectScopeKind(repo bool) string {
	if repo {
		return "repo"
	}
	return "user"
}

func inspectCwd() string {
	d, err := os.Getwd()
	if err != nil {
		return ""
	}
	return d
}

// hintsDir is where the once-per-repo skill-hint markers live, under the
// skills store. Shared with maybeAdvertiseSkillHint so the path is spelled once.
func hintsDir() string {
	store := bashySkillsDir()
	if store == "" {
		return ""
	}
	return filepath.Join(store, "hints")
}

// collectInspectMode asks each gate's REAL decider and then explains the
// answer from the same signals the decider read. The explanation is derived,
// the decision is not — if the two ever disagree the decider wins and the row
// says so, which is the whole point of reporting decisions instead of env.
func collectInspectMode() []inspectModeRow {
	var rows []inspectModeRow

	// agentic: the master switch, read verbatim.
	agentic := weavecli.IsAgent()
	rows = append(rows, inspectModeRow{Gate: "agentic", On: agentic, DecidedBy: inspectEnvSignal("BASHY_AGENTIC", "unset")})

	// agent-driven: the machine-at-the-wheel question, by either route.
	driven := weavecli.IsAgentDriven()
	drivenBy := "default (no BASHY_AGENTIC, no harness marker)"
	if agentic {
		drivenBy = inspectEnvSignal("BASHY_AGENTIC", "")
	} else if tool, ok := fleet.DetectTool(); ok {
		drivenBy = "fleet.DetectTool → " + tool + inspectMarkerHit()
	}
	rows = append(rows, inspectModeRow{Gate: "agent-driven", On: driven, DecidedBy: drivenBy})

	killed := agenticDisabled()
	explain := func(name, env string, on bool) string {
		switch {
		case killed:
			return "BASHY_AGENTIC=" + os.Getenv("BASHY_AGENTIC") + " (master kill)"
		case strings.TrimSpace(os.Getenv(env)) != "":
			return inspectEnvSignal(env, "")
		case driven:
			return "default under agent-driven (" + drivenBy + ")"
		default:
			return "default: off for an interactive human"
		}
	}
	rows = append(rows,
		inspectModeRow{Gate: "advisor", On: advisorEnabled(), DecidedBy: explain("advisor", "BASHY_ADVISOR", advisorEnabled()), Evidence: defaultMemoryPath()},
		inspectModeRow{Gate: "hints", On: hintsEnabled(), DecidedBy: explain("hints", "BASHY_HINTS", hintsEnabled()), Evidence: hintsDir()},
		inspectModeRow{Gate: "exechist", On: execHistEnabled(), DecidedBy: explain("exechist", "BASHY_EXECHIST", execHistEnabled()), Evidence: execHistDir()},
	)

	learnBy := "advisor || hints (BASHY_LEARN unset)"
	if strings.EqualFold(strings.TrimSpace(os.Getenv("BASHY_LEARN")), "off") {
		learnBy = "BASHY_LEARN=off"
	}
	rows = append(rows, inspectModeRow{Gate: "learn", On: learnEnabled(), DecidedBy: learnBy, Evidence: craft.OpenFacts(craftStoreDir()).Path()})

	rows = append(rows, inspectModeRow{Gate: "audit", On: auditEnabled(), DecidedBy: inspectEnvSignal("BASHY_AUDIT", "unset"), Evidence: inspectIf(auditEnabled(), auditPath())})

	telBy := inspectEnvSignal("OTEL_TRACES_EXPORTER", "default (file spool)")
	if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); ep != "" {
		telBy += "; OTEL_EXPORTER_OTLP_ENDPOINT set"
	}
	rows = append(rows, inspectModeRow{Gate: "telemetry", On: telemetry.Enabled(), DecidedBy: telBy, Evidence: inspectIf(telemetry.Enabled(), telemetry.SpoolPath())})

	reduceOn := outputReductionEnabled(os.Environ())
	reduceBy := inspectEnvSignal("BASHY_OUTPUT_REDUCE", "default")
	switch {
	case strings.EqualFold(strings.TrimSpace(os.Getenv("VSC_PROFILE")), "cert"):
		reduceBy = "VSC_PROFILE=cert (certification run; every output request is overridden)"
	case noElideFlag:
		reduceBy = "--no-elide"
	case reduceFlag.set:
		reduceBy = fmt.Sprintf("--reduce=%v", reduceFlag.value)
	}
	rows = append(rows, inspectModeRow{Gate: "output-reduce", On: reduceOn, DecidedBy: reduceBy})

	rows = append(rows, inspectModeRow{Gate: "dryrun", On: dryRunRequested(), DecidedBy: inspectIfElse(dryRunRequested(), "--dryrun at startup", "default")})
	return rows
}

// inspectEnvSignal renders NAME=value, or the fallback when NAME is unset.
func inspectEnvSignal(name, unset string) string {
	v, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(v) == "" {
		return unset
	}
	return name + "=" + v
}

// inspectMarkerHit names the harness marker variables that are set — the WHY
// behind fleet.DetectTool, read from the same registry-derived list.
func inspectMarkerHit() string {
	var hit []string
	for _, env := range fleet.MarkerEnvs() {
		if v, ok := os.LookupEnv(env); ok && strings.TrimSpace(v) != "" {
			hit = append(hit, env)
		}
	}
	if len(hit) == 0 {
		return ""
	}
	sort.Strings(hit)
	return " (marker " + strings.Join(hit, ", ") + ")"
}

func inspectIf(cond bool, s string) string {
	if cond {
		return s
	}
	return ""
}

func inspectIfElse(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// inspectAtlasVerbs is the set of names the index may reference: everything
// bashy actually dispatches (builtins, the userland, the front-door verbs,
// and the hidden aliases), as `bashy commands --all` would list it.
func inspectAtlasVerbs() map[string]bool {
	out := map[string]bool{}
	for _, r := range liveAtlas(true) {
		out[r.Name] = true
	}
	return out
}
