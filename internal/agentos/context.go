// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"crypto/fips140"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/coreutils/pkg/chat"
	"github.com/qiangli/coreutils/pkg/handoff"
	coreskills "github.com/qiangli/coreutils/pkg/skills"
)

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

const contextSchemaVersion = "bashy-context-v1"

type contextReport struct {
	SchemaVersion       string           `json:"schema_version"`
	BashyPath           string           `json:"bashy_path"`
	Argv0               string           `json:"argv0,omitempty"`
	CWD                 string           `json:"cwd"`
	InitialCWD          string           `json:"initial_cwd,omitempty"`
	ProjectRoot         string           `json:"project_root,omitempty"`
	WorkspaceMount      string           `json:"workspace_mount,omitempty"`
	Mode                contextMode      `json:"mode"`
	Runtime             contextRuntime   `json:"runtime"`
	System              contextSystem    `json:"system"`
	Capabilities        contextCaps      `json:"capabilities"`
	RecommendedCommands []contextCommand `json:"recommended_commands"`
	// PendingHandoffs is work someone left for whoever comes next — possibly a
	// different tool, possibly a different machine, possibly a teammate.
	//
	// This is the discoverability half of `bashy handoff`, and it is the half that
	// has to be right. `resume` runs COLD: the agent invoking it has no memory of
	// the handoff, was very likely not the tool that wrote it, and cannot be
	// TOLD to look — nobody is there to tell it. So the pending work must surface
	// on the FIRST call an agent makes, which is this one. A handoff that nobody
	// finds is a handoff that did not happen.
	PendingHandoffs []contextHandoff `json:"pending_handoffs,omitempty"`
	Notes           []string         `json:"notes,omitempty"`
	// Tools maps common CLI names to their resolved PATH location — replaces
	// per-tool `which X` probes. Only tools actually found are listed.
	Tools map[string]string `json:"tools,omitempty"`
	// Environment holds only the vars whose VALUE is safe to show (curated
	// allowlist: paths/locale/shell/toolchain). Every other var's value is hidden;
	// their NAMES go in EnvironmentRedacted as a single comma-joined list — far
	// cheaper in tokens than a per-var "<redacted>" map — so an agent still sees
	// which vars (including secrets) are SET, without their values.
	Environment         map[string]string `json:"environment,omitempty"`
	EnvironmentRedacted string            `json:"environment_redacted,omitempty"`
	// Skills is the L1 progressive-disclosure surface of the env-gated skill
	// catalog: only skills applicable at THIS host's coordinate, name +
	// one-liner (bodies via `bashy skills show`). Verified = the skill
	// carries a machine-checkable contract (dhnt dual bundle).
	Skills []coreskills.Advertised `json:"skills,omitempty"`
}

type contextMode struct {
	Agentic bool `json:"agentic"`
	Advisor bool `json:"advisor"`
	// Agent is the detected driving agent ("claude", "codex", …) from
	// the env markers each agentic tool sets; empty when none detected.
	Agent string `json:"agent,omitempty"`
}

type contextRuntime struct {
	GOOS    string `json:"goos"`
	GOARCH  string `json:"goarch"`
	NumCPU  int    `json:"num_cpu"`
	Shell   string `json:"shell,omitempty"`
	Build   string `json:"build,omitempty"`
	FIPS140 bool   `json:"fips140"` // the validated crypto module is active (built GOFIPS140 and/or GODEBUG=fips140=on)
}

// contextSystem is the uname/hostname/identity block — replaces `uname -a`,
// `hostname`, `id`, and container-detection probes.
type contextSystem struct {
	Hostname      string `json:"hostname,omitempty"`
	Sysname       string `json:"sysname,omitempty"`        // uname -s (Darwin/Linux/…)
	KernelRelease string `json:"kernel_release,omitempty"` // uname -r
	KernelVersion string `json:"kernel_version,omitempty"` // uname -v
	Machine       string `json:"machine,omitempty"`        // uname -m
	User          string `json:"user,omitempty"`
	UID           int    `json:"uid"`
	EUID          int    `json:"euid"`
	IsRoot        bool   `json:"is_root"`
	Home          string `json:"home,omitempty"`
	TempDir       string `json:"temp_dir,omitempty"`
	StdinTTY      bool   `json:"stdin_tty"`
	Container     string `json:"container,omitempty"` // docker|podman|kubernetes|""
}

