// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/coreutils/tool"
)

const checkSchemaVersion = "bashy-check-v1"

type checkOptions struct {
	mode           string
	json           bool
	strictSystem   bool
	allowContainer bool
	noSource       bool
	sourceRoot     string
	maxDepth       int
}

type checkReport struct {
	SchemaVersion string                 `json:"schema_version"`
	Mode          string                 `json:"mode"`
	Summary       checkSummary           `json:"summary"`
	Files         []checkFile            `json:"files"`
	Inventory     checkInventory         `json:"inventory"`
	Diagnostics   []checkDiagnostic      `json:"diagnostics"`
	ByFile        map[string][]checkDiag `json:"-"`
}

type checkSummary struct {
	Commands      int `json:"commands"`
	FilesAnalyzed int `json:"files_analyzed"`
	BashyNative   int `json:"bashy_native"`
	Container     int `json:"container"`
	System        int `json:"system"`
	NotFound      int `json:"not_found"`
	Dynamic       int `json:"dynamic"`
	Errors        int `json:"errors"`
	Warnings      int `json:"warnings"`
}

type checkFile struct {
	Path string `json:"path"`
	Role string `json:"role"`
	From string `json:"from,omitempty"`
}

type checkInventory struct {
	BashyNative []checkInvItem `json:"bashy_native,omitempty"`
	System      []checkInvItem `json:"system,omitempty"`
	Container   []checkInvItem `json:"container,omitempty"`
	NotFound    []checkInvItem `json:"not_found,omitempty"`
	Dynamic     []checkInvItem `json:"dynamic,omitempty"`
	Scripts     []checkInvItem `json:"scripts,omitempty"`
}

