package agentos

import "strings"

// DeclaredNudgeRow is one entry of the generated nudge table. Rules are no
// longer Go switches: they are declared rows derived from the vocabulary
// (common-options/lens/adapt), and Suggest routes through them. This file is
// the bashy-side projection of the trigger table's hint generation.
type DeclaredNudgeRow struct {
	When    string // human condition
	Match   func(args []string, isBuiltin bool) bool
	Suggest string
}

// declaredNudgeRows is generated from the vocabulary. Failure-path rows fire
// only after a foreign command rejects a common option; improvisation-path rows
// fire on argv shape alone. Budget: one hint per tool per session.
var declaredNudgeRows = []DeclaredNudgeRow{
	{
		When: "failure: foreign command rejects --json",
		Match: func(args []string, _ bool) bool {
			return containsArg(args, "--json") && isForeignCommand(args[0])
		},
		Suggest: "that command does not support --json directly — try `bashy adapt --json -- CMD` (common options for any command).",
	},
	{
		When: "failure: foreign command rejects --dry-run",
		Match: func(args []string, _ bool) bool {
			return containsArg(args, "--dry-run") && isForeignCommand(args[0])
		},
		Suggest: "that command does not support --dry-run — try `bashy adapt --dry-run -- CMD` or check `bashy help common-options`.",
	},
	{
		When: "improvisation: python yaml / yq / pdftotext",
		Match: func(args []string, _ bool) bool {
			if args[0] == "python" && containsSubstring(args, "yaml") {
				return true
			}
			if args[0] == "yq" {
				return true
			}
			if args[0] == "pdftotext" || containsSubstring(args, "PyPDF2") || containsSubstring(args, "markitdown") {
				return true
			}
			return false
		},
		Suggest: "for structured data use `lens -f json -t yaml` / `lens -t md` (representation filter) instead of ad-hoc python/yq/pdftotext.",
	},
	{
		When: "improvisation: unzip -l / tar -t",
		Match: func(args []string, _ bool) bool {
			if args[0] == "unzip" && containsArg(args, "-l") {
				return true
			}
			if args[0] == "tar" && containsArg(args, "-t") {
				return true
			}
			return false
		},
		Suggest: "to list any archive/container uniformly, use `lens -t listing FILE` instead of tar -t / unzip -l.",
	},
	{
		When: "improvisation: mutating coreutils outside workspace",
		Match: func(args []string, _ bool) bool {
			if !isMutatingCoreutils(args[0]) {
				return false
			}
			// Heuristic: path outside workspace (contains "/" and not relative-only)
			for _, a := range args[1:] {
				if strings.HasPrefix(a, "/") && !strings.HasPrefix(a, "/tmp") {
					return true
				}
			}
			return false
		},
		Suggest: "mutating outside the workspace — preview first with `--dry-run` (e.g. `rm -r build --dry-run`).",
	},
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsSubstring(args []string, sub string) bool {
	for _, a := range args {
		if strings.Contains(a, sub) {
			return true
		}
	}
	return false
}

func isForeignCommand(name string) bool {
	// Foreign = not a bashy-owned cobra verb and not a coreutils applet.
	// Approximate by checking atlas/cobra verb list; unknown => foreign.
	verbs, _, _ := commandsCatalog()
	for _, v := range verbs {
		if v == name {
			return false
		}
	}
	// Also treat git, docker, kubectl, tar, unzip, rsync, make, apt as foreign curated.
	foreignCurated := map[string]bool{"git": true, "docker": true, "kubectl": true, "tar": true, "unzip": true, "rsync": true, "make": true, "apt": true, "ps": true, "df": true}
	if foreignCurated[name] {
		return true
	}
	return true // default: hint for unknown foreign, no harm if later filtered
}

func isMutatingCoreutils(name string) bool {
	mutators := map[string]bool{"rm": true, "mv": true, "cp": true, "ln": true, "chmod": true, "chown": true, "touch": true, "mkdir": true, "rmdir": true, "truncate": true, "install": true}
	return mutators[name]
}

// SuggestDeclared returns the first declared-row hint for the given argv, or "".
func SuggestDeclared(args []string, isBuiltin bool) string {
	if len(args) == 0 {
		return ""
	}
	for _, row := range declaredNudgeRows {
		if row.Match(args, isBuiltin) {
			return row.Suggest
		}
	}
	return ""
}
