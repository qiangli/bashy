package agentos

import (
	"os"
	"runtime"
	"strings"

	"mvdan.cc/sh/v3/pathconv"
)

// transpilePathArgs resolves filesystem operands from the shell's spelling to
// the host spelling before the BashSharp transpiler reaches os.Open, os.Stat,
// and os.WriteFile. On Windows, Bashy deliberately exposes paths such as
// /c/Users/... and /tmp/... to scripts; those spellings must work at this
// front-door boundary just as they do for a normal script operand.
func transpilePathArgs(args []string) []string {
	if runtime.GOOS != "windows" {
		return args
	}
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	return transpilePathArgsMode(args, dir, true)
}

// transpilePathArgsMode carries an explicit platform mode so argument
// classification can be covered on every host. It mirrors transpile.Main's
// scanner: import paths and language/version values are identifiers, while
// file operands are converted through the engine's shared path converter.
func transpilePathArgsMode(args []string, dir string, windows bool) []string {
	if !windows {
		return args
	}
	out := append([]string(nil), args...)
	pathValue := map[string]bool{
		"-o":              true,
		"--map":           true,
		"--go-file":       true,
		"--go-test-file":  true,
		"--go-xtest-file": true,
		"--go-library":    true,
	}
	nonPathValue := map[string]bool{
		"--source":         true,
		"--go-version":     true,
		"--go-import-base": true,
		"--go-import-path": true,
	}
	convert := func(path string) string {
		if path == "" || path == "-" {
			return path
		}
		return pathconv.ToOSMode(dir, path, true)
	}
	convertPackage := func(value string) string {
		importPath, files, ok := strings.Cut(value, "=")
		if !ok {
			return value
		}
		parts := strings.Split(files, ",")
		for i := range parts {
			parts[i] = convert(parts[i])
		}
		return importPath + "=" + strings.Join(parts, ",")
	}

	inFlags := true
	for i := 0; i < len(out); i++ {
		arg := out[i]
		if inFlags && arg == "--" {
			inFlags = false
			continue
		}
		if !inFlags {
			out[i] = convert(arg)
			continue
		}
		if pathValue[arg] {
			if i+1 < len(out) {
				i++
				out[i] = convert(out[i])
			}
			continue
		}
		if nonPathValue[arg] {
			i++
			continue
		}
		if arg == "--go-package" {
			if i+1 < len(out) {
				i++
				out[i] = convertPackage(out[i])
			}
			continue
		}
		if strings.HasPrefix(arg, "--go-package=") {
			out[i] = "--go-package=" + convertPackage(strings.TrimPrefix(arg, "--go-package="))
			continue
		}
		converted := false
		for flag := range pathValue {
			prefix := flag + "="
			if strings.HasPrefix(arg, prefix) {
				out[i] = prefix + convert(strings.TrimPrefix(arg, prefix))
				converted = true
				break
			}
		}
		if converted {
			continue
		}
		if strings.HasPrefix(arg, "-o") && len(arg) > 2 {
			out[i] = "-o" + convert(arg[2:])
			continue
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			out[i] = convert(arg)
		}
	}
	return out
}
