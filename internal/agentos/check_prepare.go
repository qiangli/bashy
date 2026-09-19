package agentos

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/polyglot"
)

// islandToolsFor maps a fence language to the tool names its island asks the
// resolver for (toolchains.go). Rust links through the provisioned cc, and
// TypeScript needs the compiler module as well as node.
func islandToolsFor(language string) []string {
	switch language {
	case "python":
		return []string{"python3"}
	case "typescript":
		return []string{"node", "typescript"}
	case "rust":
		return []string{"rustc", "cc-linker"}
	case "c":
		return []string{"cc"}
	case "cpp":
		return []string{"c++"}
	case "go":
		return []string{"go"}
	}
	return nil
}

// fenceLanguages lists the island languages a Bash# script opens (`~~~py`,
// `~~~ts as ts`, …); a Go source program (--source=go, .go) is a Go island.
func fenceLanguages(path string) ([]string, error) {
	if strings.HasSuffix(path, ".go") {
		return []string{"go"}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	seen := map[string]bool{}
	var langs []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "~~~") {
			continue
		}
		word := strings.Fields(strings.TrimPrefix(line, "~~~"))
		if len(word) == 0 {
			continue
		}
		lang := polyglot.CanonicalLanguage(word[0])
		if islandToolsFor(lang) != nil && !seen[lang] {
			seen[lang] = true
			langs = append(langs, lang)
		}
	}
	return langs, sc.Err()
}

// checkPrepare provisions the toolchains the given scripts' islands will ask
// for — the same rows the resolver answers at run time, paid now instead of
// on first use (a CI image, a Dockerfile step, an air-gapped host). With no
// script it walks every row. Provisioning is cache-first, so a second run
// changes nothing and downloads nothing; it prints the same lines. Nothing
// is executed and no island is built: the worker build is per run.
func checkPrepare(scripts []string, stdout, stderr io.Writer) int {
	if certProfile() {
		fmt.Fprintln(stderr, "check --prepare: islands resolve their tools from PATH under VSC_PROFILE=cert; nothing to provision")
		return 2
	}
	names := map[string]bool{}
	if len(scripts) == 0 {
		for _, lang := range []string{"go", "c", "cpp", "python", "typescript", "rust"} {
			for _, n := range islandToolsFor(lang) {
				names[n] = true
			}
		}
	}
	for _, script := range scripts {
		langs, err := fenceLanguages(script)
		if err != nil {
			fmt.Fprintf(stderr, "check --prepare: %s: %v\n", script, err)
			return 2
		}
		for _, lang := range langs {
			for _, n := range islandToolsFor(lang) {
				names[n] = true
			}
		}
	}
	ordered := make([]string, 0, len(names))
	for n := range names {
		ordered = append(ordered, n)
	}
	sort.Strings(ordered)
	if len(ordered) == 0 {
		fmt.Fprintln(stdout, "check --prepare: no islands; nothing to provision")
		return 0
	}
	rc := 0
	for _, name := range ordered {
		argv, why, err := islandToolResolver(name)
		if err != nil {
			fmt.Fprintf(stderr, "check --prepare: %s: %v\n", name, err)
			rc = 1
			continue
		}
		fmt.Fprintf(stdout, "%-10s %s\t%s\n", name, strings.Join(argv, " "), why)
	}
	return rc
}
