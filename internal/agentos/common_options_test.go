package agentos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/yoke/pkg/binmgr"
)

const (
	commonOptionsBaselineFile = "testdata/common_options_short_baseline.txt"
	commonOptionsMatrixFile   = "testdata/common_options_matrix.txt"
)

// commonOptionNames is the audited matrix scope: the six PRIMARY common options
// (Sprint 446 master plan) plus the --depth/--since amendment, all rows of the
// vocabulary table in docs/common-options.md (dhnt umbrella, the authority).
// The table's global/naming flags (--help, --verbose, --quiet, --debug, --yes,
// --force) are deliberately NOT matrix columns: they are naming rules, not
// per-tool capabilities. --quiet is still lint-checked (must be bool). --plain
// is not a common option.
var commonOptionNames = []string{"depth", "dry-run", "explain", "fields", "json", "no-input", "null", "since"}

// Tools whose --help is not safe to execute (posix gate/provider runners; man
// excludes them too). They never become a fourth status: each cell is
// "unsupported", justified by the tool's source, and the set is checked
// separately in TestHelpExcludedToolsAreUnsupported.
var helpExcludedTools = map[string]string{
	"posix-gate":      "cmds/posixgate/posixgate.go parses only --profile/--manifest/--shell/--bindir/--multicall",
	"posix-providers": "cmds/posixproviders/posixproviders.go top level takes only subcommands; --json exists only under `list`",
}

var matrixStatuses = []string{"supported", "native-mapped", "unsupported"}

// Tools with their own non-pflag grammar (expression languages, subcommand
// dispatchers): --help lists no pflag options, so no common option can be
// declared; the matrix records every option unsupported. Reviewed list: a tool
// outside it that parses no flags fails, and a tool in it that starts
// declaring flags fails, so the list cannot go stale.
var customGrammarTools = map[string]bool{
	"ar": true, "ast": true, "ctags": true, "db": true, "ex": true, "find": true,
	"foreman": true, "gencat": true, "graph": true, "localedef": true, "man": true,
	"nm": true, "strip": true, "test": true, "time": true, "vi": true, "xargs": true,
}

// binManagedExternalTools names tools whose --help is served by a
// binmgr-managed external binary rather than in-process flag declarations.
// Running such a handler without a cached binary downloads the pinned release
// from GitHub, which makes the audit network-dependent: on a cold CI cache a
// slow or rate-limited download returns truncated (or empty) help inside the
// toolHelp timeout, and the audit fails spuriously (Sprint 514: 'why' parsed
// 0 flags on macos/windows runners while passing locally and on rerun).
//
// When the managed binary is not already cached, the audit reads the pinned
// --help captured below instead of touching the network, and records the
// substitution with its reason; the golden stays stable and a silent 0-flag
// help can never slip through. Every entry must carry a non-empty reason
// (pinned by TestBinManagedExternalPins): no reason-free skips.
type binManagedExternal struct {
	// upstream is the managed binary name probed with binmgr.CachedBinary.
	upstream string
	// version is the pinned upstream release the captured help belongs to.
	// It must match the tool's pin (whycmd.PinnedVersion for "why"); bumping
	// the pin means re-capturing help below.
	version string
	// reason justifies the network-free substitution for this tool.
	reason string
	// help is the captured `$upstream --help` output for version.
	help string
}

var binManagedExternalTools = map[string]binManagedExternal{
	"why": {
		upstream: "witr",
		version:  "v0.3.3",
		reason:   "cmds/why/why.go forwards --help to the binmgr-pinned witr binary, so uncached help requires a GitHub download",
		help:     pinnedWitrHelp,
	},
}

