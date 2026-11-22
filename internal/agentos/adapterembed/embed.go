// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Package adapterembed embeds the Bash# command adapter scripts (.bsh) from
// optional/, toolchain/, yoke/, examples/, and custom/ into the bashy binary
// as the last-resort fallback in resolveAdapterScript. Only the .bsh adapter
// wrappers are embedded; command Go sources stay unembedded and build on demand.
//
//go:generate go run ./sync.go
package adapterembed

import (
	"embed"
	"path"
	"strings"
)

//go:embed optional toolchain yoke examples custom
var FS embed.FS

// Folders lists the top-level adapter trees embedded in FS.
var Folders = []string{"optional", "toolchain", "yoke", "examples", "custom"}

// ExtraExampleScripts lists adapter/helper .bsh scripts under examples/ whose
// filenames do not match their immediate parent directory name.
var ExtraExampleScripts = map[string]bool{
	"examples/defaults/bashy-defaults.bsh":                true,
	"examples/desk/kit/outpost.bsh":                       true,
	"examples/transcribe/adapters/transcribe-adapter.bsh": true,
}

// ShouldEmbed reports whether a slash-separated path relative to the bashy
// repository root belongs in the embedded adapter bundle.
func ShouldEmbed(relSlash string) bool {
	relSlash = strings.TrimPrefix(relSlash, "./")
	if !strings.HasSuffix(relSlash, ".bsh") {
		return false
	}
	if strings.Contains(relSlash, "/checks/") || strings.Contains(relSlash, "/testdata/") || strings.HasSuffix(relSlash, "/contract.bsh") {
		return false
	}
	top, rest, ok := strings.Cut(relSlash, "/")
	if !ok || rest == "" {
		return false
	}
	validFolder := false
	for _, f := range Folders {
		if top == f {
			validFolder = true
			break
		}
	}
	if !validFolder {
		return false
	}
	if ExtraExampleScripts[relSlash] {
		return true
	}
	dir := path.Dir(relSlash)
	if dir == top || dir == "." {
		return false
	}
	return path.Base(dir)+".bsh" == path.Base(relSlash)
}
