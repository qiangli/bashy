// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Package cmdgen generates standalone tiny main packages from bashy command
// catalog entries and builds them on demand into content-addressed build caches
// and yoke/pkg/shape/slot side slots.
//
// Sources stay in their owning packages (coreutils/cmds/*, yoke/cmds/*,
// yoke/pkg/*, yoke/external/*); there is no optional/ folder. Each catalog
// entry declares the Go package, module version, and entrypoint wiring, and
// parallel P4 workers can run cmdgen mechanically over disjoint verb lists.
package cmdgen

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Supported command entrypoint kinds.
const (
	KindCobra = "cobra" // constructor returns *cobra.Command
	KindTool  = "tool"  // blank-imported coreutils/yoke tool.Register applet
	KindFunc  = "func"  // direct Go function returning int exit code
)

// Supported function signatures for KindFunc.
const (
	SigArgs       = "args"         // fn(args []string) int
	SigCtxArgs    = "ctx_args"     // fn(ctx context.Context, args []string) int
	SigArgsOutErr = "args_out_err" // fn(args []string, stdout, stderr io.Writer) int
	SigArgsStdio  = "args_stdio"   // fn(args []string, stdin io.Reader, stdout, stderr io.Writer) int
)

// Entry describes one catalog command whose standalone binary cmdgen emits.
type Entry struct {
	Name        string   `yaml:"name" json:"name"`
	Aliases     []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Synopsis    string   `yaml:"synopsis,omitempty" json:"synopsis,omitempty"`
	Batch       string   `yaml:"batch,omitempty" json:"batch,omitempty"`
	Kind        string   `yaml:"kind" json:"kind"`
	Module      string   `yaml:"module" json:"module"`
	Version     string   `yaml:"version,omitempty" json:"version,omitempty"`
	Package     string   `yaml:"package" json:"package"`
	Constructor string   `yaml:"constructor,omitempty" json:"constructor,omitempty"`
	ToolName    string   `yaml:"tool_name,omitempty" json:"tool_name,omitempty"`
	// ArgsSignature selects the call form for kind=func (args, ctx_args,
	// args_out_err, args_stdio). Ignored for cobra and tool.
	ArgsSignature string `yaml:"args_signature,omitempty" json:"args_signature,omitempty"`
	// Use optionally overrides cmd.Use for cobra commands.
	Use string `yaml:"use,omitempty" json:"use,omitempty"`
	// PrependArgs are fixed arguments prepended ahead of os.Args[1:]
	// (for example, acp -> ["acp"] on herald.Run).
	PrependArgs []string `yaml:"prepend_args,omitempty" json:"prepend_args,omitempty"`
	// ExitFunc names an exit-code extractor for cobra Execute() errors
	// (e.g. "ExitCode" in the target package, or "pkg.ExitCode" via extra_imports).
	ExitFunc string `yaml:"exit_func,omitempty" json:"exit_func,omitempty"`
	// PrintErr controls whether a non-nil cobra Execute() error is printed as
	// "bashy <name>: <err>" to stderr. Defaults to true when nil.
	PrintErr *bool `yaml:"print_err,omitempty" json:"print_err,omitempty"`
	// SilenceCompletion disables Cobra's default completion subcommand.
	// Defaults to true when nil.
	SilenceCompletion *bool `yaml:"silence_completion,omitempty" json:"silence_completion,omitempty"`
	// ExtraImports are additional Go import specs ("path", "alias path", or "_ path").
	ExtraImports []string `yaml:"extra_imports,omitempty" json:"extra_imports,omitempty"`
	// ExtraRequires are additional "module version" pins required in go.mod.
	ExtraRequires []ModuleReq `yaml:"extra_requires,omitempty" json:"extra_requires,omitempty"`
	// Setup holds optional Go statements emitted before constructing/invoking
	// the command (for P4c host-wiring hooks).
	Setup []string `yaml:"setup,omitempty" json:"setup,omitempty"`
	// PostConstruct holds optional Go statements emitted after `cmd := ...`
	// and before `cmd.Execute()` for cobra commands.
	PostConstruct []string `yaml:"post_construct,omitempty" json:"post_construct,omitempty"`
	// BuildTags are optional Go build tags passed when compiling this command.
	BuildTags []string `yaml:"build_tags,omitempty" json:"build_tags,omitempty"`
}

// ModuleReq is one explicit Go module requirement pin.
type ModuleReq struct {
	Module  string `yaml:"module" json:"module"`
	Version string `yaml:"version" json:"version"`
}

// Replace is one Go module replace directive extracted from a host go.mod.
type Replace struct {
	OldPath    string
	OldVersion string
	NewPath    string
	NewVersion string
}

// CatalogFile is the optional wrapper document when multiple entries are placed
// in a single YAML file.
type CatalogFile struct {
	Entries []Entry `yaml:"entries"`
}