type checkInvItem struct {
	Name  string `json:"name,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Path  string `json:"path,omitempty"`
	Image string `json:"image,omitempty"`
	Span  string `json:"span,omitempty"`
	Text  string `json:"text,omitempty"`
}

type checkDiagnostic struct {
	Code     string `json:"code"`
	Level    string `json:"level"`
	File     string `json:"file,omitempty"`
	Line     uint   `json:"line,omitempty"`
	Column   uint   `json:"column,omitempty"`
	Command  string `json:"command,omitempty"`
	Message  string `json:"message"`
	Resolver string `json:"resolver,omitempty"`
	Path     string `json:"path,omitempty"`
}

type checkDiag struct {
	checkDiagnostic
	sortKey string
}

type checkAnalyzer struct {
	opts       checkOptions
	report     checkReport
	seen       map[string]bool
	functions  map[string]bool
	builtins   map[string]bool
	coreutils  map[string]bool
	verbs      map[string]bool
	gnuCore    map[string]bool
	invSeen    map[string]bool
	sourceRoot string
}

func dispatchCheck(args []string) int {
	opts := checkOptions{mode: "bashy", maxDepth: 8}
	var scripts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			opts.json = true
		case a == "--strict-system":
			opts.strictSystem = true
		case a == "--allow-container":
			opts.allowContainer = true
		case a == "--no-source":
			opts.noSource = true
		case a == "-h" || a == "--help":
			printCheckUsage(os.Stdout)
			return 0
		case a == "--mode" || a == "--source-root" || a == "--max-depth":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "check: %s requires an argument\n", a)
				return 2
			}
			i++
			switch a {
			case "--mode":
				opts.mode = args[i]
			case "--source-root":
				opts.sourceRoot = args[i]
			case "--max-depth":
				var n int
				if _, err := fmt.Sscanf(args[i], "%d", &n); err != nil || n < 0 {
					fmt.Fprintf(os.Stderr, "check: invalid --max-depth %q\n", args[i])
					return 2
				}
				opts.maxDepth = n
			}
		case strings.HasPrefix(a, "--mode="):
			opts.mode = strings.TrimPrefix(a, "--mode=")
		case strings.HasPrefix(a, "--source-root="):
			opts.sourceRoot = strings.TrimPrefix(a, "--source-root=")
		case strings.HasPrefix(a, "--max-depth="):
			var n int
			v := strings.TrimPrefix(a, "--max-depth=")
			if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 0 {
				fmt.Fprintf(os.Stderr, "check: invalid --max-depth %q\n", v)
				return 2
			}
			opts.maxDepth = n
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "check: unknown option %q\n", a)
			return 2
		default:
			scripts = append(scripts, a)
		}
	}
	if len(scripts) == 0 {
		fmt.Fprintln(os.Stderr, "check: at least one script is required")
		return 2
	}
	if opts.mode != "bashy" && opts.mode != "bash53" && opts.mode != "posix" {
		fmt.Fprintf(os.Stderr, "check: unsupported --mode %q\n", opts.mode)
		return 2
	}
	report := newCheckAnalyzer(opts).run(scripts)
	if opts.json {
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
	} else {
		printCheckReport(os.Stdout, report)
	}
	if report.Summary.Errors > 0 {
		return 1
	}
	return 0
}

func printCheckUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: bashy check [--mode bash53|posix|bashy] [--json] [--strict-system] SCRIPT...")
	fmt.Fprintln(w, "Statically check shell scripts for syntax, recursive script references, and command resolution.")
}

func newCheckAnalyzer(opts checkOptions) *checkAnalyzer {
	builtins, _, verbs := commandsCatalog()
	core := tool.Names()
	a := &checkAnalyzer{
		opts:      opts,
		seen:      map[string]bool{},
		functions: map[string]bool{},
		builtins:  sliceSet(builtins),
		coreutils: sliceSet(core),
		verbs:     sliceSet(append(append([]string{}, verbs...), "docker", "sh")),
		gnuCore:   sliceSet(gnuCoreutilsCommands),
		invSeen:   map[string]bool{},
	}
	a.report = checkReport{
		SchemaVersion: checkSchemaVersion,
		Mode:          opts.mode,
		ByFile:        map[string][]checkDiag{},
	}
	return a
}

func (a *checkAnalyzer) run(scripts []string) checkReport {
	if a.opts.sourceRoot != "" {
		if abs, err := filepath.Abs(a.opts.sourceRoot); err == nil {
			a.sourceRoot = abs
		}
	}
	for _, script := range scripts {
		a.analyzePath(script, "entry", "", 0)
	}
	a.finalize()
	return a.report
}

func (a *checkAnalyzer) analyzePath(path, role, from string, depth int) {
	if depth > a.opts.maxDepth {
		a.addDiag(checkDiagnostic{Code: "BASHY0601", Level: "warning", File: from, Message: fmt.Sprintf("recursive script analysis exceeded --max-depth at %s", path)})
		return
	}
	resolved := a.resolveScriptPath(path, from)
	if resolved == "" {
		a.addDiag(checkDiagnostic{Code: "BASHY0600", Level: "warning", File: from, Command: path, Message: "script file not found or not readable"})
		return
	}
	if a.seen[resolved] {
		return
	}
	a.seen[resolved] = true
	a.report.Files = append(a.report.Files, checkFile{Path: resolved, Role: role, From: from})

	data, err := os.ReadFile(resolved)
	if err != nil {
		a.addDiag(checkDiagnostic{Code: "BASHY0600", Level: "warning", File: resolved, Message: err.Error()})
		return
	}
	parser := syntax.NewParser(a.parserOptions()...)
	file, err := parser.Parse(strings.NewReader(string(data)), resolved)
	if err != nil {
		a.addDiag(checkDiagnostic{Code: "BASHY0001", Level: "error", File: resolved, Message: err.Error()})
		return
	}
	a.collectFunctions(file)
	a.walkFile(resolved, file, depth)
}

func (a *checkAnalyzer) parserOptions() []syntax.ParserOption {
	if a.opts.mode == "posix" {
		return []syntax.ParserOption{syntax.Variant(syntax.LangBash), syntax.PosixMode(true)}
	}
	return []syntax.ParserOption{syntax.Variant(syntax.LangBash)}
}

func (a *checkAnalyzer) resolveScriptPath(path, from string) string {
	if path == "" {
		return ""
	}
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		if from != "" {
			candidates = append(candidates, filepath.Join(filepath.Dir(from), path))
		}
		if a.sourceRoot != "" {
			candidates = append(candidates, filepath.Join(a.sourceRoot, path))
		}
	}
	for _, cand := range candidates {
		abs, err := filepath.Abs(cand)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err == nil && !info.IsDir() {
			return abs
		}
	}
	return ""
}

func (a *checkAnalyzer) collectFunctions(file *syntax.File) {
	syntax.Walk(file, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if ok && fn.Name != nil {
			a.functions[fn.Name.Value] = true
		}
		return true
	})
}

func (a *checkAnalyzer) walkFile(path string, file *syntax.File, depth int) {
	syntax.Walk(file, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		a.handleCall(path, call, depth)
		return true
	})
}

func (a *checkAnalyzer) handleCall(file string, call *syntax.CallExpr, depth int) {
	words := staticWords(call.Args)
	first := words[0]
	if !first.Static {
		a.report.Summary.Commands++
		a.addInventory("dynamic", checkInvItem{Span: span(file, call.Args[0].Pos()), Text: wordDebug(call.Args[0])})
		a.addDiag(checkDiagnostic{
			Code:    "BASHY0500",
			Level:   "info",
			File:    file,
			Line:    call.Args[0].Pos().Line(),
			Column:  call.Args[0].Pos().Col(),
			Message: "dynamic command name cannot be proven",
		})
		return
	}
	cmd := first.Value
	if cmd == "" {
		return
	}
	a.report.Summary.Commands++
	pos := call.Args[0].Pos()
	res := a.resolveCommand(cmd, file)
	diag := checkDiagnostic{
		Code:     res.Code,
		Level:    res.Level,
		File:     file,
		Line:     pos.Line(),
		Column:   pos.Col(),
		Command:  cmd,
		Message:  res.Message,
		Resolver: res.Resolver,
		Path:     res.Path,
	}
	if a.opts.strictSystem && res.Kind == "system" {
		diag.Code = "BASHY0301"
		diag.Level = "error"
		diag.Message = "command resolves to system PATH and --strict-system forbids host fallback"
	}
	a.addDiag(diag)
	a.addResolvedInventory(cmd, res, file, pos)
	a.maybeRecurse(file, cmd, words, depth)
}

type checkResolution struct {
	Kind     string
	Code     string
	Level    string
	Resolver string
	Path     string
	Message  string
}

func (a *checkAnalyzer) resolveCommand(name, from string) checkResolution {
	base := filepath.Base(name)
	if strings.ContainsAny(name, `/\`) {
		if resolved := a.resolveScriptPath(name, from); resolved != "" && isShellScript(resolved) {
			return checkResolution{Kind: "script", Code: "BASHY0203", Level: "ok", Resolver: "script", Path: resolved, Message: "statically resolved shell script"}
		}
		return a.systemOrNotFound(name)
	}
	if a.functions[name] {
		return checkResolution{Kind: "bashy", Code: "BASHY0200", Level: "ok", Resolver: "function", Message: "script function"}
	}
	if a.builtins[name] {
		return checkResolution{Kind: "bashy", Code: "BASHY0200", Level: "ok", Resolver: "builtin", Message: "bash builtin"}
	}
	if a.coreutils[name] {
		return checkResolution{Kind: "bashy", Code: "BASHY0201", Level: "ok", Resolver: "coreutil", Message: "bashy in-process userland"}
	}
	if a.verbs[name] {
		return checkResolution{Kind: "bashy", Code: "BASHY0202", Level: "ok", Resolver: "verb", Message: "bashy front-door verb"}
	}
	if a.opts.allowContainer && a.gnuCore[base] {
		return checkResolution{Kind: "container", Code: "BASHY0302", Level: "warning", Resolver: "gnu-coreutils-container", Message: "missing in bashy; available through managed GNU coreutils container"}
	}
	return a.systemOrNotFound(name)
}

func (a *checkAnalyzer) systemOrNotFound(name string) checkResolution {
	if lp, err := exec.LookPath(name); err == nil {
		return checkResolution{Kind: "system", Code: "BASHY0700", Level: "warning", Resolver: "system", Path: lp, Message: "command resolves to system PATH"}
	}
	return checkResolution{Kind: "not_found", Code: "BASHY0701", Level: "error", Resolver: "not-found", Message: "command not found in bashy, container fallback, or system PATH"}
}

func (a *checkAnalyzer) addResolvedInventory(cmd string, res checkResolution, file string, pos syntax.Pos) {
	switch res.Kind {
	case "bashy":
		a.addInventory("bashy", checkInvItem{Name: cmd, Kind: res.Resolver})
	case "container":
		a.addInventory("container", checkInvItem{Name: cmd, Image: "gnu-coreutils"})
	case "system":
		a.addInventory("system", checkInvItem{Name: cmd, Path: res.Path})
	case "not_found":
		a.addInventory("not_found", checkInvItem{Name: cmd})
	case "script":
		a.addInventory("script", checkInvItem{Name: cmd, Path: res.Path, Span: span(file, pos)})
	}
}

func (a *checkAnalyzer) maybeRecurse(file, cmd string, words []staticWord, depth int) {
	if a.opts.noSource {
		return
	}
	if (cmd == "source" || cmd == ".") && len(words) > 1 {
		if words[1].Static {
			a.analyzePath(words[1].Value, "source", file, depth+1)
		}
		return
	}
	if cmd == "bash" || cmd == "bashy" || cmd == "sh" {
		for i := 1; i < len(words); i++ {
			if !words[i].Static {
				return
			}
			arg := words[i].Value
			if arg == "-c" || arg == "--command" {
				return
			}
			if strings.HasPrefix(arg, "-") {
				continue
			}
			a.analyzePath(arg, "script", file, depth+1)
			return
		}
	}
	if strings.ContainsAny(cmd, `/\`) {
		if resolved := a.resolveScriptPath(cmd, file); resolved != "" && isShellScript(resolved) {
			a.analyzePath(resolved, "script", file, depth+1)
		}
	}
}

type staticWord struct {
	Value  string
	Static bool
}

func staticWords(words []*syntax.Word) []staticWord {
	out := make([]staticWord, 0, len(words))
	for _, w := range words {
		v, ok := staticWordValue(w)
		out = append(out, staticWord{Value: v, Static: ok})
	}
	return out
}

func staticWordValue(w *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, p := range w.Parts {
		switch x := p.(type) {
		case *syntax.Lit:
			b.WriteString(x.Value)
		case *syntax.SglQuoted:
			b.WriteString(x.Value)
		case *syntax.DblQuoted:
			for _, dp := range x.Parts {
				lit, ok := dp.(*syntax.Lit)
				if !ok {
					return "", false
				}
				b.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

func wordDebug(w *syntax.Word) string {
	if v, ok := staticWordValue(w); ok {
		return v
	}
	return "<dynamic>"
}

func isShellScript(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if strings.HasPrefix(string(data), "#!") {
		first := strings.SplitN(string(data), "\n", 2)[0]
		return strings.Contains(first, "sh") || strings.Contains(first, "bash") || strings.Contains(first, "bashy")
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".sh" || ext == ".bash" || ext == ".bashy"
}

func (a *checkAnalyzer) addDiag(d checkDiagnostic) {
	if d.Level == "" {
		d.Level = "info"
	}
	if d.Level == "error" {
		a.report.Summary.Errors++
	}
	if d.Level == "warning" {
		a.report.Summary.Warnings++
	}
	a.report.Diagnostics = append(a.report.Diagnostics, d)
	key := fmt.Sprintf("%s:%08d:%08d:%s", d.File, d.Line, d.Column, d.Command)
	a.report.ByFile[d.File] = append(a.report.ByFile[d.File], checkDiag{checkDiagnostic: d, sortKey: key})
}

func (a *checkAnalyzer) addInventory(kind string, item checkInvItem) {
	key := kind + "\x00" + item.Name + "\x00" + item.Path + "\x00" + item.Span + "\x00" + item.Text
	if a.invSeen[key] {
		return
	}
	a.invSeen[key] = true
	switch kind {
	case "bashy":
		a.report.Inventory.BashyNative = append(a.report.Inventory.BashyNative, item)
	case "container":
		a.report.Inventory.Container = append(a.report.Inventory.Container, item)
	case "system":
		a.report.Inventory.System = append(a.report.Inventory.System, item)
	case "not_found":
		a.report.Inventory.NotFound = append(a.report.Inventory.NotFound, item)
	case "dynamic":
		a.report.Inventory.Dynamic = append(a.report.Inventory.Dynamic, item)
	case "script":
		a.report.Inventory.Scripts = append(a.report.Inventory.Scripts, item)
	}
}

func (a *checkAnalyzer) finalize() {
	a.report.Summary.FilesAnalyzed = len(a.report.Files)
	a.report.Summary.BashyNative = len(a.report.Inventory.BashyNative)
	a.report.Summary.Container = len(a.report.Inventory.Container)
	a.report.Summary.System = len(a.report.Inventory.System)
	a.report.Summary.NotFound = len(a.report.Inventory.NotFound)
	a.report.Summary.Dynamic = len(a.report.Inventory.Dynamic)
	sortInv(a.report.Inventory.BashyNative)
	sortInv(a.report.Inventory.System)
	sortInv(a.report.Inventory.Container)
	sortInv(a.report.Inventory.NotFound)
	sortInv(a.report.Inventory.Dynamic)
	sortInv(a.report.Inventory.Scripts)
	for file := range a.report.ByFile {
		sort.Slice(a.report.ByFile[file], func(i, j int) bool {
			return a.report.ByFile[file][i].sortKey < a.report.ByFile[file][j].sortKey
		})
	}
}

func printCheckReport(w io.Writer, r checkReport) {
	for _, d := range r.Diagnostics {
		loc := d.File
		if d.Line > 0 {
			loc = fmt.Sprintf("%s:%d:%d", d.File, d.Line, d.Column)
		}
		cmd := d.Command
		if cmd == "" {
			cmd = "-"
		}
		fmt.Fprintf(w, "%s %-7s %-12s %s\n", loc, d.Level, cmd, d.Message)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "command inventory:")
	printInvGroup(w, "bashy native", r.Inventory.BashyNative, func(i checkInvItem) string {
		return fmt.Sprintf("    %-16s %s", i.Name, i.Kind)
	})
	printInvGroup(w, "system PATH", r.Inventory.System, func(i checkInvItem) string {
		return fmt.Sprintf("    %-16s %s", i.Name, i.Path)
	})
	printInvGroup(w, "container fallback", r.Inventory.Container, func(i checkInvItem) string {
		return fmt.Sprintf("    %-16s %s", i.Name, i.Image)
	})
	printInvGroup(w, "not found", r.Inventory.NotFound, func(i checkInvItem) string {
		return fmt.Sprintf("    %s", i.Name)
	})
	printInvGroup(w, "dynamic", r.Inventory.Dynamic, func(i checkInvItem) string {
		return fmt.Sprintf("    %-16s %s", i.Span, i.Text)
	})
	printInvGroup(w, "scripts", r.Inventory.Scripts, func(i checkInvItem) string {
		return fmt.Sprintf("    %-16s %s", i.Name, i.Path)
	})
	fmt.Fprintf(w, "\nsummary: commands=%d files=%d bashy_native=%d system=%d container=%d not_found=%d dynamic=%d errors=%d warnings=%d\n",
		r.Summary.Commands, r.Summary.FilesAnalyzed, r.Summary.BashyNative, r.Summary.System, r.Summary.Container, r.Summary.NotFound, r.Summary.Dynamic, r.Summary.Errors, r.Summary.Warnings)
}

func printInvGroup(w io.Writer, title string, items []checkInvItem, format func(checkInvItem) string) {
	fmt.Fprintf(w, "  %s:\n", title)
	if len(items) == 0 {
		fmt.Fprintln(w, "    (none)")
		return
	}
	for _, item := range items {
		fmt.Fprintln(w, format(item))
	}
}

func sortInv(items []checkInvItem) {
	sort.Slice(items, func(i, j int) bool {
		a := items[i].Name + items[i].Path + items[i].Span + items[i].Text
		b := items[j].Name + items[j].Path + items[j].Span + items[j].Text
		return a < b
	})
}

func span(file string, pos syntax.Pos) string {
	if pos.IsValid() {
		return fmt.Sprintf("%s:%d:%d", file, pos.Line(), pos.Col())
	}
	return file
}

func sliceSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

var gnuCoreutilsCommands = []string{
	"arch", "b2sum", "base32", "base64", "basename", "basenc", "cat", "chcon",
	"chgrp", "chmod", "chown", "chroot", "cksum", "comm", "coreutils", "cp",
	"csplit", "cut", "date", "dd", "df", "dir", "dircolors", "dirname", "du",
	"echo", "env", "expand", "expr", "factor", "false", "fmt", "fold", "groups",
	"head", "hostid", "hostname", "id", "install", "join", "kill", "link", "ln",
	"logname", "ls", "md5sum", "mkdir", "mkfifo", "mknod", "mktemp", "mv", "nice",
	"nl", "nohup", "nproc", "numfmt", "od", "paste", "pathchk", "pinky", "pr",
	"printenv", "printf", "ptx", "pwd", "readlink", "realpath", "rm", "rmdir",
	"runcon", "seq", "sha1sum", "sha224sum", "sha256sum", "sha384sum", "sha512sum",
	"shred", "shuf", "sleep", "sort", "split", "stat", "stdbuf", "stty", "sum",
	"sync", "tac", "tail", "tee", "test", "timeout", "touch", "tr", "true",
	"truncate", "tsort", "tty", "uname", "unexpand", "uniq", "unlink", "uptime",
	"users", "vdir", "wc", "who", "whoami", "yes",
}
