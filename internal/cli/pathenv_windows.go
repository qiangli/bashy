// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build windows

package cli

import (
	"strings"
)

func shellStartupEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		name, value, ok := strings.Cut(e, "=")
		if !ok {
			out = append(out, e)
			continue
		}
		if shellDirectoryEnv(name) {
			value = windowsPathToShell(value)
		}
		out = append(out, name+"="+value)
	}
	return out
}

func shellDirectoryEnv(name string) bool {
	switch strings.ToUpper(name) {
	case "HOME", "USERPROFILE", "PWD", "OLDPWD", "TMP", "TEMP", "TMPDIR":
		return true
	default:
		return false
	}
}

func windowsPathToShell(path string) string {
	if path == "" {
		return path
	}
	p := strings.ReplaceAll(path, `\`, "/")
	if len(p) >= 2 && asciiLetter(p[0]) && p[1] == ':' {
		drive := p[0]
		if 'A' <= drive && drive <= 'Z' {
			drive += 'a' - 'A'
		}
		rest := p[2:]
		if rest == "" {
			rest = "/"
		} else if rest[0] != '/' {
			rest = "/" + rest
		}
		return "/" + string(drive) + rest
	}
	return p
}

func asciiLetter(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}
