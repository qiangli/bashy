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

// Reproducible Standalone Build Instructions:
//
// To compile a transpiled Go output into a standalone binary:
// 1. Transpile Bash++ script to Go source:
//    bashy transpile --bashpp input.bpp -o output.go
// 2. Setup dependency clone (e.g. deps/sh pinned to commit aeecec06dde29255ed581ad61982246e9a52e617)
//    and configure module replace directive (mvdan.cc/sh/v3 => ../deps/sh):
//    go mod init standalone
//    go mod edit -require=mvdan.cc/sh/v3@v3.0.0
//    go mod edit -replace=mvdan.cc/sh/v3=../deps/sh
//    go mod tidy
// 3. Build a reproducible binary against the standard library / shell runtime:
//    go build -mod=mod -o myapp output.go
//
// Standalone binaries emitted by the lower compiler depend on the plain runtime
// base (mvdan.cc/sh/v3/lower/shellrt) using standard Go library primitives.
// Dynamic features requiring the shell execution engine are compiled to explicit
// shellrt bridge calls rather than unanalyzed interp invocations.
// See docs/transpile.md for complete build recipe and runtime details.

const sourceMapSchemaVersion = "bashy-transpile-map-v1"

type mapEntry struct {
	GoLine       int    `json:"go_line"`
	GoCol        int    `json:"go_col"`
	SourceLine   uint   `json:"source_line"`
	SourceCol    uint   `json:"source_col"`
	SourceOffset uint   `json:"source_offset"`
	Node         string `json:"node"`
}

type sourceMapArtifact struct {
	SchemaVersion string     `json:"schema_version"`
	Origin        string     `json:"origin"`
	GoDigest      string     `json:"go_digest"`
	Mappings      []mapEntry `json:"mappings"`
}

// formatDiagnostic renders a lower.Diagnostic. Structured diagnostics with a Code starting
// with "BASHPP-" are printed as exact Code + ": " + Msg without file position prefixes;
// ordinary LOWER- diagnostics retain their positioned format.
func formatDiagnostic(d lower.Diagnostic) string {
	if strings.HasPrefix(d.Code, "BASHPP-") {
		return fmt.Sprintf("%s: %s", d.Code, d.Msg)
	}
	return d.Error()
}

// normPath returns clean absolute path with symlinks resolved if possible.
func normPath(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	eval, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return filepath.Clean(eval)
	}
	dir := filepath.Dir(abs)
	base := filepath.Base(abs)
	evalDir, err := filepath.EvalSymlinks(dir)
	if err == nil {
		return filepath.Clean(filepath.Join(evalDir, base))
	}
	return filepath.Clean(abs)
}

// isSameFileOrAlias checks if two paths refer to the same underlying file or directory.
func isSameFileOrAlias(pathA, pathB string) bool {
	if pathA == "" || pathB == "" {
		return false
	}
	normA := normPath(pathA)
	normB := normPath(pathB)
	if normA == normB {
		return true
	}
	statA, errA := os.Stat(pathA)
	statB, errB := os.Stat(pathB)
	if errA == nil && errB == nil && os.SameFile(statA, statB) {
		return true
	}
	lstatA, lerrA := os.Lstat(pathA)
	lstatB, lerrB := os.Lstat(pathB)
	if lerrA == nil && lerrB == nil && os.SameFile(lstatA, lstatB) {
		return true
	}
	return false
}