// pinnedWitrHelp is `witr --help` for the pinned witr v0.3.3 release
// (pranshuparmar/witr), captured from a cached binary. Only the Flags section
// feeds the audit; the header and examples are kept so a reviewer can diff
// this against a fresh `witr --help` when the pin moves.
const pinnedWitrHelp = `witr explains why a process or port is running by tracing its ancestry.

Usage:
  witr [process name...] [flags]

Examples:

  # Inspect a running process by name
  witr nginx

  # Look up a process by PID
  witr --pid 1234

  # Find the process listening on a specific port
  witr --port 5432

  # Find the process holding a file open
  witr --file /var/lib/dpkg/lock

  # Inspect a container by name
  witr --container redis

  # Inspect a process by name with exact matching (no fuzzy search)
  witr bun --exact

  # Show the full process ancestry (who started whom)
  witr postgres --tree

  # Show only warnings (suspicious env, arguments, parents)
  witr docker --warnings

  # Display only environment variables of the process
  witr node --env

  # Short, single-line output (useful for scripts)
  witr sshd --short

  # Disable colorized output (CI or piping)
  witr redis --no-color

  # Output machine-readable JSON
  witr chrome --json

  # Show extended process information (memory, I/O, file descriptors)
  witr mysql --verbose

  # Combine flags: inspect port, show environment variables, output JSON
  witr --port 8080 --env --json

  # Multiple inputs
  witr nginx node
  witr --port 8080 --port 3000
  witr --pid 1234 --pid 5678

  # Mixed inputs
  witr nginx --pid 1234 --port 8080


Flags:
  -c, --container strings   container(s) to look up (repeatable)
      --env                 show environment variables for the process
  -x, --exact               use exact name matching (no substring search)
  -f, --file strings        file(s) held open by a process (repeatable)
  -h, --help                help for witr
  -i, --interactive         interactive mode (TUI)
      --json                show result as JSON
      --no-color            disable colorized output
  -p, --pid strings         pid(s) to look up (repeatable)
  -o, --port strings        port(s) to look up (repeatable)
  -s, --short               show only ancestry
  -t, --tree                show only ancestry as a tree
      --verbose             show extended process information
  -v, --version             version for witr
      --warnings            show only warnings
`

