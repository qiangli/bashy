// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const contextSchemaVersion = "bashy-context-v1"

type contextReport struct {
	SchemaVersion       string            `json:"schema_version"`
	BashyPath           string            `json:"bashy_path"`
	Argv0               string            `json:"argv0,omitempty"`
	CWD                 string            `json:"cwd"`
	InitialCWD          string            `json:"initial_cwd,omitempty"`
	ProjectRoot         string            `json:"project_root,omitempty"`
	WorkspaceMount      string            `json:"workspace_mount,omitempty"`
	Mode                contextMode       `json:"mode"`
	Runtime             contextRuntime    `json:"runtime"`
	Capabilities        contextCaps       `json:"capabilities"`
	RecommendedCommands []contextCommand  `json:"recommended_commands"`
	Notes               []string          `json:"notes,omitempty"`
	Environment         map[string]string `json:"environment,omitempty"`
}

type contextMode struct {
	Agentic bool `json:"agentic"`
	Advisor bool `json:"advisor"`
}

type contextRuntime struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Shell  string `json:"shell,omitempty"`
}

type contextCaps struct {
	DryRun            bool `json:"dry_run"`
	AgentJSONLines    bool `json:"agent_json_lines"`
	RunEnvelope       bool `json:"run_envelope"`
	CheckAgentJSON    bool `json:"check_agent_json"`
	CommandFeatures   bool `json:"command_features"`
	InProcessGit      bool `json:"in_process_git"`
	InProcessUserland bool `json:"in_process_userland"`
	// Code-knowledge graph (graph-impact/neighbors/hotspots/query): navigate a
	// repo's structure without a grep dance. Knowledge graph
	// (graph-note/recall/observe/pitfalls): a durable, shared per-repo "agentic
	// wiki" other agents' findings accrue into. Advertised here so agents discover
	// them on the first hop instead of re-deriving by search.
	CodeGraph      bool `json:"code_graph"`
	KnowledgeGraph bool `json:"knowledge_graph"`
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
		Runtime: contextRuntime{
			GOOS:   runtime.GOOS,
			GOARCH: runtime.GOARCH,
			Shell:  os.Getenv("SHELL"),
		},
		Capabilities: contextCaps{
			DryRun:            true,
			AgentJSONLines:    true,
			RunEnvelope:       true,
			CheckAgentJSON:    true,
			CommandFeatures:   true,
			InProcessGit:      true,
			InProcessUserland: true,
			CodeGraph:         true,
			KnowledgeGraph:    true,
		},
		RecommendedCommands: []contextCommand{
			{Purpose: "preview destructive script safely", Command: bashyPath + " --dry-run SCRIPT"},
			{Purpose: "agent-readable dry-run manifest", Command: "BASHY_AGENTIC=1 " + bashyPath + " --dry-run SCRIPT"},
			{Purpose: "script preflight", Command: bashyPath + " check --agent --script SCRIPT"},
			{Purpose: "preflight plus captured run envelope", Command: bashyPath + " run --check --capture -- SCRIPT"},
			{Purpose: "one command capability lookup", Command: bashyPath + " commands COMMAND --features"},
			{Purpose: "what code is coupled to a symbol (skip the grep dance)", Command: bashyPath + " graph-impact SYMBOL"},
			{Purpose: "recall/leave shared repo knowledge for other agents", Command: bashyPath + " graph-recall QUERY"},
		},
		Notes: []string{
			"This record is intended to replace ad hoc probes such as env, uname, file, and bashy --help.",
			"Use the reported bashy_path for explicit bashy feature calls.",
		},
		Environment: map[string]string{},
	}
	if len(os.Args) > 0 {
		report.Argv0 = os.Args[0]
	}
	for _, name := range []string{"BASHY_AGENTIC", "BASHY_ADVISOR", "DHNT_AGENT"} {
		if v, ok := os.LookupEnv(name); ok {
			report.Environment[name] = v
		}
	}
	if len(report.Environment) == 0 {
		report.Environment = nil
	}
	return report
}

func printContextPlain(r contextReport) {
	fmt.Printf("bashy_path=%s\n", r.BashyPath)
	fmt.Printf("cwd=%s\n", r.CWD)
	fmt.Printf("agentic=%t advisor=%t\n", r.Mode.Agentic, r.Mode.Advisor)
	for _, c := range r.RecommendedCommands {
		fmt.Printf("%s: %s\n", c.Purpose, c.Command)
	}
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