type contextCaps struct {
	DryRun            bool `json:"dry_run"`
	AgentJSONLines    bool `json:"agent_json_lines"`
	RunEnvelope       bool `json:"run_envelope"`
	CheckAgentJSON    bool `json:"check_agent_json"`
	CommandFeatures   bool `json:"command_features"`
	InProcessGit      bool `json:"in_process_git"`
	InProcessUserland bool `json:"in_process_userland"`
	// Code-knowledge graph (graph impact/neighbors/hotspots/query): navigate a
	// repo's structure without a grep dance. Knowledge graph
	// (graph note/recall/observe/pitfalls): a durable, shared per-repo "agentic
	// wiki" other agents' findings accrue into. Advertised here so agents discover
	// them on the first hop instead of re-deriving by search.
	CodeGraph      bool `json:"code_graph"`
	KnowledgeGraph bool `json:"knowledge_graph"`
	// The three DURABLE STORES an agent inherits and leaves behind: what is
	// KNOWN (kb), what is being SAID (mb), and what must be DONE (todo).
	//
	// They are grouped because they are one idea. Everything else bashy offers
	// helps an agent DO the work; these three are the state the work happens in,
	// and they are the only things that outlive a session. An agent that never
	// touches them begins every session from nothing and leaves nothing for the
	// next one — which four days of telemetry showed is exactly what happened:
	// 61,084 dispatched commands, and one attempt to read the store.
	//
	// KnowledgeBase (kb search/recall/add/retro) is the host-shared, cross-repo
	// memory: distilled pages agents wrote for whoever came next, plus the
	// cross-ring read surface over them. Advertised separately from
	// knowledge_graph because they are different stores — the graph is per-repo
	// and contribution-shaped, kb is per-host and claim-shaped.
	//
	// MessageBoard (mb / mb post / mb send) is the host's shared append-only
	// board: how an agent reaches a human or a peer mid-task, and how it finds
	// out something was said to it.
	//
	// Todo (todo list/add/start/done) is the work list, scoped automatically the
	// way kb is — the repo's when in one, the host's otherwise.
	KnowledgeBase bool `json:"knowledge_base"`
	MessageBoard  bool `json:"message_board"`
	Todo          bool `json:"todo"`
	// CoachReflex: the LLM-free loop-detection reflex is auto-attached to every
	// weave/invoke/delegate run (off with BASHY_NO_COACH). Advertised so an agent
	// knows its delegated work is loop-protected without invoking anything — the
	// coach is a property of delegation, not a verb to remember.
	CoachReflex bool `json:"coach_reflex"`
}

// contextHandoff is a pending handoff, summarised. Deliberately a SUMMARY, not
// the record: an agent orienting itself needs to know that work is waiting, who
// left it and what it was — not to have a full diff dumped into its context
// window. `bashy resume` reads the record.
type contextHandoff struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	CreatedAt  string `json:"created_at"`
	Brief      string `json:"brief"`
	NextAction string `json:"next_action,omitempty"`
	Resume     string `json:"resume"` // the exact command to continue it
}

type contextCommand struct {
	Purpose string `json:"purpose"`
	Command string `json:"command"`
}

func dispatchContext(args []string) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--plain":
			asJSON = false
		case "-h", "--help":
			fmt.Println("usage: bashy context [--json|--plain]")
			fmt.Println("Print one first-hop environment/discovery record for agents.")
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "context: unknown option %q\n", a)
				return 2
			}
			fmt.Fprintf(os.Stderr, "context: unexpected argument %q\n", a)
			return 2
		}
	}
	if !asJSON {
		printContextPlain(collectContext())
		return 0
	}
	b, err := json.Marshal(collectContext())
	if err != nil {
		fmt.Fprintln(os.Stderr, "context:", err)
		return 1
	}
	fmt.Println(string(b))
	return 0
}

