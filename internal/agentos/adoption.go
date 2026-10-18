package agentos

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// TriggerRow is one entry of the generated trigger-word table. The table is
// generated from CommonOptionsVocabulary plus the lens and adapt declarations
// below and projected into every adoption channel (skill description, lexicon,
// help footer, MCP schema, declared nudge rows). Channel text is never
// hand-copied; tests compare the static projections (SKILL.md, embedded
// docs) against these rows.
type TriggerRow struct {
	Intent  string // what the agent wants to do
	Trigger string // the trigger word / flag the agent types
	Word    string // short form used in the skill description
	Gloss   string // one-phrase meaning used in the skill description
	Prior   string // where it already lives in the agent's prior (clig.dev etc)
	Example string // canonical bashy spelling
}

// CommonOption is one row of the closed long-only vocabulary in
// docs/common-options.md. Rows with an Intent also become trigger rows.
type CommonOption struct {
	Option  string // "--dry-run"
	Meaning string
	Intent  string
	Prior   string
	Example string
	Gloss   string
	// Engine reports whether applyCommonOptions handles the option for any
	// bashy-owned command lacking a native spelling.
	Engine bool
}

// Key is the MCP/YAML spelling (dry-run -> dry_run).
func (o CommonOption) Key() string { return OptionKey(strings.TrimPrefix(o.Option, "--")) }

// CommonOptionsVocabulary is the single source of truth for the option rows.
var CommonOptionsVocabulary = []CommonOption{
	{Option: "--json", Meaning: "machine output: one JSON value, or NDJSON for streams", Intent: "parse output", Prior: "gh, kubectl, docker --format json", Example: "bashy adapt --json -- ps aux | jq", Gloss: "machine output"},
	{Option: "--fields", Meaning: "project JSON records to named fields", Intent: "keep only some fields", Prior: "gh --json a,b, jq", Example: "bashy adapt --json --fields=pid,cmd -- ps aux", Gloss: "project fields"},
	{Option: "--null", Meaning: "NUL-separated records", Intent: "NUL-safe lists", Prior: "find -print0, xargs -0", Example: "find . -print0 | xargs -0", Gloss: "NUL-safe", Engine: true},
	{Option: "--dry-run", Meaning: "report what would change; change nothing", Intent: "see what would change", Prior: "make -n, rsync -n, kubectl --dry-run", Example: "rm -r build --dry-run", Gloss: "preview"},
	{Option: "--no-input", Meaning: "never prompt; fail where a prompt would happen", Intent: "avoid hanging on a prompt", Prior: "clig.dev, CI flags", Example: "bashy adapt --no-input -- apt install jq", Gloss: "never prompt", Engine: true},
	{Option: "--explain", Meaning: "describe each flag and operand; never run", Intent: "understand a command before running it", Prior: "explainshell", Example: "rsync -a src/ dst/ --explain", Gloss: "describe before running", Engine: true},
	{Option: "--depth", Meaning: "bound a tree walk"},
	{Option: "--since", Meaning: "filter changes after cursor envelope (bashy-cursor-v1)"},
}

// lensTriggerDecls and adaptTriggerDecls mirror the lens row declarations and
// the adapter front door; skills/bashy/commands/{lens,adapt}.md must mention
// every Trigger (checked by TestAdoptionProjectionsMatchSource).
var lensTriggerDecls = []TriggerRow{
	{Intent: "read a pdf/docx/html/csv", Trigger: "lens -t md FILE", Word: "lens -t md", Gloss: "read pdf/docx/html/csv", Prior: "markitdown, pdftotext", Example: "lens -t md report.pdf"},
	{Intent: "convert json to yaml and back", Trigger: "lens -f json -t yaml", Word: "lens -f json -t yaml", Gloss: "json\u2194yaml", Prior: "yq, python yaml", Example: "lens -f json -t yaml < cfg.json"},
	{Intent: "list an archive or container", Trigger: "lens -t listing FILE", Word: "lens -t listing", Gloss: "list archive/container", Prior: "tar -t, unzip -l", Example: "lens -t listing release.zip"},
	{Intent: "show a command for PowerShell, cmd, MCP or a skill", Trigger: "lens -f bash -t pwsh", Word: "lens -f bash -t pwsh", Gloss: "render for PowerShell/cmd/MCP/skill", Prior: "— (new, taught by hint)", Example: "lens -f bash -t pwsh --switch=pascal"},
	{Intent: "read file at a commit without checkout", Trigger: "lens -t md git:REV:PATH", Word: "lens -t md git:REV:PATH", Gloss: "file at a commit", Prior: "git show", Example: "lens -t md git:v1.0:docs/INSTALL.md"},
}

