// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// The blog generator: a self-contained, pure-Go static site compiler for a
// personal blog or wiki with rich media. It reads a Hugo-compatible source
// tree (hugo.toml, content/, media/, static/) and writes public/. The same
// layout the Hugo CLI reads, so either builder produces the site.
//
// Rich content beyond plain Markdown, all rendered locally with no outbound
// calls: fenced code blocks with syntax coloring, fenced diagrams rendered
// to inline SVG, block and inline math from a small TeX subset, video and
// audio players with caption tracks, and terminal scenes recorded by the
// cast family rendered as colored transcripts.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: generator <source-dir> [output-dir]\n")
		os.Exit(1)
	}

	srcDir, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving source dir: %v\n", err)
		os.Exit(1)
	}

	outDir := filepath.Join(srcDir, "public")
	if len(os.Args) >= 3 {
		outDir, err = filepath.Abs(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving out dir: %v\n", err)
			os.Exit(1)
		}
	}

	if err := BuildSite(srcDir, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Blog successfully generated at: %s\n", outDir)
}

// seriesIndexHook lets a shaped copy add series index pages without editing
// this file (see demo/series-index): base blogs have no /series/ pages; a
// wish can add them by shipping one extra Go file that sets this hook.
var seriesIndexHook func(cfg SiteConfig, posts []*Post, outDir string) error

// BuildSite compiles srcDir into outDir.
func BuildSite(srcDir, outDir string) error {
	cfg := ParseConfig(filepath.Join(srcDir, "hugo.toml"))
	cfg.SourceDir = srcDir

	posts, pages, err := LoadContent(filepath.Join(srcDir, "content"))
	if err != nil {
		return err
	}
	if len(posts) == 0 && len(pages) == 0 {
		return fmt.Errorf("no content found under %s", filepath.Join(srcDir, "content"))
	}

	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("clean outDir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir outDir: %w", err)
	}

	if err := copyDir(filepath.Join(srcDir, "static"), outDir); err != nil {
		return fmt.Errorf("copy static assets: %w", err)
	}
	if err := copyDir(filepath.Join(srcDir, "media"), filepath.Join(outDir, "media")); err != nil {
		return fmt.Errorf("copy media: %w", err)
	}

	for _, post := range posts {
		html := RenderPost(cfg, post, posts)
		dir := filepath.Join(outDir, "p", post.Slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0o644); err != nil {
			return fmt.Errorf("write post %s: %w", post.Slug, err)
		}
	}

	for _, page := range pages {
		html := RenderPage(cfg, page)
		dir := filepath.Join(outDir, page.Slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0o644); err != nil {
			return fmt.Errorf("write page %s: %w", page.Slug, err)
		}
	}

	home, err := os.ReadFile(filepath.Join(srcDir, "content", "_index.md"))
	if err == nil {
		cfg.Intro = Markdown(ParseDoc(string(home), "_index").Body)
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(RenderHome(cfg, posts)), 0o644); err != nil {
		return fmt.Errorf("write home: %w", err)
	}

	if err := os.WriteFile(filepath.Join(outDir, "index.xml"), []byte(RSS(cfg, posts)), 0o644); err != nil {
		return fmt.Errorf("write rss: %w", err)
	}

	if seriesIndexHook != nil {
		if err := seriesIndexHook(cfg, posts, outDir); err != nil {
			return fmt.Errorf("series index: %w", err)
		}
	}

	_ = os.WriteFile(filepath.Join(outDir, ".nojekyll"), []byte(""), 0o644)
	return nil
}