func collectContext() contextReport {
	cwd, _ := os.Getwd()
	bashyPath := bashySelfPath()
	if abs, err := filepath.Abs(bashyPath); err == nil {
		bashyPath = abs
	}
	initialCWD := firstNonEmpty(os.Getenv("BASHY_EVAL_INITIAL_CWD"), cwd)
	projectRoot := firstNonEmpty(os.Getenv("BASHY_EVAL_PROJECT_ROOT"), detectProjectRoot(cwd))
	workspaceMount := os.Getenv("BASHY_EVAL_WORKSPACE")
	report := contextReport{
		SchemaVersion:  contextSchemaVersion,
		BashyPath:      bashyPath,
		CWD:            cwd,
		InitialCWD:     initialCWD,
		ProjectRoot:    projectRoot,
		WorkspaceMount: workspaceMount,
		Mode: contextMode{
			Agentic: envTruthy("BASHY_AGENTIC") || envTruthy("DHNT_AGENT"),
			Advisor: envTruthy("BASHY_ADVISOR"),
		},
	}
	if agent, ok := coreskills.DetectAgent(); ok {
		report.Mode.Agent = agent
	}
	report = fillContext(report, bashyPath)
	report.Skills = coreskills.Applicable(skillsOptions()...)
	report.Environment, report.EnvironmentRedacted = collectEnvironment()
	if len(os.Args) > 0 {
		report.Argv0 = os.Args[0]
	}
	return report
}