// Certification-profile signals are the bashy_cert build tag and applet-specific
// POSIXLY_CORRECT presence checks (see tool.SchemaFromFlags). This lint neither
// changes those signals nor normalizes flags.
//
// Three checks, all blocking:
//   - off-vocabulary spellings (--dryrun) and a nonbool --quiet fail immediately;
//     they are never baselined.
//   - short letters are compared with the reviewed baseline
//     (testdata/common_options_short_baseline.txt); a new letter fails.
//   - the tool x common-option matrix is compared with a deterministic golden
//     (testdata/common_options_matrix.txt): supported / native-mapped / unsupported.
//
// A tool whose --help cannot be read (other than the explicit exclusions) fails
// the audit. Regenerate goldens only after review: BASHY_UPDATE_COMMON_OPTIONS=1
// (refused while violations exist).
func TestCommonOptionsVocabularyReport(t *testing.T) {
	ringDir(t)
	var shorts, violations, matrix, skipped []string
	record := func(path string, flag tool.ArgFlag) {
		for _, finding := range commonOptionFindings(flag) {
			violations = append(violations, path+": "+finding)
		}
		if flag.Shorthand != "" {
			shorts = append(shorts, shortBaselineEntry(path, flag))
		}
	}
	_, _, verbs := commandsCatalog()
	verbs = append(verbs, hiddenVerbsCatalog()...)
	slices.Sort(verbs)
	verbs = slices.Compact(verbs)
	cobraCount := 0
	for _, name := range verbs {
		walkCobraSchemas(cobraVerb(name), name, func(path string, schema tool.ArgSchema) {
			cobraCount++
			for _, flag := range schema.Flags {
				record(path, flag)
			}
		})
	}
	// Tool flags are invocation-local and the registry exposes no FlagSet getter.
	// Read their own --help in bounded child processes, like man does, rather than
	// running tool handlers in the test process or inventing flag declarations.
	names := tool.Names()
	for _, name := range names {
		if _, excluded := helpExcludedTools[name]; excluded {
			for _, opt := range commonOptionNames {
				matrix = append(matrix, matrixEntry(name, opt, "unsupported"))
			}
			continue
		}
		var help string
		if ext, managed := binManagedExternalTools[name]; managed && binmgr.CachedBinary(ext.upstream) == "" {
			// Deterministic without network: audit the pinned help for the
			// pinned release and record the substitution with its reason.
			// The child process is never started, so no download can make
			// the help come back truncated or empty.
			skipped = append(skipped, name+": live --help skipped ("+ext.reason+")")
			help = ext.help
		} else {
			var err error
			help, err = toolHelp(name)
			if err != nil {
				t.Errorf("%s: help unavailable (%v); audit incomplete for this tool", name, err)
				continue
			}
		}
		flags := vocabularyHelpFlags(help)
		if custom := customGrammarTools[name]; custom != (len(flags) == 0) {
			t.Errorf("%s: customGrammarTools=%v but %d flags parsed from --help; review the list", name, custom, len(flags))
			continue
		}
		byName := map[string]helpFlag{}
		for _, f := range flags {
			record(name, f.ArgFlag)
			byName[strings.ReplaceAll(f.Name, "_", "-")] = f
		}
		for _, opt := range commonOptionNames {
			matrix = append(matrix, matrixEntry(name, opt, commonOptionStatus(opt, byName)))
		}
	}
	slices.Sort(shorts)
	shorts = slices.Compact(shorts)
	slices.Sort(violations)
	violations = slices.Compact(violations)
	slices.Sort(matrix)
	t.Logf("inspected %d registered tools and %d cobra command paths", len(names), cobraCount)
	for _, s := range skipped {
		t.Logf("bin-managed external help substitution: %s", s)
	}

	for _, v := range violations {
		t.Errorf("common-option vocabulary violation: %s", v)
	}
	if os.Getenv("BASHY_UPDATE_COMMON_OPTIONS") == "1" {
		if len(violations) > 0 || t.Failed() {
			t.Fatal("refusing to update goldens while violations or incomplete audits exist")
		}
		for file, lines := range map[string][]string{commonOptionsBaselineFile: shorts, commonOptionsMatrixFile: matrix} {
			if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	base := readGolden(t, commonOptionsBaselineFile)
	for _, line := range base {
		if strings.HasPrefix(line, "violation") {
			t.Errorf("baseline must not record violations: %q", line)
		}
	}
	for _, n := range newShortOptions(shorts, base) {
		t.Errorf("new short option %q: not in reviewed baseline; remove it or review and update %s", n, commonOptionsBaselineFile)
	}
	golden := readGolden(t, commonOptionsMatrixFile)
	if want := len(names) * len(commonOptionNames); len(matrix) != want {
		t.Errorf("matrix has %d cells, want %d tools x %d options", len(matrix), len(names), len(commonOptionNames))
	}
	for _, cell := range matrix {
		if f := strings.Split(cell, "\t"); len(f) != 3 || !slices.Contains(matrixStatuses, f[2]) {
			t.Errorf("matrix cell %q: status must be one of %v", cell, matrixStatuses)
		}
	}
	if !slices.Equal(matrix, golden) {
		t.Errorf("common-option matrix differs from %s (review, then BASHY_UPDATE_COMMON_OPTIONS=1):\n%s", commonOptionsMatrixFile, lineDiff(golden, matrix))
	}
}

func readGolden(t *testing.T, file string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reviewed golden %s: %v", file, err)
	}
	return strings.Split(strings.ReplaceAll(strings.TrimSpace(string(data)), "\r\n", "\n"), "\n")
}

func lineDiff(want, got []string) string {
	var b strings.Builder
	n := 0
	for _, l := range got {
		if !slices.Contains(want, l) && n < 40 {
			fmt.Fprintf(&b, "+ %s\n", l)
			n++
		}
	}
	for _, l := range want {
		if !slices.Contains(got, l) && n < 40 {
			fmt.Fprintf(&b, "- %s\n", l)
			n++
		}
	}
	return b.String()
}

func matrixEntry(tool, opt, status string) string {
	return tool + "\t--" + opt + "\t" + status
}

// toolHelp runs `tool --help` in a bounded child process and fails (rather
// than skips) on timeout, nonzero exit, or empty output.
func toolHelp(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestVocabularyToolHelp$")
	cmd.Env = append(os.Environ(), "BASHY_VOCABULARY_TOOL="+name)
	cmd.WaitDelay = time.Second
	var output cappedVocabularyOutput
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	if err := cmd.Run(); err != nil {
		return "", err
	}
	if strings.TrimSpace(output.String()) == "" {
		return "", fmt.Errorf("empty help output")
	}
	return output.String(), nil
}

func commonOptionStatus(opt string, flags map[string]helpFlag) string {
	f, declared := flags[opt]
	switch opt {
	case "json", "fields":
		if declared && !strings.Contains(f.usage, "exit 2") {
			return "supported"
		}
	case "null":
		if declared && !strings.Contains(f.usage, "native NUL-delimited spelling") {
			return "supported"
		}
		for _, native := range []string{"zero", "zero-terminated", "null-data"} {
			if n, ok := flags[native]; ok && n.Type == "bool" {
				return "native-mapped"
			}
		}
	default:
		if declared {
			return "supported"
		}
	}
	return "unsupported"
}

func shortBaselineEntry(path string, flag tool.ArgFlag) string {
	return fmt.Sprintf("%s\t-%s\t--%s", path, flag.Shorthand, flag.Name)
}

func newShortOptions(found, baseline []string) []string {
	var out []string
	for _, f := range found {
		if !slices.Contains(baseline, f) {
			out = append(out, f)
		}
	}
	return out
}

// A separate process bounds legacy/custom --help handlers and isolates any
// process-global state. posix gate/provider runners are not help-safe (man also
// excludes them). No operands or inherited credentials are supplied to handlers.
func TestVocabularyToolHelp(t *testing.T) {
	name := os.Getenv("BASHY_VOCABULARY_TOOL")
	if name == "" {
		return
	}
	if _, excluded := helpExcludedTools[name]; excluded {
		os.Exit(2)
	}
	dir := t.TempDir()
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, FS: tool.NewLocalFS(), Stdio: tool.Stdio{In: strings.NewReader(""), Out: os.Stdout, Err: io.Discard}}
	if command := tool.Lookup(name); command != nil {
		command.Run(rc, []string{"--help"})
	}
}

