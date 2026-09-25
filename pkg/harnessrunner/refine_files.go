package harnessrunner

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qiangli/yoke/pkg/atlas"
)

// File and git refiners prove the targets of everyday agent commands so the
// policy can bind them. Each operand becomes an exact effect; its scope is
// the workspace only when the canonical target lies inside the request's
// working directory (the root ycode confines the agent to). Anything that
// cannot be proven — a flag that writes, runs a program or reads a script
// from a file — stays unsupported, so the preflight remains fail-closed.

type fileArgSpec struct {
	valueFlags   map[string]bool // flags that take a separate, non-path value
	fileFlags    map[string]bool // flags whose value is a file that is read
	patternFlags map[string]bool // flags that supply the pattern/script
	denyFlags    map[string]bool // flags whose effect cannot be proven (writes, exec)
	patternFirst bool            // first positional is a pattern unless a pattern flag was given
	noFiles      bool            // positionals are never paths (tr, which)
	defaultCwd   bool            // no operand means the working directory is read
}

func flags(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

var fileReadCommands = map[string]fileArgSpec{
	"cat":       {},
	"tac":       {},
	"nl":        {valueFlags: flags("-b", "-d", "-f", "-h", "-i", "-l", "-n", "-s", "-v", "-w", "--body-numbering", "--section-delimiter", "--footer-numbering", "--header-numbering", "--line-increment", "--join-blank-lines", "--number-format", "--number-separator", "--starting-line-number", "--number-width")},
	"head":      {valueFlags: flags("-n", "-c", "--lines", "--bytes")},
	"tail":      {valueFlags: flags("-n", "-c", "-s", "--lines", "--bytes", "--sleep-interval", "--pid", "--max-unchanged-stats")},
	"wc":        {fileFlags: flags("--files0-from")},
	"stat":      {valueFlags: flags("-c", "--format", "--printf", "-f")},
	"file":      {valueFlags: flags("-m", "-F", "--magic-file", "--separator"), fileFlags: flags("-f", "--files-from")},
	"diff":      {valueFlags: flags("-U", "-C", "-L", "--label", "--unified", "--context", "-I", "--ignore-matching-lines", "-x", "--exclude", "--line-format", "--old-line-format", "--new-line-format", "--unchanged-line-format", "-F", "--show-function-line", "--horizon-lines", "-W", "--width"), fileFlags: flags("-X", "--exclude-from", "--from-file", "--to-file"), denyFlags: flags("--output")},
	"cmp":       {valueFlags: flags("-i", "-n", "--ignore-initial", "--bytes")},
	"sort":      {valueFlags: flags("-k", "-t", "-S", "--key", "--field-separator", "--buffer-size", "--parallel", "--batch-size"), fileFlags: flags("--files0-from"), denyFlags: flags("-o", "--output", "-T", "--temporary-directory", "--compress-program")},
	"cut":       {valueFlags: flags("-b", "-c", "-d", "-f", "--bytes", "--characters", "--delimiter", "--fields", "--output-delimiter")},
	"tr":        {noFiles: true},
	"which":     {noFiles: true},
	"realpath":  {valueFlags: flags("--relative-to", "--relative-base")},
	"readlink":  {},
	"du":        {defaultCwd: true, valueFlags: flags("-d", "-B", "-t", "--max-depth", "--block-size", "--threshold", "--time-style", "--exclude"), fileFlags: flags("-X", "--exclude-from", "--files0-from")},
	"tree":      {defaultCwd: true, valueFlags: flags("-L", "-P", "-I", "--charset", "--filelimit", "--timefmt", "--sort"), denyFlags: flags("-o")},
	"od":        {valueFlags: flags("-A", "-j", "-N", "-S", "-t", "-w", "--address-radix", "--skip-bytes", "--read-bytes", "--strings", "--format", "--width")},
	"md5sum":    {},
	"sha256sum": {},
	"ls":        {defaultCwd: true, valueFlags: flags("-I", "-T", "-w", "--block-size", "--color", "--format", "--hide", "--ignore", "--indicator-style", "--quoting-style", "--sort", "--tabsize", "--time", "--time-style", "--width")},
	"grep": {
		patternFirst: true,
		defaultCwd:   true,
		patternFlags: flags("-e", "--regexp", "-f", "--file"),
		fileFlags:    flags("-f", "--file", "--exclude-from"),
		valueFlags:   flags("-e", "--regexp", "-m", "--max-count", "-A", "-B", "-C", "--after-context", "--before-context", "--context", "--include", "--exclude", "--exclude-dir", "--color", "--colour", "--label", "-d", "-D", "--devices", "--directories", "--binary-files"),
	},
}

func init() {
	for name := range fileReadCommands {
		commandRefiners[name] = refineFileRead
	}
	commandRefiners["find"] = refineFind
	commandRefiners["sed"] = refineSed
	commandRefiners["git"] = refineGit
}

// effectiveCwd is the directory relative operands resolve against: the
// request's working directory, moved by any static `cd` earlier in the script.
func (intent *Intent) effectiveCwd() string {
	if intent.cursor != "" {
		return intent.cursor
	}
	return intent.Cwd
}

func resolveOperand(intent *Intent, operand string) string {
	if !filepath.IsAbs(operand) {
		operand = filepath.Join(intent.effectiveCwd(), operand)
	}
	return canonicalTarget(operand)
}

func scopeFor(intent *Intent, target string) string {
	root := intent.Cwd
	if target == root || strings.HasPrefix(target, root+string(filepath.Separator)) {
		return atlas.TierWorkspace
	}
	return atlas.TierUserland
}

func addTargetEffect(intent *Intent, kind, operand, source string) {
	target := resolveOperand(intent, operand)
	intent.Effects = append(intent.Effects, Effect{Kind: kind, Scope: scopeFor(intent, target), Target: target, Source: source, Certainty: "exact"})
}

func stripFrontDoor(argv []string) []string {
	if len(argv) > 1 && isBashyFrontDoor(argv[0]) {
		return argv[1:]
	}
	return argv
}

func refineFileRead(argv []string, allStatic bool, intent *Intent) bool {
	args := stripFrontDoor(argv)
	name := args[0]
	spec := fileReadCommands[name]
	source := "command:" + name
	if !allStatic {
		markUnsupported(intent, "dynamicCommand", strings.Join(argv, " "), "command name or arguments cannot be proven")
		return true
	}
	var positionals []string
	patternGiven := false
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			flag, value, hasValue := strings.Cut(arg, "=")
			if spec.denyFlags[flag] {
				markUnsupported(intent, "effectRefinement", name+" "+flag, "flag writes or executes outside the proven targets")
				return true
			}
			if spec.patternFlags[flag] {
				patternGiven = true
			}
			takesValue := spec.valueFlags[flag] || spec.fileFlags[flag]
			if takesValue && !hasValue && i+1 < len(args) {
				i++
				value = args[i]
				hasValue = true
			}
			if spec.fileFlags[flag] && hasValue {
				addTargetEffect(intent, atlas.EffRead, value, source)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for j := 1; j < len(arg); j++ {
				flag := "-" + string(arg[j])
				if spec.denyFlags[flag] {
					markUnsupported(intent, "effectRefinement", name+" "+flag, "flag writes or executes outside the proven targets")
					return true
				}
				if spec.patternFlags[flag] {
					patternGiven = true
				}
				if spec.valueFlags[flag] || spec.fileFlags[flag] {
					value := arg[j+1:]
					if value == "" && i+1 < len(args) {
						i++
						value = args[i]
					}
					if spec.fileFlags[flag] && value != "" {
						addTargetEffect(intent, atlas.EffRead, value, source)
					}
					break
				}
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	if spec.patternFirst && !patternGiven && len(positionals) > 0 {
		positionals = positionals[1:]
	}
	if spec.noFiles {
		positionals = nil
	}
	if len(positionals) == 0 && spec.defaultCwd {
		addTargetEffect(intent, atlas.EffRead, ".", source)
	}
	for _, operand := range positionals {
		if operand == "-" {
			continue // stdin: the producer's effects are already counted
		}
		addTargetEffect(intent, atlas.EffRead, operand, source)
	}
	return true
}

var findActionDeny = flags("-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprint0", "-fprintf", "-fls")

func refineFind(argv []string, allStatic bool, intent *Intent) bool {
	args := stripFrontDoor(argv)
	if !allStatic {
		markUnsupported(intent, "dynamicCommand", strings.Join(argv, " "), "command name or arguments cannot be proven")
		return true
	}
	var roots []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") || a == "(" || a == "!" || a == "," {
			break
		}
		roots = append(roots, a)
	}
	for ; i < len(args); i++ {
		if findActionDeny[args[i]] {
			markUnsupported(intent, "effectRefinement", "find "+args[i], "find action writes or executes a program")
			return true
		}
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	for _, root := range roots {
		addTargetEffect(intent, atlas.EffRead, root, "command:find")
	}
	return true
}

// sedScriptUnprovable reports a script that writes (w, W, s///w), executes
// (e, s///e) or reads another file (r, R): those targets are not operands.
var sedCommandUnprovable = regexp.MustCompile(`(?m)(^|[;{}\n$/,!0-9]|\s)[wWerR](\s|$)`) // after an address too; false positives only fail closed

func sedScriptUnprovable(script string) bool {
	if sedCommandUnprovable.MatchString(script) {
		return true
	}
	// s<d>regex<d>replacement<d>flags — a w or e flag writes or executes.
	for i := 0; i+1 < len(script); i++ {
		if script[i] != 's' || (i > 0 && isSedWordByte(script[i-1])) {
			continue
		}
		delim := script[i+1]
		if delim == '\\' || delim == '\n' || isSedWordByte(delim) {
			continue
		}
		j, parts := i+2, 0
		for j < len(script) && parts < 2 {
			switch script[j] {
			case '\\':
				j++
			case delim:
				parts++
			}
			j++
		}
		if parts < 2 {
			continue
		}
		for j < len(script) && strings.ContainsRune("gpiImM0123456789we", rune(script[j])) {
			if script[j] == 'w' || script[j] == 'e' {
				return true
			}
			j++
		}
	}
	return false
}

func isSedWordByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

func refineSed(argv []string, allStatic bool, intent *Intent) bool {
	args := stripFrontDoor(argv)
	source := "command:sed"
	if !allStatic {
		markUnsupported(intent, "dynamicCommand", strings.Join(argv, " "), "command name or arguments cannot be proven")
		return true
	}
	var scripts, positionals []string
	inPlace := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positionals = append(positionals, args[i+1:]...)
			i = len(args)
		case a == "-e" || a == "--expression":
			if i+1 < len(args) {
				i++
				scripts = append(scripts, args[i])
			}
		case strings.HasPrefix(a, "--expression="):
			scripts = append(scripts, strings.TrimPrefix(a, "--expression="))
		case a == "-f" || a == "--file" || strings.HasPrefix(a, "--file="):
			markUnsupported(intent, "effectRefinement", "sed -f", "script read from a file cannot be proven")
			return true
		case a == "-i" || strings.HasPrefix(a, "-i") && !strings.HasPrefix(a, "-in") || a == "--in-place" || strings.HasPrefix(a, "--in-place="):
			inPlace = true
		case a == "-l" || a == "--line-length":
			i++
		case strings.HasPrefix(a, "-") && a != "-":
			// -n -E -r -z -s -u --posix --debug --quiet --silent … are booleans
		default:
			positionals = append(positionals, a)
		}
	}
	if len(scripts) == 0 && len(positionals) > 0 {
		scripts = append(scripts, positionals[0])
		positionals = positionals[1:]
	}
	for _, script := range scripts {
		if sedScriptUnprovable(script) {
			markUnsupported(intent, "effectRefinement", "sed script", "script writes, executes or reads another file")
			return true
		}
	}
	for _, operand := range positionals {
		if operand == "-" {
			continue
		}
		addTargetEffect(intent, atlas.EffRead, operand, source)
		if inPlace {
			addTargetEffect(intent, atlas.EffWrite, operand, source)
		}
	}
	return true
}

var (
	gitReadSubcommands  = flags("status", "diff", "log", "show", "blame", "ls-files", "grep", "rev-parse", "shortlog", "describe", "cat-file", "ls-tree")
	gitWriteSubcommands = flags("add", "apply", "restore", "stash")
	// options that run a program, write a file or reconfigure git
	gitDenyArgs = []string{"--output", "--ext-diff", "--textconv", "-O", "--open-files-in-pager", "--exec"}
)

// refineGit accepts git's local read subcommands and a few workspace writes
// that run no hooks. Global -c, --git-dir, --work-tree, aliases and every
// network or history-rewriting subcommand stay unsupported.
func refineGit(argv []string, allStatic bool, intent *Intent) bool {
	args := stripFrontDoor(argv)
	if !allStatic {
		markUnsupported(intent, "dynamicCommand", strings.Join(argv, " "), "command name or arguments cannot be proven")
		return true
	}
	dir := "."
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-C" && i+1 < len(args):
			i++
			dir = args[i]
		case a == "--no-pager" || a == "-P" || a == "--no-optional-locks":
		case strings.HasPrefix(a, "-"):
			markUnsupported(intent, "effectRefinement", "git "+a, "global git option cannot be proven")
			return true
		default:
			goto subcommand
		}
	}
	return false