var adaptTriggerDecls = []TriggerRow{
	{Intent: "use any common option on a foreign command (git, tar, apt, docker\u2026)", Trigger: "bashy adapt --OPTION -- CMD", Word: "bashy adapt --OPTION -- CMD", Gloss: "common options for any foreign command", Prior: "— (new, taught by hint)", Example: "bashy adapt --json -- git status"},
}

// TriggerRows generates the trigger-word table from the vocabulary and the
// lens/adapt declarations.
func TriggerRows() []TriggerRow {
	var rows []TriggerRow
	for _, o := range CommonOptionsVocabulary {
		if o.Intent == "" {
			continue
		}
		rows = append(rows, TriggerRow{Intent: o.Intent, Trigger: o.Option, Word: o.Option, Gloss: o.Gloss, Prior: o.Prior, Example: o.Example})
	}
	rows = append(rows, lensTriggerDecls...)
	return append(rows, adaptTriggerDecls...)
}

const skillDescriptionBase = "Drive bashy, the agentic shell \u2014 a drop-in Bash 5.3 with agent-native extensions. Use whenever bashy is the shell you are running in (or is on PATH) to read what this host ALREADY KNOWS about your task before starting it, see what other agents and humans are saying, know what work is owed here, cut probing round-trips, preview destructive commands, run with structured result envelopes, navigate code without grep dances, and use environment-gated verified skills."

const skillDescriptionTail = " Open every session with `bashy inspect context --json`, then `bashy kb search \"<your task>\"`, `bashy inbox`, and `bashy todo list`; close with `bashy kb retro`."

// SkillDescription is the SKILL.md description: the fixed base, the generated
// trigger words, and the session-opening tail.
func SkillDescription() string {
	var words []string
	for _, r := range TriggerRows() {
		words = append(words, fmt.Sprintf("%s (%s)", r.Word, r.Gloss))
	}
	return skillDescriptionBase + " Trigger words: " + strings.Join(words, ", ") + "." + skillDescriptionTail
}

// Common-option support statuses in the help footer.
const (
	optSupported = "supported" // the command declares the flag itself
	optMapped    = "mapped"    // handled by the engine or a native spelling
	optMissing   = "missing"   // unavailable natively; use bashy adapt
)

// helpOptionStatus classifies one vocabulary option for a command whose
// declared long flag names are in declared.
func helpOptionStatus(o CommonOption, declared map[string]bool) string {
	name := strings.TrimPrefix(o.Option, "--")
	if declared[name] {
		return optSupported
	}
	if o.Option == "--null" && (declared["zero"] || declared["print0"]) {
		return optMapped
	}
	if o.Engine && o.Option != "--null" {
		return optMapped
	}
	return optMissing
}

// HelpCommonOptionsFooter is the "Common options" line of every skill-format
// --help: per-option status for this command (declared holds its long flag
// names, e.g. from its derived schema) plus the adapt route for the rest.
func HelpCommonOptionsFooter(commandPath string, declared map[string]bool) string {
	var parts []string
	var missing bool
	for _, o := range CommonOptionsVocabulary {
		st := helpOptionStatus(o, declared)
		missing = missing || st == optMissing
		parts = append(parts, o.Option+"="+st)
	}
	line := "Common options for " + commandPath + ": " + strings.Join(parts, " ") + "."
	if missing {
		line += " Missing options exit 2 here; for a foreign command or a missing option use `bashy adapt --OPTION -- CMD`."
	}
	return line + " See `bashy help common-options`."
}

// declaredFlagSet is the set of long flag names a derived schema declares.
func declaredFlagSet(schema tool.ArgSchema) map[string]bool {
	declared := map[string]bool{}
	for _, f := range schema.Flags {
		declared[f.Name] = true
	}
	return declared
}

// MCPCommonOptionsNote is the compact per-command common-options status for MCP
// tool descriptions. MCP verbs take an argv array, so the note names argv
// spellings rather than inventing structured properties the transport would
// ignore. Server discovery and `help --format=mcp` share it.
func MCPCommonOptionsNote(schema tool.ArgSchema) string {
	declared := declaredFlagSet(schema)
	var parts []string
	for _, o := range CommonOptionsVocabulary {
		parts = append(parts, o.Option+"="+helpOptionStatus(o, declared))
	}
	return "Common options (argv; missing: bashy adapt --OPTION -- CMD): " + strings.Join(parts, " ") + "."
}

// LexiconTerms returns the adoption terms for the lexicon (18-term cap): lens,
// adapt and the vocabulary's trigger options.
func LexiconTerms() []string {
	terms := []string{"lens", "adapt"}
	for _, o := range CommonOptionsVocabulary {
		terms = append(terms, o.Option)
	}
	return terms
}
