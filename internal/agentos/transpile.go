package agentos

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/lower"
	"mvdan.cc/sh/v3/syntax"
)

type sourceMapArtifact struct {
	Origin   string          `json:"origin"`
	GoDigest string          `json:"go_digest"`
	Mappings []lower.Mapping `json:"mappings"`
}

// dispatchTranspile handles 'bashy transpile --bashpp INPUT -o OUTPUT.go [--map MAPFILE]'
func dispatchTranspile(args []string) int {
	var bashpp bool
	var output string
	var input string
	var mapFile string

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
		} else if arg == "--map" {
			if i+1 < len(args) {
				mapFile = args[i+1]
				i++
			} else {
				fmt.Fprintln(os.Stderr, "transpile: missing argument for --map")
				return 2
			}
		} else if strings.HasPrefix(arg, "--map=") {
			mapFile = strings.TrimPrefix(arg, "--map=")
		} else if !strings.HasPrefix(arg, "-") {
			if input == "" {
				input = arg
			} else {
				fmt.Fprintf(os.Stderr, "transpile: unexpected argument: %s\n", arg)
				return 2
			}
		} else {
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

	absInput, errInput := filepath.Abs(input)
	absOutput, errOutput := filepath.Abs(output)
	if errInput == nil && errOutput == nil && absInput == absOutput {
		fmt.Fprintln(os.Stderr, "transpile: input and output path cannot be the same file")
		return 2
	}

	f, err := os.Open(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	defer f.Close()

	parser := syntax.NewParser(syntax.KeepComments(true), syntax.Variant(syntax.LangBashPP))
	file, err := parser.Parse(f, input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
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
		return 2
	}

	if mapFile == "" {
		if strings.HasSuffix(output, ".go") {
			mapFile = strings.TrimSuffix(output, ".go") + ".go.map"
		} else {
			mapFile = output + ".map"
		}
	}

	goDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(res.Source))
	mapArt := sourceMapArtifact{
		Origin:   res.Origin,
		GoDigest: goDigest,
		Mappings: res.Mappings,
	}
	mapData, err := json.MarshalIndent(mapArt, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "transpile: map marshal error: %v\n", err)
		return 2
	}

	outDir := filepath.Dir(output)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}

	tmpOut, err := os.CreateTemp(outDir, ".transpile-out-*.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	tmpOutName := tmpOut.Name()
	defer os.Remove(tmpOutName)

	if _, err := tmpOut.Write(res.Source); err != nil {
		tmpOut.Close()
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	if err := tmpOut.Sync(); err != nil {
		tmpOut.Close()
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	tmpOut.Close()

	mapDir := filepath.Dir(mapFile)
	if err := os.MkdirAll(mapDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	tmpMap, err := os.CreateTemp(mapDir, ".transpile-map-*.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	tmpMapName := tmpMap.Name()
	defer os.Remove(tmpMapName)

	if _, err := tmpMap.Write(mapData); err != nil {
		tmpMap.Close()
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	if err := tmpMap.Sync(); err != nil {
		tmpMap.Close()
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	tmpMap.Close()

	if err := os.Rename(tmpOutName, output); err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}

	if err := os.Rename(tmpMapName, mapFile); err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}

	return 0
}