var (
	validCommandName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	validIdentOrCall = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?(\(.*\))?$`)
)

// PrintError reports whether cobra Execute() errors should be printed to stderr.
func (e Entry) PrintError() bool {
	if e.PrintErr == nil {
		return true
	}
	return *e.PrintErr
}

// DisableCompletion reports whether Cobra's completion subcommand is disabled.
func (e Entry) DisableCompletion() bool {
	if e.SilenceCompletion == nil {
		return true
	}
	return *e.SilenceCompletion
}

// Names returns the canonical name followed by any aliases.
func (e Entry) Names() []string {
	out := make([]string, 0, 1+len(e.Aliases))
	if e.Name != "" {
		out = append(out, e.Name)
	}
	for _, a := range e.Aliases {
		if a != "" && !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out
}

// Validate checks structural invariants on a catalog entry.
func (e Entry) Validate() error {
	if !validCommandName.MatchString(e.Name) {
		return fmt.Errorf("cmdgen: invalid command name %q", e.Name)
	}
	for _, a := range e.Aliases {
		if !validCommandName.MatchString(a) {
			return fmt.Errorf("cmdgen: command %q: invalid alias %q", e.Name, a)
		}
	}
	if strings.TrimSpace(e.Module) == "" {
		return fmt.Errorf("cmdgen: command %q: module is required", e.Name)
	}
	if strings.TrimSpace(e.Package) == "" {
		return fmt.Errorf("cmdgen: command %q: package is required", e.Name)
	}
	if strings.ContainsAny(e.Module, " \t\r\n\"") || strings.ContainsAny(e.Package, " \t\r\n\"") {
		return fmt.Errorf("cmdgen: command %q: module/package contains invalid whitespace or quotes", e.Name)
	}
	switch e.Kind {
	case KindCobra:
		if strings.TrimSpace(e.Constructor) == "" {
			return fmt.Errorf("cmdgen: command %q: kind=cobra requires constructor", e.Name)
		}
		if !validIdentOrCall.MatchString(strings.TrimSpace(e.Constructor)) {
			return fmt.Errorf("cmdgen: command %q: invalid constructor %q", e.Name, e.Constructor)
		}
	case KindTool:
		// ToolName defaults to Name; Constructor is unused.
	case KindFunc:
		if strings.TrimSpace(e.Constructor) == "" {
			return fmt.Errorf("cmdgen: command %q: kind=func requires constructor (function name)", e.Name)
		}
		if !validIdentOrCall.MatchString(strings.TrimSpace(e.Constructor)) {
			return fmt.Errorf("cmdgen: command %q: invalid func %q", e.Name, e.Constructor)
		}
		switch e.ArgsSignature {
		case "", SigArgs, SigCtxArgs, SigArgsOutErr, SigArgsStdio:
		default:
			return fmt.Errorf("cmdgen: command %q: unsupported args_signature %q", e.Name, e.ArgsSignature)
		}
	default:
		return fmt.Errorf("cmdgen: command %q: kind must be %s, %s, or %s (got %q)", e.Name, KindCobra, KindTool, KindFunc, e.Kind)
	}
	return nil
}

// ParseEntries decodes one or more catalog entries from YAML bytes.
// It accepts:
//   - a single Entry YAML document,
//   - a CatalogFile document with an `entries:` list, or
//   - a multi-document YAML stream separated by `---`.
func ParseEntries(data []byte) ([]Entry, error) {
	var out []Entry
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cmdgen: parse yaml: %w", err)
		}
		if node.Kind == 0 || (node.Kind == yaml.DocumentNode && len(node.Content) == 0) {
			continue
		}
		body, err := yaml.Marshal(&node)
		if err != nil {
			return nil, err
		}
		// Detect `entries:` wrapper vs single Entry document.
		var raw map[string]any
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("cmdgen: parse yaml map: %w", err)
		}
		if len(raw) == 0 {
			continue
		}
		if _, hasEntries := raw["entries"]; hasEntries {
			var cf CatalogFile
			d := yaml.NewDecoder(bytes.NewReader(body))
			d.KnownFields(true)
			if err := d.Decode(&cf); err != nil {
				return nil, fmt.Errorf("cmdgen: parse catalog file: %w", err)
			}
			out = append(out, cf.Entries...)
			continue
		}
		var e Entry
		d := yaml.NewDecoder(bytes.NewReader(body))
		d.KnownFields(true)
		if err := d.Decode(&e); err != nil {
			return nil, fmt.Errorf("cmdgen: parse entry: %w", err)
		}
		out = append(out, e)
	}
	return out, nil
}

// LoadCatalog loads all catalog entries from a single YAML file or a directory
// of `*.yaml`/`*.yml` files, validates every entry, rejects duplicate names or
// aliases, and returns them sorted by canonical Name.
func LoadCatalog(path string) ([]Entry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		entries, err := ParseEntries(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return finalizeEntries(entries)
	}
	return LoadCatalogFS(os.DirFS(path), ".")
}

// LoadCatalogFS loads all `*.yaml`/`*.yml` catalog entries under dir inside fsys.
func LoadCatalogFS(fsys fs.FS, dir string) ([]Entry, error) {
	var files []string
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var all []Entry
	for _, f := range files {
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		entries, err := ParseEntries(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		all = append(all, entries...)
	}
	return finalizeEntries(all)
}

func finalizeEntries(entries []Entry) ([]Entry, error) {
	seen := map[string]string{}
	for _, e := range entries {
		if err := e.Validate(); err != nil {
			return nil, err
		}
		for _, n := range e.Names() {
			if prev, dup := seen[n]; dup {
				return nil, fmt.Errorf("cmdgen: duplicate command name/alias %q (claimed by %q and %q)", n, prev, e.Name)
			}
			seen[n] = e.Name
		}
	}
	out := append([]Entry(nil), entries...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ReadVerbListFile parses a verb-list file for parallel P4 workers: one verb
// per line (or whitespace/comma-separated), ignoring blank lines and `#`
// comments. Preserves order and deduplicates.
func ReadVerbListFile(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseVerbList(string(b)), nil
}

// ParseVerbList parses verb names from comma/newline/whitespace text with `#`
// line comments.
func ParseVerbList(text string) []string {
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.ReplaceAll(line, ",", " ")
		for _, tok := range strings.Fields(line) {
			if !seen[tok] {
				seen[tok] = true
				out = append(out, tok)
			}
		}
	}
	return out
}