// fillContext completes the static sections of the report.
func fillContext(report contextReport, bashyPath string) contextReport {
	report.Runtime = contextRuntime{
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
		NumCPU:  runtime.NumCPU(),
		Shell:   os.Getenv("SHELL"),
		Build:   cli.BuildID(),
		FIPS140: fips140.Enabled(),
	}
	report.System = collectSystem()
	report.Capabilities = contextCaps{
		DryRun:            true,
		AgentJSONLines:    true,
		RunEnvelope:       true,
		CheckAgentJSON:    true,
		CommandFeatures:   true,
		InProcessGit:      true,
		InProcessUserland: true,
		CodeGraph:         true,
		KnowledgeGraph:    true,
		KnowledgeBase:     true,
		MessageBoard:      true,
		Todo:              true,
		CoachReflex:       chat.ReflexEnabled(),
	}
	report.RecommendedCommands = []contextCommand{
		// kb leads this list deliberately, and the reason is measured rather
		// than asserted. Four days of telemetry: 61,084 dispatched commands, 36
		// kb pages holding knowledge whose trigger situations demonstrably
		// arose (8 of 10 probed pages), and exactly ONE agent reaching for the
		// store unprompted — with a verb that did not exist, so it exited 1 and
		// never tried again. Successful agent uses: zero.
		//
		// The cause was not the store; it was this list. `context --json` is
		// documented as the FIRST call an agent makes, it recommended eight
		// commands, and not one of them mentioned kb. An agent that did exactly
		// what bashy told it to do was never told the memory existed.
		//
		// So: first entry, both halves of the loop, and the read comes before
		// the write because that is the order the work happens in.
		// kb / mb / todo lead this list as a SET, and the grouping is the point:
		// they are the three things that do not survive a session boundary —
		// what is KNOWN, what is being SAID, and what must be DONE. Every other
		// verb here helps do the work; these three are the state the work
		// happens in, and an agent that never touches them starts every session
		// from nothing and leaves nothing behind.
		{Purpose: "WHAT IS ALREADY KNOWN about this task — run BEFORE starting; other agents on this host left it for you", Command: bashyPath + " kb search QUERY"},
		{Purpose: "same question across EVERY memory ring (kb + capabilities), one envelope", Command: bashyPath + " kb recall QUERY"},
		{Purpose: "write back what THIS task taught — run AFTER finishing; validate/correct what you consulted, never blind-append", Command: bashyPath + " kb retro"},
		{Purpose: "WHAT IS BEING SAID on this host — the shared message board every agent and human reads; check for messages addressed to you", Command: bashyPath + " mb"},
		{Purpose: "say something to everyone (or one agent: mb send AGENT ...) — how you reach a human or a peer mid-task", Command: bashyPath + " mb post MESSAGE"},
		{Purpose: "WHAT MUST BE DONE here — the work list, repo-scoped automatically; read it before deciding what to do next", Command: bashyPath + " todo list"},
		{Purpose: "record work so it outlives this session (todo start/done to move it)", Command: bashyPath + " todo add TITLE"},
		{Purpose: "preview destructive script safely", Command: bashyPath + " --dry-run SCRIPT"},
		{Purpose: "agent-readable dry-run manifest", Command: "BASHY_AGENTIC=1 " + bashyPath + " --dry-run SCRIPT"},
		{Purpose: "script preflight", Command: bashyPath + " check --agent --script SCRIPT"},
		{Purpose: "preflight plus captured run envelope", Command: bashyPath + " run --check --capture -- SCRIPT"},
		{Purpose: "one command capability lookup", Command: bashyPath + " commands COMMAND --features"},
		{Purpose: "what code is coupled to a symbol (skip the grep dance)", Command: bashyPath + " graph impact SYMBOL"},
		{Purpose: "recall/leave shared repo knowledge for other agents", Command: bashyPath + " graph recall QUERY"},
		{Purpose: "skills applicable on this host (read one: skills show NAME; run attested: skills run NAME)", Command: bashyPath + " skills list"},
	}
	report.Notes = []string{
		"Replaces first-hop probes: `system` = uname/hostname/id, `tools` = which/tool discovery, `environment` = env — an agent need not run env/uname/hostname/id/which itself.",
		"environment shows values only for safe names (paths/locale/shell/toolchain); environment_redacted is a comma-joined list of the remaining var NAMES (values hidden) — a secret's existence is visible, not its value.",
		"Use the reported bashy_path for explicit bashy feature calls.",
	}
	report.PendingHandoffs = pendingHandoffs(report.ProjectRoot, report.CWD, bashyPath)
	if len(report.PendingHandoffs) > 0 {
		report.Notes = append(report.Notes,
			fmt.Sprintf("%d pending handoff(s) for this project: someone left work for whoever came next. "+
				"Read pending_handoffs and continue with the `resume` command shown there BEFORE starting anything new — "+
				"they may have been mid-way through the very thing you are about to begin.",
				len(report.PendingHandoffs)))
		report.RecommendedCommands = append(report.RecommendedCommands, contextCommand{
			Purpose: "continue work someone handed off (any tool, any machine)",
			Command: bashyPath + " resume",
		})
	}
	report.Tools = detectTools()
	return report
}

// pendingHandoffs finds work left for a successor. It is deliberately
// FAIL-QUIET: a broken or unreadable handoff store must never stop `bashy
// context` from answering, because context is the first call an agent makes and
// breaking it strands every tool.
func pendingHandoffs(projectRoot, cwd, bashyPath string) []contextHandoff {
	root := projectRoot
	if root == "" {
		root = cwd
	}
	recs, err := handoff.Pending(handoff.DefaultDir(), handoff.ProjectRoots(root))
	if err != nil || len(recs) == 0 {
		return nil
	}
	out := make([]contextHandoff, 0, len(recs))
	for _, r := range recs {
		from := r.From.Name
		if from == "" {
			from = string(r.From.Kind)
		}
		if r.From.Host != "" {
			from = strings.TrimSpace(from + "@" + r.From.Host)
		}
		out = append(out, contextHandoff{
			ID:         r.ID,
			From:       from,
			CreatedAt:  r.CreatedAt.Format(time.RFC3339),
			Brief:      firstParagraph(r.Continuity),
			NextAction: r.NextAction,
			Resume:     bashyPath + " resume " + r.ID,
		})
	}
	return out
}