// dispatchTranspile handles 'bashy transpile --bashpp INPUT -o OUTPUT.go [--map MAPFILE]'
func dispatchTranspile(args []string) int {
	var bashpp bool
	var output string
	var input string
	var mapFile string

	inFlags := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if inFlags && arg == "--" {
			inFlags = false
			continue
		}
		if inFlags && arg == "--bashpp" {
			bashpp = true
		} else if inFlags && arg == "-o" {
			if i+1 < len(args) {
				output = args[i+1]
				i++
			} else {
				fmt.Fprintln(os.Stderr, "transpile: missing argument for -o")
				return 2
			}
		} else if inFlags && strings.HasPrefix(arg, "-o") && len(arg) > 2 {
			output = arg[2:]
		} else if inFlags && arg == "--map" {
			if i+1 < len(args) {
				mapFile = args[i+1]
				i++
			} else {
				fmt.Fprintln(os.Stderr, "transpile: missing argument for --map")
				return 2
			}
		} else if inFlags && strings.HasPrefix(arg, "--map=") {
			mapFile = strings.TrimPrefix(arg, "--map=")
		} else if !inFlags || !strings.HasPrefix(arg, "-") {
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

	if mapFile == "" {
		if strings.HasSuffix(output, ".go") {
			mapFile = strings.TrimSuffix(output, ".go") + ".go.map"
		} else {
			mapFile = output + ".map"
		}
	}

	// Reject all path collisions
	if isSameFileOrAlias(input, output) {
		fmt.Fprintln(os.Stderr, "transpile: input and output path cannot be the same file")
		return 2
	}
	if isSameFileOrAlias(input, mapFile) {
		fmt.Fprintln(os.Stderr, "transpile: input and map path cannot be the same file")
		return 2
	}
	if isSameFileOrAlias(output, mapFile) {
		fmt.Fprintln(os.Stderr, "transpile: output and map path cannot be the same file")
		return 2
	}

	// Validate destination directory vs file types
	if st, err := os.Stat(input); err == nil && st.IsDir() {
		fmt.Fprintf(os.Stderr, "transpile: input path is a directory: %s\n", input)
		return 2
	}
	if st, err := os.Stat(output); err == nil && st.IsDir() {
		fmt.Fprintf(os.Stderr, "transpile: output path is a directory: %s\n", output)
		return 2
	}
	if st, err := os.Stat(mapFile); err == nil && st.IsDir() {
		fmt.Fprintf(os.Stderr, "transpile: map path is a directory: %s\n", mapFile)
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
				fmt.Fprintln(os.Stderr, formatDiagnostic(diag))
			}
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		return 2
	}

	entries := make([]mapEntry, 0, len(res.Mappings))
	for _, m := range res.Mappings {
		entries = append(entries, mapEntry{
			GoLine:       m.GoLine,
			GoCol:        m.GoCol,
			SourceLine:   m.Pos.Line(),
			SourceCol:    m.Pos.Col(),
			SourceOffset: m.Pos.Offset(),
			Node:         m.Node,
		})
	}

	goDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(res.Source))
	mapArt := sourceMapArtifact{
		SchemaVersion: sourceMapSchemaVersion,
		Origin:        res.Origin,
		GoDigest:      goDigest,
		Mappings:      entries,
	}
	mapData, err := json.MarshalIndent(mapArt, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "transpile: map marshal error: %v\n", err)
		return 2
	}

	return writeOutputsAtomic(output, res.Source, mapFile, mapData)
}

func writeOutputsAtomic(outputPath string, outputData []byte, mapPath string, mapData []byte) int {
	outDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	mapDir := filepath.Dir(mapPath)
	if err := os.MkdirAll(mapDir, 0755); err != nil {
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

	if _, err := tmpOut.Write(outputData); err != nil {
		tmpOut.Close()
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	if err := tmpOut.Sync(); err != nil {
		tmpOut.Close()
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}
	if err := tmpOut.Close(); err != nil {
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
	if err := tmpMap.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}

	var origOutData []byte
	var origOutMode os.FileMode = 0644
	outExisted := false
	if st, err := os.Stat(outputPath); err == nil {
		if data, err := os.ReadFile(outputPath); err == nil {
			origOutData = data
			origOutMode = st.Mode().Perm()
			outExisted = true
		}
	}

	var origMapData []byte
	var origMapMode os.FileMode = 0644
	mapExisted := false
	if st, err := os.Stat(mapPath); err == nil {
		if data, err := os.ReadFile(mapPath); err == nil {
			origMapData = data
			origMapMode = st.Mode().Perm()
			mapExisted = true
		}
	}

	if err := os.Rename(tmpOutName, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}

	if err := os.Rename(tmpMapName, mapPath); err != nil {
		if outExisted {
			if rerr := os.WriteFile(outputPath, origOutData, origOutMode); rerr != nil {
				fmt.Fprintf(os.Stderr, "transpile: rollback failed restoring output file: %v\n", rerr)
			} else if cerr := os.Chmod(outputPath, origOutMode); cerr != nil {
				fmt.Fprintf(os.Stderr, "transpile: rollback failed setting permissions on output file: %v\n", cerr)
			}
		} else {
			if rerr := os.Remove(outputPath); rerr != nil && !os.IsNotExist(rerr) {
				fmt.Fprintf(os.Stderr, "transpile: rollback failed removing output file: %v\n", rerr)
			}
		}
		if mapExisted {
			if rerr := os.WriteFile(mapPath, origMapData, origMapMode); rerr != nil {
				fmt.Fprintf(os.Stderr, "transpile: rollback failed restoring map file: %v\n", rerr)
			} else if cerr := os.Chmod(mapPath, origMapMode); cerr != nil {
				fmt.Fprintf(os.Stderr, "transpile: rollback failed setting permissions on map file: %v\n", cerr)
			}
		}
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		return 2
	}

	return 0
}
