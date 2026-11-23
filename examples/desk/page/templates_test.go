package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTemplatesLoad keeps every shipped template usable out of the box:
// it parses and `page add -f templates/NAME.md` stores it.
func TestTemplatesLoad(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("templates", "*.md"))
	if err != nil || len(files) < 3 {
		t.Fatalf("templates: %v %v", files, err)
	}
	url, a := startServer(t, "")
	run := cli(url, a)
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			segs, err := parseBody(string(b))
			if err != nil {
				t.Fatalf("parseBody: %v", err)
			}
			blocks := 0
			for _, s := range segs {
				if s.Block != nil {
					blocks++
				}
			}
			if blocks == 0 {
				t.Fatalf("%s has no block fences", f)
			}
			if code, out := run("add", "--id", name, "-f", f); code != 0 {
				t.Fatalf("page add -f %s: %d %q", f, code, out)
			}
			if code, out := run("show", name); code != 0 {
				t.Fatalf("page show %s: %d %q", name, code, out)
			}
		})
	}
}