// firstParagraph keeps the orientation cheap: enough to know what is waiting,
// not so much that reading `context --json` costs what reading the record would.
func firstParagraph(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n\n"); i > 0 {
		s = s[:i]
	}
	if len(s) > 280 {
		s = s[:277] + "..."
	}
	return s
}

// collectSystem gathers uname/hostname/identity info (replacing uname -a,
// hostname, id, container probes).
func collectSystem() contextSystem {
	sysname, release, version, machine := unameInfo()
	if sysname == "" {
		sysname = strings.Title(runtime.GOOS) // windows fallback (no uname)
	}
	if machine == "" {
		machine = runtime.GOARCH
	}
	host, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	return contextSystem{
		Hostname:      host,
		Sysname:       sysname,
		KernelRelease: release,
		KernelVersion: version,
		Machine:       machine,
		User:          firstNonEmpty(os.Getenv("USER"), os.Getenv("LOGNAME")),
		UID:           os.Getuid(),
		EUID:          os.Geteuid(),
		IsRoot:        os.Geteuid() == 0,
		Home:          home,
		TempDir:       os.TempDir(),
		StdinTTY:      isTerminal(os.Stdin),
		Container:     detectContainer(),
	}
}

// detectTools resolves the location of common CLIs so agents skip `which X`.
func detectTools() map[string]string {
	names := []string{
		"git", "go", "python3", "python", "node", "npm", "pnpm", "yarn", "bun",
		"docker", "podman", "kubectl", "make", "cmake", "cc", "gcc", "clang",
		"rustc", "cargo", "java", "ruby", "jq", "rg", "fd", "gh", "curl", "wget",
		"brew", "bash", "zsh", "fish", "tmux",
	}
	out := map[string]string{}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			out[n] = p
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func detectContainer() string {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "kubernetes"
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return "podman"
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	return ""
}

// collectEnvironment splits the environment into (shown, redacted): shown maps
// each safe var to its value; redacted is a sorted, comma-joined list of the
// NAMES whose values are hidden — one compact string instead of a per-var
// "<redacted>" entry, so the token cost is a name list, not a padded map.
func collectEnvironment() (map[string]string, string) {
	shown := map[string]string{}
	var redacted []string
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		name, val := kv[:i], kv[i+1:]
		if envValueSafe(name) {
			shown[name] = val
		} else {
			redacted = append(redacted, name)
		}
	}
	sort.Strings(redacted)
	if len(shown) == 0 {
		shown = nil
	}
	return shown, strings.Join(redacted, ",")
}

func envValueSafe(name string) bool {
	if envLooksSensitive(name) {
		return false
	}
	return safeEnvNames[strings.ToUpper(name)]
}

// envLooksSensitive is a defense-in-depth denylist: even if a name were added to
// the allowlist, a secret-shaped name is never shown.
func envLooksSensitive(name string) bool {
	u := strings.ToUpper(name)
	for _, p := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "PRIVATE", "SIGNING", "APIKEY", "PASSPHRASE"} {
		if strings.Contains(u, p) {
			return true
		}
	}
	return false
}