subcommand:
	sub := args[i]
	rest := args[i+1:]
	for _, a := range rest {
		for _, deny := range gitDenyArgs {
			if a == deny || strings.HasPrefix(a, deny+"=") || (deny == "-O" && strings.HasPrefix(a, "-O")) {
				markUnsupported(intent, "effectRefinement", "git "+sub+" "+a, "option writes a file or runs a program")
				return true
			}
		}
	}
	switch {
	case gitReadSubcommands[sub]:
		addTargetEffect(intent, atlas.EffRead, dir, "command:git")
		return true
	case gitWriteSubcommands[sub]:
		if sub == "stash" && len(rest) > 0 && (rest[0] == "drop" || rest[0] == "clear") {
			markUnsupported(intent, "effectRefinement", "git stash "+rest[0], "discards stashed work")
			return true
		}
		addTargetEffect(intent, atlas.EffRead, dir, "command:git")
		addTargetEffect(intent, atlas.EffWrite, dir, "command:git")
		return true
	}
	return false
}

// refineCd binds `cd DIR` to a read of DIR and moves the cursor that later
// relative operands resolve against. The cursor ignores subshell scoping,
// which only ever resolves a later operand under a directory the script named.
func refineCd(argv []string, allStatic bool, intent *Intent) {
	if !allStatic {
		markUnsupported(intent, "dynamicCommand", strings.Join(argv, " "), "command name or arguments cannot be proven")
		return
	}
	var operands []string
	for _, a := range argv[1:] {
		if a == "-L" || a == "-P" || a == "-e" || a == "-@" {
			continue
		}
		operands = append(operands, a)
	}
	if len(operands) != 1 || operands[0] == "-" {
		markUnsupported(intent, "effectRefinement", strings.Join(argv, " "), "cd needs exactly one explicit directory")
		return
	}
	fact := CommandFact{Name: "cd", Argv: append([]string(nil), argv...), Resolver: "shell-builtin", Tier: atlas.TierUserland, Stage: atlas.StageCross, Shape: string(atlas.ShapeResult), MaximumEffects: []string{atlas.EffRead}}
	fact.ContentDigest, _ = executableDigest()
	if fact.ContentDigest == "" {
		markUnsupported(intent, "executableDigest", "cd", "running Bashy binary could not be digested")
	}
	intent.Commands = append(intent.Commands, fact)
	target := resolveOperand(intent, operands[0])
	intent.Effects = append(intent.Effects, Effect{Kind: atlas.EffRead, Scope: scopeFor(intent, target), Target: target, Source: "builtin:cd", Certainty: "exact"})
	intent.cursor = target
}
