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
)

// Certification-profile signals are the bashy_cert build tag and applet-specific
// POSIXLY_CORRECT presence checks (see tool.SchemaFromFlags). This audit neither
// changes those signals nor normalizes flags. docs/common-options.md in the dhnt
// umbrella is the vocabulary authority. Existing short options are audit
// candidates, not violations: new short letters require review against this
// report. No finding below fails the test or rewrites a certified command.
func TestCommonOptionsVocabularyReport(t *testing.T) {
	ringDir(t)
	report := func(path string, flag tool.ArgFlag) {
		for _, finding := range commonOptionFindings(flag) {
			t.Logf("%s: %s", path, finding)
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
				report(path, flag)
			}
		})
	}
	// Tool flags are invocation-local and the registry exposes no FlagSet getter.
	// Read their own --help in bounded child processes, like man does, rather than
	// running tool handlers in the test process or inventing flag declarations.
	for _, name := range tool.Names() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestVocabularyToolHelp$")
		cmd.Env = append(os.Environ(), "BASHY_VOCABULARY_TOOL="+name)
		var output cappedVocabularyOutput
		cmd.Stdout, cmd.Stderr = &output, io.Discard
		err := cmd.Run()
		cancel()
		if err != nil {
			t.Logf("%s: help unavailable (%v); audit incomplete for this tool", name, err)
			continue
		}
		for _, flag := range vocabularyHelpFlags(output.String()) {
			report(name, flag)
		}
	}
	t.Logf("report-only: inspected %d registered tools and %d cobra command paths", len(tool.Names()), cobraCount)
}

// A separate process bounds legacy/custom --help handlers and isolates any
// process-global state. posix gate/provider runners are not help-safe (man also
// excludes them). No operands or inherited credentials are supplied to handlers.
func TestVocabularyToolHelp(t *testing.T) {
	name := os.Getenv("BASHY_VOCABULARY_TOOL")
	if name == "" {
		return
	}
	if name == "posix-providers" || name == "posix-gate" {
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
	if flag.Name == "dryrun" {
		findings = append(findings, "--dryrun: vocabulary spelling is --dry-run")
	}
	if strings.ReplaceAll(flag.Name, "_", "-") == "quiet" && flag.Type != "bool" {
		findings = append(findings, fmt.Sprintf("--quiet: expected bool, found %s", flag.Type))
	}
	if flag.Shorthand != "" {
		findings = append(findings, fmt.Sprintf("-%s (--%s): short-option audit candidate; preserve existing letters, add no new letters", flag.Shorthand, flag.Name))
	}
	return findings
}

// pflag help separates the declaration and prose with two or more spaces.
// Unknown/custom formats are left unclassified rather than guessed as booleans.
var vocabularyHelpFlag = regexp.MustCompile(`^\s*(?:-([A-Za-z0-9]),\s+)?--([A-Za-z0-9_-]+)(?:[ =]([^\s]+))?\s{2,}`)

func vocabularyHelpFlags(help string) []tool.ArgFlag {
	var flags []tool.ArgFlag
	for _, line := range strings.Split(help, "\n") {
		m := vocabularyHelpFlag.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		typ := m[3]
		if typ == "" {
			typ = "bool"
		}
		flags = append(flags, tool.ArgFlag{Name: m[2], Shorthand: m[1], Type: typ})
	}
	return flags
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
		{tool.ArgFlag{Name: "json", Type: "bool", Shorthand: "J"}, 1},
	} {
		if got := len(commonOptionFindings(tc.flag)); got != tc.count {
			t.Errorf("%+v: %d findings, want %d", tc.flag, got, tc.count)
		}
	}
	flags := vocabularyHelpFlags("  -q, --quiet string   quiet mode\n      --dryrun         preview\n")
	if len(flags) != 2 || flags[0].Type != "string" || flags[1].Type != "bool" {
		t.Fatalf("parsed flags: %+v", flags)
	}
}