// SelectVerbs filters entries to the requested verb names (matching canonical
// name or alias). Returns an error naming any requested verb missing from the
// catalog so parallel P4 worker manifests fail loudly on typos.
func SelectVerbs(entries []Entry, verbs []string) ([]Entry, error) {
	if len(verbs) == 0 {
		return append([]Entry(nil), entries...), nil
	}
	byName := map[string]Entry{}
	for _, e := range entries {
		for _, n := range e.Names() {
			byName[n] = e
		}
	}
	var selected []Entry
	seen := map[string]bool{}
	var missing []string
	for _, v := range verbs {
		e, ok := byName[v]
		if !ok {
			missing = append(missing, v)
			continue
		}
		if !seen[e.Name] {
			seen[e.Name] = true
			selected = append(selected, e)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("cmdgen: unknown verb(s) not in catalog: %s", strings.Join(missing, ", "))
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })
	return selected, nil
}

// GoModInfo holds module versions and replace directives parsed from a go.mod.
type GoModInfo struct {
	GoVersion string
	Toolchain string
	Requires  map[string]string
	Replaces  []Replace
}

// ParseGoModFile extracts go version, required module versions, and replace
// directives from a go.mod file using stdlib-only line parsing.
func ParseGoModFile(path string) (GoModInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return GoModInfo{}, err
	}
	return ParseGoMod(b), nil
}

// ParseGoMod extracts go version, required module versions, and replace
// directives from go.mod bytes.
func ParseGoMod(data []byte) GoModInfo {
	info := GoModInfo{Requires: map[string]string{}}
	inRequire := false
	inReplace := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if inRequire {
			if line == ")" {
				inRequire = false
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				info.Requires[fields[0]] = fields[1]
			}
			continue
		}
		if inReplace {
			if line == ")" {
				inReplace = false
				continue
			}
			if r, ok := parseReplaceLine(line); ok {
				info.Replaces = append(info.Replaces, r)
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "go "):
			info.GoVersion = strings.TrimSpace(strings.TrimPrefix(line, "go "))
		case strings.HasPrefix(line, "toolchain "):
			info.Toolchain = strings.TrimSpace(strings.TrimPrefix(line, "toolchain "))
		case line == "require (":
			inRequire = true
		case strings.HasPrefix(line, "require "):
			fields := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(fields) >= 2 {
				info.Requires[fields[0]] = fields[1]
			}
		case line == "replace (":
			inReplace = true
		case strings.HasPrefix(line, "replace "):
			if r, ok := parseReplaceLine(strings.TrimPrefix(line, "replace ")); ok {
				info.Replaces = append(info.Replaces, r)
			}
		}
	}
	return info
}

func parseReplaceLine(line string) (Replace, bool) {
	left, right, ok := strings.Cut(line, "=>")
	if !ok {
		return Replace{}, false
	}
	lf := strings.Fields(left)
	rf := strings.Fields(right)
	if len(lf) == 0 || len(rf) == 0 {
		return Replace{}, false
	}
	r := Replace{OldPath: lf[0], NewPath: rf[0]}
	if len(lf) >= 2 {
		r.OldVersion = lf[1]
	}
	if len(rf) >= 2 {
		r.NewVersion = rf[1]
	}
	return r, true
}

// ResolveVersion fills in e.Version from info.Requires when omitted.
func (e Entry) ResolveVersion(info GoModInfo) Entry {
	if e.Version == "" {
		if v, ok := info.Requires[e.Module]; ok {
			e.Version = v
		}
	}
	return e
}
