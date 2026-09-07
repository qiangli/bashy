package agentos

import (
	"fmt"
	"os"
	"strings"

	"mvdan.cc/sh/v3/lower"
	"mvdan.cc/sh/v3/syntax"
)

// dispatchTranspile handles 'bashy transpile --bashpp INPUT -o OUTPUT.go'
func dispatchTranspile(args []string) int {
	var bashpp bool
	var output string
	var input string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--bashpp" {
			bashpp = true
		} else if arg == "-o" {
			if i+1 < len(args) {
				output = args[i+1]
				i++
			} else {
				fmt.Fprintln(os.Stderr, "transpile: missing argument for -o")
				return 2
			}
		} else if strings.HasPrefix(arg, "-o") && len(arg) > 2 {
			output = arg[2:]
		} else if !strings.HasPrefix(arg, "-") {
			if input == "" {
				input = arg
			} else {
				fmt.Fprintf(os.Stderr, "transpile: unexpected argument: %s\n", arg)
				return 2
			}
		} else {
			// Preserve Classic/POSIX modes: if they pass --posix instead of --bashpp, we reject it
			// because transpile currently requires BashPP. But we parse it to give a clear error.
			if arg == "--posix" || arg == "--classic" {
				fmt.Fprintf(os.Stderr, "transpile: %s is not supported for transpilation\n", arg)
				return 2
			}
			fmt.Fprintf(os.Stderr, "transpile: unknown flag: %s\n", arg)
			return 2
		}
	}

	if input == "" {
		fmt.Fprintln(os.Stderr, "transpile: missing INPUT")
		return 2
	}
	if !bashpp {
		fmt.Fprintln(os.Stderr, "transpile: --bashpp is required")
		return 2
	}
	if output == "" {
		fmt.Fprintln(os.Stderr, "transpile: missing -o OUTPUT.go")
		return 2
	}

	f, err := os.Open(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 1
	}
	defer f.Close()

	parser := syntax.NewParser(syntax.KeepComments(true), syntax.Variant(syntax.LangBashPP))
	file, err := parser.Parse(f, input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	opts := lower.Options{
		Origin: input,
	}
	res, err := lower.Compile(file, opts)
	if err != nil {
		if el, ok := err.(lower.ErrorList); ok {
			for _, diag := range el {
				fmt.Fprintln(os.Stderr, diag.Error())
			}
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		return 1 // nonzero exit/no output on rejection
	}

	// Write output only on success, stable emission independent output path
	if err := os.WriteFile(output, res.Source, 0666); err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 1
	}

	return 0
}