// safeEnvNames: env vars whose VALUE is safe to show (non-secret, useful for an
// agent to orient) — paths, locale, shell, terminal, toolchain roots. Everything
// else is redacted. Matched case-insensitively.
var safeEnvNames = map[string]bool{
	// identity / shell / dirs
	"HOME": true, "USER": true, "LOGNAME": true, "SHELL": true, "SHLVL": true,
	"PWD": true, "OLDPWD": true, "TMPDIR": true, "HOSTNAME": true, "HOST": true,
	"COMPUTERNAME": true, "OS": true,
	// locale
	"LANG": true, "LANGUAGE": true, "TZ": true, "LC_ALL": true, "LC_CTYPE": true,
	"LC_MESSAGES": true, "LC_COLLATE": true, "LC_NUMERIC": true, "LC_TIME": true,
	"LC_MONETARY": true,
	// search paths
	"PATH": true, "MANPATH": true, "INFOPATH": true, "FPATH": true,
	// terminal / editor prefs
	"TERM": true, "TERM_PROGRAM": true, "TERM_PROGRAM_VERSION": true, "COLORTERM": true,
	"LINES": true, "COLUMNS": true, "EDITOR": true, "VISUAL": true, "PAGER": true, "MANPAGER": true,
	// toolchain roots (non-secret)
	"GOPATH": true, "GOROOT": true, "GOBIN": true, "GOOS": true, "GOARCH": true,
	"GOMODCACHE": true, "GOCACHE": true, "GO111MODULE": true, "CGO_ENABLED": true,
	"CARGO_HOME": true, "RUSTUP_HOME": true, "JAVA_HOME": true, "ANDROID_HOME": true,
	"PYENV_ROOT": true, "RBENV_ROOT": true, "NVM_DIR": true, "NODE_ENV": true,
	"VIRTUAL_ENV": true, "CONDA_PREFIX": true, "CONDA_DEFAULT_ENV": true,
	// xdg dirs
	"XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true, "XDG_DATA_HOME": true,
	"XDG_STATE_HOME": true, "XDG_RUNTIME_DIR": true,
	// display / arch
	"DISPLAY": true, "WAYLAND_DISPLAY": true, "PROCESSOR_ARCHITECTURE": true,
	"NUMBER_OF_PROCESSORS": true,
	// bashy/dhnt own (non-secret markers)
	"BASHY_AGENTIC": true, "BASHY_ADVISOR": true, "BASHY_SESSION": true,
	"BASHY_EPISODE": true, "BASHY_AGENT": true, "BASHY_AGENT_ID": true, "DHNT_AGENT": true,
}

func printContextPlain(r contextReport) {
	s := r.System
	fmt.Printf("bashy_path=%s\n", r.BashyPath)
	fmt.Printf("cwd=%s\n", r.CWD)
	fmt.Printf("host=%s  %s %s (%s)  cpus=%d\n", s.Hostname, s.Sysname, s.KernelRelease, s.Machine, r.Runtime.NumCPU)
	fmt.Printf("user=%s uid=%d euid=%d root=%t  shell=%s  tty=%t%s\n",
		s.User, s.UID, s.EUID, s.IsRoot, r.Runtime.Shell, s.StdinTTY, containerSuffix(s.Container))
	fmt.Printf("agentic=%t advisor=%t\n", r.Mode.Agentic, r.Mode.Advisor)
	if len(r.Tools) > 0 {
		fmt.Printf("tools: %s\n", strings.Join(sortedKeys(r.Tools), " "))
	}
	if len(r.Skills) > 0 {
		fmt.Println("skills (applicable here; read: bashy skills show NAME):")
		for _, s := range r.Skills {
			mark := ""
			if s.Verified {
				mark = " [verified]"
			}
			fmt.Printf("  %s%s: %s\n", s.Name, mark, s.Description)
		}
	}
	fmt.Println("recommended:")
	for _, c := range r.RecommendedCommands {
		fmt.Printf("  %s: %s\n", c.Purpose, c.Command)
	}
	if len(r.Environment) > 0 {
		fmt.Printf("environment (%d shown; real value for safe names):\n", len(r.Environment))
		for _, k := range sortedKeys(r.Environment) {
			fmt.Printf("  %s=%s\n", k, r.Environment[k])
		}
	}
	if r.EnvironmentRedacted != "" {
		n := strings.Count(r.EnvironmentRedacted, ",") + 1
		fmt.Printf("environment_redacted (%d, values hidden): %s\n", n, r.EnvironmentRedacted)
	}
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func containerSuffix(c string) string {
	if c == "" {
		return ""
	}
	return "  container=" + c
}

func envTruthy(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v != "" && v != "0" && v != "false" && v != "no" && v != "off"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func detectProjectRoot(cwd string) string {
	for dir := cwd; dir != "" && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		for _, marker := range []string{".git", "go.mod", "README.md", ".benchmark"} {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
	}
	return ""
}
