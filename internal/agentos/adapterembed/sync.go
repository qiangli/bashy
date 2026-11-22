//go:build ignore

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var folders = []string{"optional", "toolchain", "yoke", "examples", "custom"}

var extraExampleScripts = map[string]bool{
	"examples/defaults/bashy-defaults.bsh":                true,
	"examples/desk/kit/outpost.bsh":                       true,
	"examples/transcribe/adapters/transcribe-adapter.bsh": true,
}

func shouldEmbed(relSlash string) bool {
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
	for _, f := range folders {
		if top == f {
			validFolder = true
			break
		}
	}
	if !validFolder {
		return false
	}
	if extraExampleScripts[relSlash] {
		return true
	}
	dir := path.Dir(relSlash)
	if dir == top || dir == "." {
		return false
	}
	return path.Base(dir)+".bsh" == path.Base(relSlash)
}

func main() {
	here, err := os.Getwd()
	if err != nil {
		fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(here, "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		fatalf("cannot locate bashy root at %s: %v", repoRoot, err)
	}

	for _, f := range folders {
		if err := os.RemoveAll(filepath.Join(here, f)); err != nil {
			fatalf("clean %s: %v", f, err)
		}
	}

	count := 0
	totalBytes := 0
	for _, f := range folders {
		srcDir := filepath.Join(repoRoot, f)
		err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(repoRoot, p)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			if !shouldEmbed(relSlash) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
			dst := filepath.Join(here, filepath.FromSlash(relSlash))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dst, raw, 0o644); err != nil {
				return err
			}
			count++
			totalBytes += len(raw)
			return nil
		})
		if err != nil {
			fatalf("walk %s: %v", srcDir, err)
		}
	}
	fmt.Printf("synced %d adapter .bsh scripts (%d bytes)\n", count, totalBytes)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "adapterembed sync: "+format+"\n", args...)
	os.Exit(1)
}