type cappedVocabularyOutput struct{ bytes.Buffer }

func (b *cappedVocabularyOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := (1 << 20) - b.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

func commonOptionFindings(flag tool.ArgFlag) []string {
	var findings []string
	for bad, good := range map[string]string{"dryrun": "dry-run", "noinput": "no-input"} {
		if flag.Name == bad {
			findings = append(findings, fmt.Sprintf("--%s: vocabulary spelling is --%s", bad, good))
		}
	}
	if strings.ReplaceAll(flag.Name, "_", "-") == "quiet" && flag.Type != "bool" {
		findings = append(findings, fmt.Sprintf("--quiet: expected bool, found %s", flag.Type))
	}
	return findings
}

// pflag help separates the declaration and prose with two or more spaces.
// Unknown/custom formats are left unclassified rather than guessed as booleans.
var vocabularyHelpFlag = regexp.MustCompile(`^\s*(?:-([A-Za-z0-9]),\s+)?--([A-Za-z0-9_-]+)(?:[ =]([^\s]+))?\s{2,}(.*)$`)

type helpFlag struct {
	tool.ArgFlag
	usage string
}

func vocabularyHelpFlags(help string) []helpFlag {
	var flags []helpFlag
	for _, line := range strings.Split(help, "\n") {
		m := vocabularyHelpFlag.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		typ := m[3]
		if typ == "" {
			typ = "bool"
		}
		flags = append(flags, helpFlag{tool.ArgFlag{Name: m[2], Shorthand: m[1], Type: typ}, m[4]})
	}
	return flags
}

func TestHelpExcludedToolsAreUnsupported(t *testing.T) {
	registered := tool.Names()
	for name, why := range helpExcludedTools {
		if !slices.Contains(registered, name) || why == "" {
			t.Errorf("help exclusion %q is stale or unjustified", name)
		}
		for _, opt := range commonOptionNames {
			if want := matrixEntry(name, opt, "unsupported"); !slices.Contains(readGolden(t, commonOptionsMatrixFile), want) {
				t.Errorf("golden lacks %q", want)
			}
		}
	}
	if len(helpExcludedTools) != 2 {
		t.Errorf("help exclusion list changed; review: %v", helpExcludedTools)
	}
}

// TestBinManagedExternalPins keeps the network-free substitutions honest:
// every entry names a registered tool with a reason, its pinned help parses
// to real flags (never a silent 0-flag help), and the pinned help reproduces
// the golden matrix and short-baseline rows for that tool, so the pin cannot
// rot unnoticed on warm caches where the substitution never runs.
func TestBinManagedExternalPins(t *testing.T) {
	registered := tool.Names()
	goldenMatrix := readGolden(t, commonOptionsMatrixFile)
	goldenShorts := readGolden(t, commonOptionsBaselineFile)
	for name, ext := range binManagedExternalTools {
		if !slices.Contains(registered, name) {
			t.Errorf("bin-managed external %q is not a registered tool", name)
		}
		if ext.upstream == "" || ext.version == "" || ext.reason == "" {
			t.Errorf("bin-managed external %q needs an upstream, version, and reason", name)
		}
		if _, excluded := helpExcludedTools[name]; excluded {
			t.Errorf("bin-managed external %q is also help-excluded; pick one", name)
		}
		if customGrammarTools[name] {
			t.Errorf("bin-managed external %q is a custom-grammar tool; pinned help must not parse flags", name)
		}
		flags := vocabularyHelpFlags(ext.help)
		if len(flags) == 0 {
			t.Errorf("bin-managed external %q: pinned %s %s help parses to 0 flags", name, ext.upstream, ext.version)
			continue
		}
		byName := map[string]helpFlag{}
		var shorts []string
		for _, f := range flags {
			for _, finding := range commonOptionFindings(f.ArgFlag) {
				t.Errorf("bin-managed external %q pinned help: %s", name, finding)
			}
			if f.Shorthand != "" {
				shorts = append(shorts, shortBaselineEntry(name, f.ArgFlag))
			}
			byName[strings.ReplaceAll(f.Name, "_", "-")] = f
		}
		for _, opt := range commonOptionNames {
			if want := matrixEntry(name, opt, commonOptionStatus(opt, byName)); !slices.Contains(goldenMatrix, want) {
				t.Errorf("bin-managed external %q pinned help: golden lacks %q", name, want)
			}
		}
		for _, s := range shorts {
			if !slices.Contains(goldenShorts, s) {
				t.Errorf("bin-managed external %q pinned help: baseline lacks %q", name, s)
			}
		}
	}
	if len(binManagedExternalTools) == 0 {
		t.Errorf("bin-managed external table is empty; add the tool or drop the machinery")
	}
}

func TestCommonOptionFindingRules(t *testing.T) {
	for _, tc := range []struct {
		flag  tool.ArgFlag
		count int
	}{
		{tool.ArgFlag{Name: "dryrun", Type: "bool"}, 1},
		{tool.ArgFlag{Name: "dry-run", Type: "bool"}, 0},
		{tool.ArgFlag{Name: "dry_run", Type: "bool"}, 0},
		{tool.ArgFlag{Name: "quiet", Type: "string"}, 1},
		{tool.ArgFlag{Name: "quiet", Type: "bool"}, 0},
		{tool.ArgFlag{Name: "json", Type: "bool", Shorthand: "J"}, 0},
	} {
		if got := len(commonOptionFindings(tc.flag)); got != tc.count {
			t.Errorf("%+v: %d findings, want %d", tc.flag, got, tc.count)
		}
	}
	flags := vocabularyHelpFlags("  -q, --quiet string   quiet mode\n      --dryrun         preview\n")
	if len(flags) != 2 || flags[0].Type != "string" || flags[1].Type != "bool" {
		t.Fatalf("parsed flags: %+v", flags)
	}

	flagsOf := func(help string) map[string]helpFlag {
		m := map[string]helpFlag{}
		for _, f := range vocabularyHelpFlags(help) {
			m[f.Name] = f
		}
		return m
	}
	fm := flagsOf("      --json            structured output (exit 2 if the command has no structured output)\n" +
		"      --null            use the command's native NUL-delimited spelling (-z/--zero/--null-data)\n" +
		"  -z, --zero            nul\n      --quiet           q\n      --explain         e\n")
	for opt, want := range map[string]string{"json": "unsupported", "null": "native-mapped", "explain": "supported", "fields": "unsupported", "dry-run": "unsupported"} {
		if got := commonOptionStatus(opt, fm); got != want {
			t.Errorf("status(%s)=%s want %s", opt, got, want)
		}
	}
	if got := commonOptionStatus("json", flagsOf("      --json   emit json\n")); got != "supported" {
		t.Errorf("native json: %s", got)
	}

	existing := shortBaselineEntry("cat", tool.ArgFlag{Name: "number", Shorthand: "n"})
	added := shortBaselineEntry("cat", tool.ArgFlag{Name: "newflag", Shorthand: "Z"})
	if got := newShortOptions([]string{existing, added}, []string{existing}); len(got) != 1 || got[0] != added {
		t.Fatalf("new short letter not detected: %v", got)
	}
	if got := newShortOptions([]string{existing}, []string{existing}); len(got) != 0 {
		t.Fatalf("baseline short letter flagged: %v", got)
	}
}
